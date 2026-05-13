package embedder

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/qdrant/go-client/qdrant"

	"github.com/jarviisha/codohue/internal/core/embedstrategy"
	"github.com/jarviisha/codohue/internal/core/namespace"
)

// --- fakes ----------------------------------------------------------------

type fakeRepo struct {
	loadItem *PendingItem
	loadErr  error

	markInFlightAttempt int
	markInFlightErr     error

	markEmbeddedErr   error
	markFailedErr     error
	markDeadLetterErr error

	calls struct {
		loadByID       []int64
		markInFlight   []int64
		markEmbedded   []embeddedCall
		markFailed     []failedCall
		markDeadLetter []failedCall
	}
}

type embeddedCall struct {
	id              int64
	strategyID      string
	strategyVersion string
}

type failedCall struct {
	id   int64
	last string
}

func (f *fakeRepo) LoadByID(_ context.Context, id int64) (*PendingItem, error) {
	f.calls.loadByID = append(f.calls.loadByID, id)
	return f.loadItem, f.loadErr
}

func (f *fakeRepo) MarkInFlight(_ context.Context, id int64) (int, error) {
	f.calls.markInFlight = append(f.calls.markInFlight, id)
	return f.markInFlightAttempt, f.markInFlightErr
}

func (f *fakeRepo) MarkEmbedded(_ context.Context, id int64, sid, sver string, _ time.Time) error {
	f.calls.markEmbedded = append(f.calls.markEmbedded, embeddedCall{id, sid, sver})
	return f.markEmbeddedErr
}

func (f *fakeRepo) MarkFailed(_ context.Context, id int64, last string) error {
	f.calls.markFailed = append(f.calls.markFailed, failedCall{id, last})
	return f.markFailedErr
}

func (f *fakeRepo) MarkDeadLetter(_ context.Context, id int64, last string) error {
	f.calls.markDeadLetter = append(f.calls.markDeadLetter, failedCall{id, last})
	return f.markDeadLetterErr
}

type fakeNSCfg struct {
	cfg *namespace.Config
	err error
}

func (f *fakeNSCfg) Get(_ context.Context, _ string) (*namespace.Config, error) {
	return f.cfg, f.err
}

type fakeIDMapper struct {
	pointID uint64
	err     error
	calls   int
}

func (f *fakeIDMapper) GetOrCreateObjectID(_ context.Context, _, _ string) (uint64, error) {
	f.calls++
	return f.pointID, f.err
}

type fakeStrategy struct {
	id, version string
	dim         int
	embedFn     func(ctx context.Context, content string) ([]float32, error)
}

func (f *fakeStrategy) ID() string         { return f.id }
func (f *fakeStrategy) Version() string    { return f.version }
func (f *fakeStrategy) Dim() int           { return f.dim }
func (f *fakeStrategy) MaxInputBytes() int { return 0 }
func (f *fakeStrategy) Embed(ctx context.Context, content string) ([]float32, error) {
	if f.embedFn != nil {
		return f.embedFn(ctx, content)
	}
	return make([]float32, f.dim), nil
}

type fakeRegistry struct {
	strategy embedstrategy.Strategy
	err      error
	calls    int
}

func (f *fakeRegistry) Build(_, _ string, _ embedstrategy.Params) (embedstrategy.Strategy, error) {
	f.calls++
	return f.strategy, f.err
}

// --- helpers --------------------------------------------------------------

func enabledCfg(dim int) *namespace.Config {
	return &namespace.Config{
		Namespace:              "ns",
		EmbeddingDim:           dim,
		CatalogEnabled:         true,
		CatalogStrategyID:      "internal-hashing-ngrams",
		CatalogStrategyVersion: "v1",
		CatalogMaxAttempts:     5,
		CatalogMaxContentBytes: 32768,
		DenseDistance:          "cosine",
	}
}

