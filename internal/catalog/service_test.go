package catalog

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/jarviisha/codohue/internal/core/namespace"
	"github.com/jarviisha/codohue/pkg/codohuetypes"
)

// fakeRepo records calls and returns canned values.
type fakeRepo struct {
	res      *UpsertResult
	err      error
	called   int
	lastNS   string
	lastObj  string
	lastHash []byte

	listRows   []ObjectRow
	listTotal  int
	listErr    error
	lastSince  *time.Time
	lastLimit  int
	lastOffset int
}

func (f *fakeRepo) Upsert(_ context.Context, ns, obj, _ string, hash []byte, _ map[string]any) (*UpsertResult, error) {
	f.called++
	f.lastNS = ns
	f.lastObj = obj
	f.lastHash = hash
	return f.res, f.err
}

func (f *fakeRepo) ListObjects(_ context.Context, _ string, since *time.Time, limit, offset int) ([]ObjectRow, int, error) {
	f.lastSince = since
	f.lastLimit = limit
	f.lastOffset = offset
	return f.listRows, f.listTotal, f.listErr
}

// fakeAuthorWriter captures the write-through into the objects domain.
type fakeAuthorWriter struct {
	calls []string // "ns/object/author" per call
	err   error
}

func (f *fakeAuthorWriter) SetAuthor(_ context.Context, ns, obj, author string) error {
	f.calls = append(f.calls, ns+"/"+obj+"/"+author)
	return f.err
}

// fakeNSConfig returns canned namespace configs.
type fakeNSConfig struct {
	cfg *namespace.Config
	err error
}

func (f *fakeNSConfig) Get(_ context.Context, _ string) (*namespace.Config, error) {
	return f.cfg, f.err
}

// fakeXAdder records every XAdd call.
type fakeXAdder struct {
	calls []*redis.XAddArgs
	err   error
}

func (f *fakeXAdder) XAdd(_ context.Context, args *redis.XAddArgs) *redis.StringCmd {
	f.calls = append(f.calls, args)
	cmd := redis.NewStringCmd(context.Background(), "XADD")
	if f.err != nil {
		cmd.SetErr(f.err)
	} else {
		cmd.SetVal("0-1")
	}
	return cmd
}

// helpers ------------------------------------------------------------------

func enabledCfg() *namespace.Config {
	return &namespace.Config{
		Namespace:              "ns",
		DenseSource:            "catalog",
		CatalogStrategyID:      "internal-hashing-ngrams",
		CatalogStrategyVersion: "v1",
		CatalogMaxContentBytes: 32768,
		EmbeddingDim:           128,
	}
}

func newSvc(repo catalogRepository, nsCfg nsConfigGetter, pub xAdder) *Service {
	return &Service{
		repo:        repo,
		nsConfigSvc: nsCfg,
		publisher:   pub,
		clock:       func() time.Time { return time.Date(2026, 5, 9, 0, 0, 0, 0, time.UTC) },
	}
}

// tests --------------------------------------------------------------------

func TestServiceIngest_RejectsEmptyNamespace(t *testing.T) {
	svc := newSvc(&fakeRepo{}, &fakeNSConfig{}, &fakeXAdder{})
	_, err := svc.Ingest(context.Background(), "", &IngestRequest{ObjectID: "o1", Content: "x"})
	if !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("expected ErrInvalidRequest, got %v", err)
	}
}

func TestServiceIngest_RejectsNilRequest(t *testing.T) {
	svc := newSvc(&fakeRepo{}, &fakeNSConfig{}, &fakeXAdder{})
	_, err := svc.Ingest(context.Background(), "ns", nil)
	if !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("expected ErrInvalidRequest, got %v", err)
	}
}

func TestServiceIngest_RejectsMissingObjectID(t *testing.T) {
	svc := newSvc(&fakeRepo{}, &fakeNSConfig{}, &fakeXAdder{})
	_, err := svc.Ingest(context.Background(), "ns", &IngestRequest{Content: "hello"})
	if !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("expected ErrInvalidRequest, got %v", err)
	}
}