//nolint:gocritic // returning the rig as 6 values keeps test call sites concise; bundling into a struct adds no clarity.
func newSvc(t *testing.T, opts ...func(*Service)) (*Service, *fakeRepo, *fakeNSCfg, *fakeRegistry, *fakeIDMapper, *[]upsertCall) {
	t.Helper()
	repo := &fakeRepo{
		loadItem: &PendingItem{ID: 7, Namespace: "ns", ObjectID: "obj1", Content: "hello world"},
	}
	nsCfg := &fakeNSCfg{cfg: enabledCfg(128)}
	reg := &fakeRegistry{strategy: &fakeStrategy{id: "internal-hashing-ngrams", version: "v1", dim: 128}}
	idmap := &fakeIDMapper{pointID: 99}
	upserts := &[]upsertCall{}

	svc := &Service{
		repo:     repo,
		nsCfg:    nsCfg,
		registry: reg,
		idmap:    idmap,
		clock:    func() time.Time { return time.Date(2026, 5, 9, 0, 0, 0, 0, time.UTC) },
		cache:    make(map[string]cachedStrategy),
		ensured:  make(map[string]struct{}),
	}
	svc.qdrantUpsertFn = func(_ context.Context, p *qdrant.UpsertPoints) error {
		*upserts = append(*upserts, upsertCall{collection: p.CollectionName, points: p.Points})
		return nil
	}
	svc.ensureCollFn = func(_ context.Context, _ string, _ uint64, _ string) error { return nil }

	for _, opt := range opts {
		opt(svc)
	}
	return svc, repo, nsCfg, reg, idmap, upserts
}

type upsertCall struct {
	collection string
	points     []*qdrant.PointStruct
}

// --- happy path -----------------------------------------------------------

func TestServiceProcessItem_HappyPath_Embedded(t *testing.T) {
	svc, repo, _, reg, idmap, upserts := newSvc(t)
	repo.markInFlightAttempt = 1

	out, err := svc.ProcessItem(context.Background(), 7)
	if err != nil {
		t.Fatalf("ProcessItem: %v", err)
	}
	if out != OutcomeEmbedded {
		t.Errorf("expected OutcomeEmbedded, got %v", out)
	}
	if !out.ShouldAck() {
		t.Errorf("OutcomeEmbedded should ACK")
	}
	if len(repo.calls.markEmbedded) != 1 {
		t.Errorf("expected exactly 1 MarkEmbedded call, got %d", len(repo.calls.markEmbedded))
	}
	if len(*upserts) != 1 {
		t.Errorf("expected 1 qdrant upsert, got %d", len(*upserts))
	}
	if (*upserts)[0].collection != "ns_objects_dense" {
		t.Errorf("collection: got %q", (*upserts)[0].collection)
	}
	if reg.calls != 1 {
		t.Errorf("expected registry built once, got %d", reg.calls)
	}
	if idmap.calls != 1 {
		t.Errorf("expected idmap called once, got %d", idmap.calls)
	}
}

// --- skip outcomes --------------------------------------------------------

func TestServiceProcessItem_ItemNotFound_Skipped(t *testing.T) {
	svc, repo, _, _, _, _ := newSvc(t)
	repo.loadErr = ErrItemNotFound

	out, err := svc.ProcessItem(context.Background(), 7)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != OutcomeSkipped {
		t.Errorf("expected OutcomeSkipped, got %v", out)
	}
	if !out.ShouldAck() {
		t.Errorf("OutcomeSkipped should ACK")
	}
	if len(repo.calls.markInFlight) != 0 {
		t.Errorf("MarkInFlight should NOT be called when item missing")
	}
}

func TestServiceProcessItem_NamespaceDisabled_Skipped(t *testing.T) {
	svc, repo, nsCfg, _, _, _ := newSvc(t)
	cfg := enabledCfg(128)
	cfg.CatalogEnabled = false
	nsCfg.cfg = cfg

	out, err := svc.ProcessItem(context.Background(), 7)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != OutcomeSkipped {
		t.Errorf("expected OutcomeSkipped, got %v", out)
	}
	if len(repo.calls.markInFlight) != 0 {
		t.Errorf("MarkInFlight should NOT be called when namespace disabled")
	}
}

func TestServiceProcessItem_NamespaceMissing_Skipped(t *testing.T) {
	svc, _, nsCfg, _, _, _ := newSvc(t)
	nsCfg.cfg = nil

	out, err := svc.ProcessItem(context.Background(), 7)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != OutcomeSkipped {
		t.Errorf("expected OutcomeSkipped, got %v", out)
	}
}

func TestServiceProcessItem_MarkInFlightRaceItemDeleted_Skipped(t *testing.T) {
	svc, repo, _, _, _, _ := newSvc(t)
	repo.markInFlightErr = ErrItemNotFound

	out, err := svc.ProcessItem(context.Background(), 7)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != OutcomeSkipped {
		t.Errorf("expected OutcomeSkipped, got %v", out)
	}
}

// --- dead-letter outcomes -------------------------------------------------

func TestServiceProcessItem_AttemptExceedsMax_DeadLetter(t *testing.T) {
	svc, repo, _, _, _, _ := newSvc(t)
	repo.markInFlightAttempt = 6 // max=5 from enabledCfg

	out, err := svc.ProcessItem(context.Background(), 7)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != OutcomeDeadLetter {
		t.Errorf("expected OutcomeDeadLetter, got %v", out)
	}
	if len(repo.calls.markDeadLetter) != 1 {
		t.Errorf("expected MarkDeadLetter called")
	}
}

func TestServiceProcessItem_StrategyResolveError_DeadLetter(t *testing.T) {
	svc, repo, _, reg, _, _ := newSvc(t)
	repo.markInFlightAttempt = 1
	reg.err = embedstrategy.ErrUnknownStrategy

	out, err := svc.ProcessItem(context.Background(), 7)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != OutcomeDeadLetter {
		t.Errorf("expected OutcomeDeadLetter on strategy resolve failure, got %v", out)
	}
	if len(repo.calls.markDeadLetter) != 1 {
		t.Errorf("expected MarkDeadLetter called")
	}
}

func TestServiceProcessItem_ZeroNormVector_DeadLetter(t *testing.T) {
	svc, repo, _, reg, _, _ := newSvc(t)
	repo.markInFlightAttempt = 1
	reg.strategy = &fakeStrategy{id: "x", version: "v1", dim: 128, embedFn: func(_ context.Context, _ string) ([]float32, error) {
		return nil, embedstrategy.ErrZeroNorm
	}}

	out, err := svc.ProcessItem(context.Background(), 7)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != OutcomeDeadLetter {
		t.Errorf("expected OutcomeDeadLetter on zero-norm, got %v", out)
	}
}

func TestServiceProcessItem_DimensionMismatch_DeadLetter(t *testing.T) {
	svc, repo, _, reg, _, _ := newSvc(t)
	repo.markInFlightAttempt = 1
	reg.strategy = &fakeStrategy{id: "x", version: "v1", dim: 128, embedFn: func(_ context.Context, _ string) ([]float32, error) {
		// Strategy reports dim=128 but actually returns a 64-d vector.
		return make([]float32, 64), nil
	}}

	out, err := svc.ProcessItem(context.Background(), 7)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != OutcomeDeadLetter {
		t.Errorf("expected OutcomeDeadLetter on dim mismatch, got %v", out)
	}
}

// --- failed (transient) outcomes -----------------------------------------

func TestServiceProcessItem_TransientEmbedError_Failed(t *testing.T) {
	svc, repo, _, reg, _, _ := newSvc(t)
	repo.markInFlightAttempt = 1
	reg.strategy = &fakeStrategy{id: "x", version: "v1", dim: 128, embedFn: func(_ context.Context, _ string) ([]float32, error) {
		return nil, embedstrategy.ErrTransient
	}}

	out, err := svc.ProcessItem(context.Background(), 7)
	if err == nil {
		t.Fatal("expected error to surface for OutcomeFailed")
	}
	if out != OutcomeFailed {
		t.Errorf("expected OutcomeFailed, got %v", out)
	}
	if out.ShouldAck() {
		t.Errorf("OutcomeFailed must NOT ACK")
	}
	if len(repo.calls.markFailed) != 1 {
		t.Errorf("expected MarkFailed called")
	}
}

func TestServiceProcessItem_ContextCanceled_Failed(t *testing.T) {
	svc, repo, _, reg, _, _ := newSvc(t)
	repo.markInFlightAttempt = 1
	reg.strategy = &fakeStrategy{id: "x", version: "v1", dim: 128, embedFn: func(_ context.Context, _ string) ([]float32, error) {
		return nil, context.Canceled
	}}

	out, _ := svc.ProcessItem(context.Background(), 7)
	if out != OutcomeFailed {
		t.Errorf("expected OutcomeFailed, got %v", out)
	}
}