func TestServiceIngest_RejectsEmptyContent(t *testing.T) {
	svc := newSvc(&fakeRepo{}, &fakeNSConfig{}, &fakeXAdder{})
	for _, c := range []string{"", "   ", "\t\n  "} {
		_, err := svc.Ingest(context.Background(), "ns", &IngestRequest{ObjectID: "o1", Content: c})
		if !errors.Is(err, ErrEmptyContent) {
			t.Errorf("content=%q: expected ErrEmptyContent, got %v", c, err)
		}
	}
}

func TestServiceIngest_NamespaceNotFound(t *testing.T) {
	svc := newSvc(&fakeRepo{}, &fakeNSConfig{cfg: nil}, &fakeXAdder{})
	_, err := svc.Ingest(context.Background(), "ns", &IngestRequest{ObjectID: "o1", Content: "hi"})
	if !errors.Is(err, ErrNamespaceNotFound) {
		t.Fatalf("expected ErrNamespaceNotFound, got %v", err)
	}
}

func TestServiceIngest_NamespaceNotEnabled(t *testing.T) {
	cfg := enabledCfg()
	cfg.DenseSource = "disabled"
	svc := newSvc(&fakeRepo{}, &fakeNSConfig{cfg: cfg}, &fakeXAdder{})
	_, err := svc.Ingest(context.Background(), "ns", &IngestRequest{ObjectID: "o1", Content: "hi"})
	if !errors.Is(err, ErrNamespaceNotEnabled) {
		t.Fatalf("expected ErrNamespaceNotEnabled, got %v", err)
	}
}

func TestServiceIngest_NamespaceConfigError(t *testing.T) {
	svc := newSvc(&fakeRepo{}, &fakeNSConfig{err: errors.New("db down")}, &fakeXAdder{})
	_, err := svc.Ingest(context.Background(), "ns", &IngestRequest{ObjectID: "o1", Content: "hi"})
	if err == nil || errors.Is(err, ErrNamespaceNotFound) {
		t.Fatalf("expected wrapped DB error, got %v", err)
	}
}

func TestServiceIngest_ContentTooLarge(t *testing.T) {
	cfg := enabledCfg()
	cfg.CatalogMaxContentBytes = 10
	svc := newSvc(&fakeRepo{}, &fakeNSConfig{cfg: cfg}, &fakeXAdder{})
	_, err := svc.Ingest(context.Background(), "ns", &IngestRequest{
		ObjectID: "o1",
		Content:  strings.Repeat("x", 11),
	})
	if !errors.Is(err, ErrContentTooLarge) {
		t.Fatalf("expected ErrContentTooLarge, got %v", err)
	}
}

// Attribution no longer lands on catalog_items — it is written through to
// the objects domain so it works under every dense_source.
func TestServiceIngest_WritesAuthorThroughToObjects(t *testing.T) {
	writer := &fakeAuthorWriter{}
	svc := newSvc(&fakeRepo{res: &UpsertResult{Item: &Item{ID: 1}}}, &fakeNSConfig{cfg: enabledCfg()}, &fakeXAdder{})
	svc.SetAuthorWriter(writer)

	if _, err := svc.Ingest(context.Background(), "ns", &IngestRequest{
		ObjectID: "o1", Content: "hello", AuthorSubjectID: "u1",
	}); err != nil {
		t.Fatalf("Ingest: %v", err)
	}
	if len(writer.calls) != 1 || writer.calls[0] != "ns/o1/u1" {
		t.Errorf("author write-through: got %v", writer.calls)
	}
}

// Omitting the author must not call through at all — absence means
// "unspecified", not "clear whatever the objects endpoint set".
func TestServiceIngest_NoAuthorSkipsWriteThrough(t *testing.T) {
	writer := &fakeAuthorWriter{}
	svc := newSvc(&fakeRepo{res: &UpsertResult{Item: &Item{ID: 1}}}, &fakeNSConfig{cfg: enabledCfg()}, &fakeXAdder{})
	svc.SetAuthorWriter(writer)

	if _, err := svc.Ingest(context.Background(), "ns", &IngestRequest{
		ObjectID: "o1", Content: "hello",
	}); err != nil {
		t.Fatalf("Ingest: %v", err)
	}
	if len(writer.calls) != 0 {
		t.Errorf("expected no write-through, got %v", writer.calls)
	}
}

// The catalog row is already durable when attribution is attempted, so a
// failure there must not fail the ingest.
func TestServiceIngest_AuthorWriteFailureIsNotFatal(t *testing.T) {
	writer := &fakeAuthorWriter{err: errors.New("objects table down")}
	svc := newSvc(&fakeRepo{res: &UpsertResult{Item: &Item{ID: 1}}}, &fakeNSConfig{cfg: enabledCfg()}, &fakeXAdder{})
	svc.SetAuthorWriter(writer)

	if _, err := svc.Ingest(context.Background(), "ns", &IngestRequest{
		ObjectID: "o1", Content: "hello", AuthorSubjectID: "u1",
	}); err != nil {
		t.Fatalf("ingest must survive an attribution failure, got: %v", err)
	}
}

func TestServiceIngest_HappyPath_PublishesToStream(t *testing.T) {
	cfg := enabledCfg()
	repo := &fakeRepo{res: &UpsertResult{
		Item: &Item{
			ID: 7, Namespace: "ns", ObjectID: "o1", State: StatePending,
		},
		NeedsPublish: true,
	}}
	pub := &fakeXAdder{}
	svc := newSvc(repo, &fakeNSConfig{cfg: cfg}, pub)

	item, err := svc.Ingest(context.Background(), "ns", &IngestRequest{
		ObjectID: "o1",
		Content:  "Hôm nay trời đẹp",
	})
	if err != nil {
		t.Fatalf("Ingest: %v", err)
	}
	if item.ID != 7 {
		t.Errorf("item.ID: got %d, want 7", item.ID)
	}
	if repo.called != 1 {
		t.Errorf("repo called %d times", repo.called)
	}
	if repo.lastNS != "ns" || repo.lastObj != "o1" {
		t.Errorf("repo args: ns=%s obj=%s", repo.lastNS, repo.lastObj)
	}
	if len(pub.calls) != 1 {
		t.Fatalf("expected 1 XAdd call, got %d", len(pub.calls))
	}
	xa := pub.calls[0]
	if xa.Stream != "catalog:embed:ns" {
		t.Errorf("stream: got %q", xa.Stream)
	}
	v, ok := xa.Values.(map[string]any)
	if !ok {
		t.Fatalf("values: got %T", xa.Values)
	}
	if v["catalog_item_id"] != int64(7) {
		t.Errorf("catalog_item_id: got %v", v["catalog_item_id"])
	}
	if v["namespace"] != "ns" {
		t.Errorf("namespace: got %v", v["namespace"])
	}
	if v["object_id"] != "o1" {
		t.Errorf("object_id: got %v", v["object_id"])
	}
	if v["strategy_id"] != "internal-hashing-ngrams" {
		t.Errorf("strategy_id: got %v", v["strategy_id"])
	}
	if v["strategy_version"] != "v1" {
		t.Errorf("strategy_version: got %v", v["strategy_version"])
	}
	if v["enqueued_at"] != "2026-05-09T00:00:00Z" {
		t.Errorf("enqueued_at: got %v", v["enqueued_at"])
	}
}

func TestServiceIngest_IdempotentDoesNotPublish(t *testing.T) {
	cfg := enabledCfg()
	repo := &fakeRepo{res: &UpsertResult{
		Item: &Item{
			ID: 7, Namespace: "ns", ObjectID: "o1", State: StateEmbedded,
		},
		NeedsPublish: false,
	}}
	pub := &fakeXAdder{}
	svc := newSvc(repo, &fakeNSConfig{cfg: cfg}, pub)

	item, err := svc.Ingest(context.Background(), "ns", &IngestRequest{
		ObjectID: "o1",
		Content:  "hello world",
	})
	if err != nil {
		t.Fatalf("Ingest: %v", err)
	}
	if item.State != StateEmbedded {
		t.Errorf("state: got %s", item.State)
	}
	if len(pub.calls) != 0 {
		t.Errorf("expected no XAdd on idempotent re-ingest, got %d calls", len(pub.calls))
	}
}