func TestServiceProcessItem_UnknownEmbedError_Failed(t *testing.T) {
	svc, repo, _, reg, _, _ := newSvc(t)
	repo.markInFlightAttempt = 1
	reg.strategy = &fakeStrategy{id: "x", version: "v1", dim: 128, embedFn: func(_ context.Context, _ string) ([]float32, error) {
		return nil, errors.New("kaboom")
	}}

	out, _ := svc.ProcessItem(context.Background(), 7)
	if out != OutcomeFailed {
		t.Errorf("expected OutcomeFailed for unknown error, got %v", out)
	}
	if len(repo.calls.markFailed) != 1 {
		t.Errorf("expected MarkFailed called")
	}
}

func TestServiceProcessItem_QdrantUpsertError_Failed(t *testing.T) {
	svc, repo, _, _, _, _ := newSvc(t)
	repo.markInFlightAttempt = 1
	svc.qdrantUpsertFn = func(_ context.Context, _ *qdrant.UpsertPoints) error {
		return errors.New("qdrant down")
	}

	out, _ := svc.ProcessItem(context.Background(), 7)
	if out != OutcomeFailed {
		t.Errorf("expected OutcomeFailed on qdrant error, got %v", out)
	}
	if len(repo.calls.markFailed) != 1 {
		t.Errorf("expected MarkFailed called")
	}
	if len(repo.calls.markEmbedded) != 0 {
		t.Errorf("MarkEmbedded must NOT be called when qdrant fails")
	}
}

func TestServiceProcessItem_IDMapError_Failed(t *testing.T) {
	svc, repo, _, _, idmap, _ := newSvc(t)
	repo.markInFlightAttempt = 1
	idmap.err = errors.New("idmap down")

	out, _ := svc.ProcessItem(context.Background(), 7)
	if out != OutcomeFailed {
		t.Errorf("expected OutcomeFailed on idmap error, got %v", out)
	}
}

func TestServiceProcessItem_EnsureCollectionsError_Failed(t *testing.T) {
	svc, repo, _, _, _, _ := newSvc(t)
	repo.markInFlightAttempt = 1
	svc.ensureCollFn = func(_ context.Context, _ string, _ uint64, _ string) error {
		return errors.New("ensure failed")
	}

	out, _ := svc.ProcessItem(context.Background(), 7)
	if out != OutcomeFailed {
		t.Errorf("expected OutcomeFailed on ensure error, got %v", out)
	}
}

func TestServiceProcessItem_MarkEmbeddedError_FailedButQdrantWritten(t *testing.T) {
	svc, repo, _, _, _, upserts := newSvc(t)
	repo.markInFlightAttempt = 1
	repo.markEmbeddedErr = errors.New("postgres down")

	out, _ := svc.ProcessItem(context.Background(), 7)
	if out != OutcomeFailed {
		t.Errorf("expected OutcomeFailed when MarkEmbedded fails, got %v", out)
	}
	if len(*upserts) != 1 {
		t.Errorf("qdrant upsert SHOULD have happened before MarkEmbedded failed, got %d", len(*upserts))
	}
}

// --- caching --------------------------------------------------------------

func TestServiceProcessItem_StrategyCachedAcrossCalls(t *testing.T) {
	svc, repo, _, reg, _, _ := newSvc(t)
	repo.markInFlightAttempt = 1

	for i := 0; i < 3; i++ {
		_, _ = svc.ProcessItem(context.Background(), 7)
	}
	if reg.calls != 1 {
		t.Errorf("expected registry built once across 3 calls, got %d", reg.calls)
	}
}

func TestServiceProcessItem_EnsureCollectionsCachedAcrossCalls(t *testing.T) {
	svc, repo, _, _, _, _ := newSvc(t)
	repo.markInFlightAttempt = 1

	ensureCalls := 0
	svc.ensureCollFn = func(_ context.Context, _ string, _ uint64, _ string) error {
		ensureCalls++
		return nil
	}

	for i := 0; i < 3; i++ {
		_, _ = svc.ProcessItem(context.Background(), 7)
	}
	if ensureCalls != 1 {
		t.Errorf("expected EnsureDenseCollections called once across 3 calls, got %d", ensureCalls)
	}
}

func TestProcessOutcome_ShouldAck(t *testing.T) {
	cases := []struct {
		out  ProcessOutcome
		want bool
	}{
		{OutcomeEmbedded, true},
		{OutcomeDeadLetter, true},
		{OutcomeSkipped, true},
		{OutcomeFailed, false},
	}
	for _, c := range cases {
		if got := c.out.ShouldAck(); got != c.want {
			t.Errorf("outcome=%v ShouldAck(): got %v, want %v", c.out, got, c.want)
		}
	}
}