func TestServiceIngest_RepoErrorPropagates(t *testing.T) {
	cfg := enabledCfg()
	repo := &fakeRepo{err: errors.New("db error")}
	svc := newSvc(repo, &fakeNSConfig{cfg: cfg}, &fakeXAdder{})

	_, err := svc.Ingest(context.Background(), "ns", &IngestRequest{ObjectID: "o1", Content: "hi"})
	if err == nil {
		t.Fatal("expected error")
	}
	if errors.Is(err, ErrNamespaceNotEnabled) || errors.Is(err, ErrInvalidRequest) {
		t.Errorf("expected non-sentinel wrapped error, got %v", err)
	}
}

func TestServiceIngest_PublishFailureSurfaceErrorButDoesNotRollBack(t *testing.T) {
	// The row is already committed by the time XAdd runs. A publish failure
	// MUST be surfaced so observability sees it, but the row must not be
	// rolled back — the recovery sweep will pick it up.
	cfg := enabledCfg()
	repo := &fakeRepo{res: &UpsertResult{
		Item:         &Item{ID: 7, Namespace: "ns", ObjectID: "o1", State: StatePending},
		NeedsPublish: true,
	}}
	pub := &fakeXAdder{err: errors.New("redis down")}
	svc := newSvc(repo, &fakeNSConfig{cfg: cfg}, pub)

	item, err := svc.Ingest(context.Background(), "ns", &IngestRequest{ObjectID: "o1", Content: "hi"})
	if err == nil {
		t.Fatal("expected publish error to surface")
	}
	if item == nil || item.ID != 7 {
		t.Errorf("expected item to still be returned despite publish failure, got %+v", item)
	}
}

func TestServiceIngest_ZeroMaxContentBytesMeansNoCheck(t *testing.T) {
	// CatalogMaxContentBytes=0 means "use default at higher level"; service
	// must not enforce against zero (which would reject everything).
	cfg := enabledCfg()
	cfg.CatalogMaxContentBytes = 0
	repo := &fakeRepo{res: &UpsertResult{
		Item:         &Item{ID: 1, Namespace: "ns", ObjectID: "o1", State: StatePending},
		NeedsPublish: true,
	}}
	svc := newSvc(repo, &fakeNSConfig{cfg: cfg}, &fakeXAdder{})

	_, err := svc.Ingest(context.Background(), "ns", &IngestRequest{
		ObjectID: "o1",
		Content:  strings.Repeat("x", 1<<16),
	})
	if err != nil {
		t.Fatalf("expected no size check when limit=0, got %v", err)
	}
}

func TestStreamName(t *testing.T) {
	if got := streamName("foo"); got != "catalog:embed:foo" {
		t.Errorf("streamName(foo): %q", got)
	}
}

func TestServiceIngestBatch_PerItemResultsAndCounts(t *testing.T) {
	repo := &fakeRepo{res: &UpsertResult{Item: &Item{ID: 1, Namespace: "ns", ObjectID: "o1"}, NeedsPublish: false}}
	svc := newSvc(repo, &fakeNSConfig{cfg: enabledCfg()}, &fakeXAdder{})

	resp, err := svc.IngestBatch(context.Background(), "ns", &BatchIngestRequest{Items: []IngestRequest{
		{ObjectID: "o1", Content: "hello"},
		{ObjectID: "o2", Content: "   "}, // empty after trim → per-item rejection
		{ObjectID: "", Content: "hi"},    // missing object_id → per-item rejection
	}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Accepted != 1 || resp.Rejected != 2 || len(resp.Results) != 3 {
		t.Fatalf("counts wrong: %+v", resp)
	}
	if !resp.Results[0].Accepted || resp.Results[0].Error != "" {
		t.Fatalf("first item should be accepted: %+v", resp.Results[0])
	}
	if resp.Results[1].Error != "empty_content" || resp.Results[2].Error != "invalid_request" {
		t.Fatalf("per-item codes wrong: %+v", resp.Results)
	}
}

func TestServiceIngestBatch_NamespaceErrorAbortsWholeBatch(t *testing.T) {
	svc := newSvc(&fakeRepo{}, &fakeNSConfig{cfg: nil}, &fakeXAdder{})
	_, err := svc.IngestBatch(context.Background(), "ghost", &BatchIngestRequest{Items: []IngestRequest{
		{ObjectID: "o1", Content: "hello"},
	}})
	if !errors.Is(err, ErrNamespaceNotFound) {
		t.Fatalf("expected ErrNamespaceNotFound, got %v", err)
	}
}

func TestServiceIngestBatch_CapAndEmptyRejected(t *testing.T) {
	svc := newSvc(&fakeRepo{}, &fakeNSConfig{cfg: enabledCfg()}, &fakeXAdder{})
	if _, err := svc.IngestBatch(context.Background(), "ns", &BatchIngestRequest{}); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("empty batch must be ErrInvalidRequest, got %v", err)
	}
	over := make([]IngestRequest, codohuetypes.CatalogBatchMaxItems+1)
	for i := range over {
		over[i] = IngestRequest{ObjectID: "o", Content: "c"}
	}
	if _, err := svc.IngestBatch(context.Background(), "ns", &BatchIngestRequest{Items: over}); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("oversized batch must be ErrInvalidRequest, got %v", err)
	}
}

func TestServiceListObjects_MapsRows(t *testing.T) {
	repo := &fakeRepo{
		listRows:  []ObjectRow{{ObjectID: "o1", UpdatedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)}},
		listTotal: 7,
	}
	svc := newSvc(repo, &fakeNSConfig{cfg: enabledCfg()}, &fakeXAdder{})
	since := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	resp, err := svc.ListObjects(context.Background(), "ns", &since, 50, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Total != 7 || resp.Limit != 50 || resp.Offset != 10 {
		t.Fatalf("paging meta wrong: %+v", resp)
	}
	if len(resp.Items) != 1 || resp.Items[0].ObjectID != "o1" || resp.Items[0].UpdatedAt != "2026-01-02T03:04:05Z" {
		t.Fatalf("items wrong: %+v", resp.Items)
	}
	if repo.lastSince == nil || !repo.lastSince.Equal(since) || repo.lastLimit != 50 || repo.lastOffset != 10 {
		t.Fatalf("repo args wrong: since=%v limit=%d offset=%d", repo.lastSince, repo.lastLimit, repo.lastOffset)
	}
}

func TestServiceListObjects_NamespaceNotFound(t *testing.T) {
	svc := newSvc(&fakeRepo{}, &fakeNSConfig{cfg: nil}, &fakeXAdder{})
	if _, err := svc.ListObjects(context.Background(), "ghost", nil, 100, 0); !errors.Is(err, ErrNamespaceNotFound) {
		t.Fatalf("expected ErrNamespaceNotFound, got %v", err)
	}
}

func TestItemErrorCode_Mapping(t *testing.T) {
	cases := map[error]string{
		ErrEmptyContent:                  "empty_content",
		ErrContentTooLarge:               "content_too_large",
		ErrInvalidRequest:                "invalid_request",
		errors.New("something exploded"): "internal_error",
	}
	for err, want := range cases {
		if got := itemErrorCode(err); got != want {
			t.Errorf("itemErrorCode(%v) = %q, want %q", err, got, want)
		}
	}
}

func TestNewServiceAndSetters(t *testing.T) {
	svc := NewService(nil, &fakeNSConfig{}, &fakeXAdder{})
	svc.SetDefaultMaxContentBytes(1024)
	if svc.defaultMaxContentBytes != 1024 {
		t.Fatalf("default max content bytes not wired: %d", svc.defaultMaxContentBytes)
	}
	if NewHandler(svc) == nil {
		t.Fatal("NewHandler returned nil")
	}
}
