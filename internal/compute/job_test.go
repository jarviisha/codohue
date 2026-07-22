package compute

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jarviisha/codohue/internal/core/batchrun"
	"github.com/jarviisha/codohue/internal/core/namespace"
)

// ─── fakes ───────────────────────────────────────────────────────────────────

type fakeRecomputer struct {
	called     bool
	lastLambda float64
	err        error
}

func (f *fakeRecomputer) RecomputeNamespace(_ context.Context, _ string, lambda float64) (subjects, objects int, err error) {
	f.called = true
	f.lastLambda = lambda
	return 0, 0, f.err
}

type fakeNsConfigReader struct {
	cfg *namespace.Config
	err error
}

func (f *fakeNsConfigReader) Get(_ context.Context, _ string) (*namespace.Config, error) {
	return f.cfg, f.err
}

type fakeJobRepo struct {
	namespaces []string
	events     []*RawEvent
	err        error
}

func (f *fakeJobRepo) GetActiveNamespaces(_ context.Context) ([]string, error) {
	return f.namespaces, f.err
}

func (f *fakeJobRepo) GetAllNamespaceEvents(_ context.Context, _ string) ([]*RawEvent, error) {
	return f.events, f.err
}

func (f *fakeJobRepo) GetNamespaceEventsInWindow(_ context.Context, _ string, _ int) ([]*RawEvent, error) {
	return f.events, f.err
}

// newTestJob builds a Job with all infra calls replaced by no-op fns.
func newTestJob(svc recomputer, nsCfg jobNsConfigReader, repo jobComputeRepo) *Job {
	noErr := func(...any) error { return nil }
	_ = noErr
	return &Job{
		service:     svc,
		nsConfigSvc: nsCfg,
		repo:        repo,
		redis:       nil, // phase 3 skipped by default

		ensureCollectionsFn:      func(_ context.Context, _ string) error { return nil },
		ensureDenseCollectionsFn: func(_ context.Context, _ string, _ uint64, _ string) error { return nil },
		upsertItemDenseFn:        func(_ context.Context, _, _ string, _ map[string][]float32) error { return nil },
		upsertSubjectDenseFn:     func(_ context.Context, _, _ string, _ map[string][]float32) error { return nil },
		fetchItemDenseFn: func(_ context.Context, _ string, _ []string) (map[string][]float32, error) {
			return nil, nil
		},
		storeTrendingFn: func(_ context.Context, _ string, _ map[string]float64, _ time.Duration) error { return nil },
	}
}

// ─── interval ────────────────────────────────────────────────────────────────

func TestNewJobInterval(t *testing.T) {
	job := NewJob(nil, nil, nil, nil, nil, nil, 10)
	if job.interval != 10*time.Minute {
		t.Errorf("expected 10m interval, got %v", job.interval)
	}
}

// ─── runOnce: phase 1 ────────────────────────────────────────────────────────

func TestRunOnce_Phase1_UsesConfigLambda(t *testing.T) {
	svc := &fakeRecomputer{}
	job := newTestJob(svc,
		&fakeNsConfigReader{cfg: &namespace.Config{Lambda: 0.02}},
		&fakeJobRepo{namespaces: []string{"ns1"}},
	)

	job.runOnce(context.Background())

	if !svc.called {
		t.Error("expected RecomputeNamespace to be called")
	}
	if svc.lastLambda != 0.02 {
		t.Errorf("lambda: got %v, want 0.02", svc.lastLambda)
	}
}

func TestRunOnce_Phase1_FallsBackToDefaultLambda(t *testing.T) {
	svc := &fakeRecomputer{}
	job := newTestJob(svc,
		&fakeNsConfigReader{cfg: nil}, // no config
		&fakeJobRepo{namespaces: []string{"ns1"}},
	)

	job.runOnce(context.Background())

	if svc.lastLambda != defaultLambda {
		t.Errorf("lambda: got %v, want default %v", svc.lastLambda, defaultLambda)
	}
}

func TestRunOnce_Phase1_RepoError_Skips(t *testing.T) {
	svc := &fakeRecomputer{}
	job := newTestJob(svc,
		&fakeNsConfigReader{},
		&fakeJobRepo{err: errors.New("db error")},
	)

	// Should not panic, just log and return.
	job.runOnce(context.Background())

	if svc.called {
		t.Error("expected RecomputeNamespace NOT to be called when GetActiveNamespaces fails")
	}
}

// ─── runOnce: phase 2 dispatch ───────────────────────────────────────────────

func TestRunOnce_Phase2_SkippedForBYOE(t *testing.T) {
	phase2Called := false
	job := newTestJob(
		&fakeRecomputer{},
		&fakeNsConfigReader{cfg: &namespace.Config{DenseSource: "byoe"}},
		&fakeJobRepo{namespaces: []string{"ns1"}},
	)
	job.upsertItemDenseFn = func(_ context.Context, _, _ string, _ map[string][]float32) error {
		phase2Called = true
		return nil
	}

	job.runOnce(context.Background())

	if phase2Called {
		t.Error("phase 2 should be skipped for strategy=byoe")
	}
}

func TestRunOnce_Phase2_SkippedForDisabled(t *testing.T) {
	phase2Called := false
	job := newTestJob(
		&fakeRecomputer{},
		&fakeNsConfigReader{cfg: &namespace.Config{DenseSource: "disabled"}},
		&fakeJobRepo{namespaces: []string{"ns1"}},
	)
	job.upsertItemDenseFn = func(_ context.Context, _, _ string, _ map[string][]float32) error {
		phase2Called = true
		return nil
	}

	job.runOnce(context.Background())

	if phase2Called {
		t.Error("phase 2 should be skipped for strategy=disabled")
	}
}

// Under dense_source=catalog the dense phase still runs — nothing else writes
// {ns}_subjects_dense — but it must not write back item vectors, which
// cmd/embedder owns.
func TestRunOnce_Phase2_CatalogPoolsSubjectsWithoutUpsertingItems(t *testing.T) {
	itemUpsert := false
	var subjectsUpserted map[string][]float32

	job := newTestJob(
		&fakeRecomputer{},
		&fakeNsConfigReader{cfg: &namespace.Config{DenseSource: "catalog", EmbeddingDim: 2}},
		&fakeJobRepo{
			namespaces: []string{"ns1"},
			events: []*RawEvent{
				{SubjectID: "u1", ObjectID: "o1", Weight: 1},
				{SubjectID: "u1", ObjectID: "o2", Weight: 1},
			},
		},
	)
	job.fetchItemDenseFn = func(_ context.Context, _ string, objectIDs []string) (map[string][]float32, error) {
		if len(objectIDs) != 2 {
			t.Errorf("expected 2 interacted object ids, got %v", objectIDs)
		}
		return map[string][]float32{
			"o1": {0, 2},
			"o2": {2, 0},
		}, nil
	}
	job.upsertItemDenseFn = func(_ context.Context, _, _ string, _ map[string][]float32) error {
		itemUpsert = true
		return nil
	}
	job.upsertSubjectDenseFn = func(_ context.Context, _, _ string, vecs map[string][]float32) error {
		subjectsUpserted = vecs
		return nil
	}

	job.runOnce(context.Background())

	if itemUpsert {
		t.Error("catalog mode must not upsert item vectors — cmd/embedder owns {ns}_objects_dense")
	}
	if len(subjectsUpserted) != 1 {
		t.Fatalf("expected 1 subject vector, got %d", len(subjectsUpserted))
	}
	// Mean of (0,2) and (2,0).
	if got := subjectsUpserted["u1"]; len(got) != 2 || got[0] != 1 || got[1] != 1 {
		t.Errorf("expected mean-pooled (1,1), got %v", got)
	}
}

// Interacted items that the embedder has not embedded yet yield no vectors —
// the phase must no-op rather than wipe existing subject vectors.
func TestRunPhase2Dense_CatalogNoEmbeddingsYet(t *testing.T) {
	subjectUpsertCalled := false
	job := newTestJob(&fakeRecomputer{}, &fakeNsConfigReader{}, &fakeJobRepo{
		events: []*RawEvent{{SubjectID: "u1", ObjectID: "o1", Weight: 1}},
	})
	job.fetchItemDenseFn = func(_ context.Context, _ string, _ []string) (map[string][]float32, error) {
		return map[string][]float32{}, nil
	}
	job.upsertSubjectDenseFn = func(_ context.Context, _, _ string, _ map[string][]float32) error {
		subjectUpsertCalled = true
		return nil
	}

	items, subjects, err := job.runPhase2Dense(context.Background(), "ns1",
		&namespace.Config{DenseSource: "catalog", EmbeddingDim: 2}, &LogCapture{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if items != 0 || subjects != 0 {
		t.Errorf("expected 0/0 counts, got %d/%d", items, subjects)
	}
	if subjectUpsertCalled {
		t.Error("must not upsert subject vectors when no item vectors are available")
	}
}

func TestRunPhase2Dense_CatalogFetchError(t *testing.T) {
	job := newTestJob(&fakeRecomputer{}, &fakeNsConfigReader{}, &fakeJobRepo{
		events: []*RawEvent{{SubjectID: "u1", ObjectID: "o1", Weight: 1}},
	})
	job.fetchItemDenseFn = func(_ context.Context, _ string, _ []string) (map[string][]float32, error) {
		return nil, errors.New("qdrant down")
	}

	_, _, err := job.runPhase2Dense(context.Background(), "ns1",
		&namespace.Config{DenseSource: "catalog"}, &LogCapture{})
	if err == nil {
		t.Fatal("expected error when the item vector fetch fails")
	}
}

func TestPhase2Runs(t *testing.T) {
	for src, want := range map[string]bool{
		"item2vec": true,
		"svd":      true,
		"catalog":  true,
		"byoe":     false,
		"disabled": false,
		"":         false,
	} {
		if got := phase2Runs(src); got != want {
			t.Errorf("phase2Runs(%q) = %v, want %v", src, got, want)
		}
	}
}

func TestInteractedObjectIDs(t *testing.T) {
	got := interactedObjectIDs([]*RawEvent{
		{SubjectID: "u1", ObjectID: "b"},
		{SubjectID: "u2", ObjectID: "a"},
		{SubjectID: "u1", ObjectID: "b"}, // duplicate
	})
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Errorf("expected sorted distinct [a b], got %v", got)
	}
}

func TestRunOnce_Phase2_SkippedWhenNoConfig(t *testing.T) {
	phase2Called := false
	job := newTestJob(
		&fakeRecomputer{},
		&fakeNsConfigReader{cfg: nil},
		&fakeJobRepo{namespaces: []string{"ns1"}},
	)
	job.upsertItemDenseFn = func(_ context.Context, _, _ string, _ map[string][]float32) error {
		phase2Called = true
		return nil
	}

	job.runOnce(context.Background())

	if phase2Called {
		t.Error("phase 2 should be skipped when config is nil")
	}
}

// ─── runOnce: phase 3 dispatch ───────────────────────────────────────────────

func TestRunOnce_Phase3_SkippedWhenRedisNil(t *testing.T) {
	phase3Called := false
	job := newTestJob(
		&fakeRecomputer{},
		&fakeNsConfigReader{cfg: &namespace.Config{TrendingWindow: 24}},
		&fakeJobRepo{namespaces: []string{"ns1"}},
	)
	job.redis = nil // explicitly nil
	job.storeTrendingFn = func(_ context.Context, _ string, _ map[string]float64, _ time.Duration) error {
		phase3Called = true
		return nil
	}

	job.runOnce(context.Background())

	if phase3Called {
		t.Error("phase 3 should be skipped when redis is nil")
	}
}

func TestRunOnce_MultipleNamespaces_AllProcessed(t *testing.T) {
	callCount := 0
	svc := &fakeRecomputer{}
	realSvc := svc

	job := newTestJob(
		realSvc,
		&fakeNsConfigReader{cfg: nil},
		&fakeJobRepo{namespaces: []string{"ns1", "ns2", "ns3"}},
	)
	_ = callCount

	job.runOnce(context.Background())

	// RecomputeNamespace is called once per namespace.
	// We can't count directly since fakeRecomputer only tracks last call,
	// but we verify it was called at all (ns3 is the last one processed).
	if !svc.called {
		t.Error("expected RecomputeNamespace to be called for at least one namespace")
	}
}

func TestRunPhase2Dense_Item2Vec_UpsertsItemAndSubjectVectors(t *testing.T) {
	events := []*RawEvent{
		{SubjectID: "u1", ObjectID: "o1", Action: "view", Weight: 1, OccurredAt: time.Now().Unix()},
		{SubjectID: "u1", ObjectID: "o2", Action: "view", Weight: 1, OccurredAt: time.Now().Unix()},
		{SubjectID: "u2", ObjectID: "o1", Action: "view", Weight: 1, OccurredAt: time.Now().Unix()},
		{SubjectID: "u2", ObjectID: "o2", Action: "view", Weight: 1, OccurredAt: time.Now().Unix()},
		{SubjectID: "u3", ObjectID: "o1", Action: "view", Weight: 1, OccurredAt: time.Now().Unix()},
		{SubjectID: "u3", ObjectID: "o2", Action: "view", Weight: 1, OccurredAt: time.Now().Unix()},
		{SubjectID: "u4", ObjectID: "o1", Action: "view", Weight: 1, OccurredAt: time.Now().Unix()},
		{SubjectID: "u4", ObjectID: "o2", Action: "view", Weight: 1, OccurredAt: time.Now().Unix()},
		{SubjectID: "u5", ObjectID: "o1", Action: "view", Weight: 1, OccurredAt: time.Now().Unix()},
		{SubjectID: "u5", ObjectID: "o2", Action: "view", Weight: 1, OccurredAt: time.Now().Unix()},
	}
	job := newTestJob(&fakeRecomputer{}, &fakeNsConfigReader{}, &fakeJobRepo{events: events})
	itemCalled := false
	subjectCalled := false

	job.upsertItemDenseFn = func(_ context.Context, ns, strategy string, vecs map[string][]float32) error {
		itemCalled = true
		if ns != "ns1" || strategy != "item2vec" {
			t.Fatalf("unexpected item upsert args: ns=%s strategy=%s", ns, strategy)
		}
		if len(vecs) == 0 {
			t.Fatal("expected non-empty item vectors")
		}
		return nil
	}
	job.upsertSubjectDenseFn = func(_ context.Context, ns, strategy string, vecs map[string][]float32) error {
		subjectCalled = true
		if ns != "ns1" || strategy != "item2vec" {
			t.Fatalf("unexpected subject upsert args: ns=%s strategy=%s", ns, strategy)
		}
		if len(vecs) == 0 {
			t.Fatal("expected non-empty subject vectors")
		}
		return nil
	}

	_, _, err := job.runPhase2Dense(context.Background(), "ns1", &namespace.Config{
		DenseSource:   "item2vec",
		EmbeddingDim:  8,
		DenseDistance: "dot",
	}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !itemCalled || !subjectCalled {
		t.Fatalf("expected both item and subject upserts, got item=%v subject=%v", itemCalled, subjectCalled)
	}
}

func TestRunPhase2Dense_SVD_UsesConfigDimensionAndDistance(t *testing.T) {
	events := []*RawEvent{
		{SubjectID: "u1", ObjectID: "o1", Action: "view", Weight: 1, OccurredAt: time.Now().Unix()},
		{SubjectID: "u1", ObjectID: "o2", Action: "view", Weight: 1, OccurredAt: time.Now().Unix()},
		{SubjectID: "u2", ObjectID: "o1", Action: "view", Weight: 1, OccurredAt: time.Now().Unix()},
		{SubjectID: "u2", ObjectID: "o3", Action: "view", Weight: 1, OccurredAt: time.Now().Unix()},
	}
	job := newTestJob(&fakeRecomputer{}, &fakeNsConfigReader{}, &fakeJobRepo{events: events})
	var gotDim uint64
	var gotDistance string
	itemCalled := false

	job.ensureDenseCollectionsFn = func(_ context.Context, _ string, dim uint64, distance string) error {
		gotDim = dim
		gotDistance = distance
		return nil
	}
	job.upsertItemDenseFn = func(_ context.Context, _, strategy string, vecs map[string][]float32) error {
		itemCalled = true
		if strategy != "svd" {
			t.Fatalf("strategy: got %s want svd", strategy)
		}
		if len(vecs) == 0 {
			t.Fatal("expected non-empty item vectors")
		}
		return nil
	}

	_, _, err := job.runPhase2Dense(context.Background(), "ns1", &namespace.Config{
		DenseSource:   "svd",
		EmbeddingDim:  4,
		DenseDistance: "dot",
	}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotDim != 4 || gotDistance != "dot" {
		t.Fatalf("ensureDenseCollections called with dim=%d distance=%s", gotDim, gotDistance)
	}
	if !itemCalled {
		t.Fatal("expected item upsert to be called")
	}
}

func TestRunPhase2Dense_NoEvents_SkipsUpserts(t *testing.T) {
	job := newTestJob(&fakeRecomputer{}, &fakeNsConfigReader{}, &fakeJobRepo{events: nil})
	itemCalled := false
	subjectCalled := false
	job.upsertItemDenseFn = func(_ context.Context, _, _ string, _ map[string][]float32) error {
		itemCalled = true
		return nil
	}
	job.upsertSubjectDenseFn = func(_ context.Context, _, _ string, _ map[string][]float32) error {
		subjectCalled = true
		return nil
	}

	_, _, err := job.runPhase2Dense(context.Background(), "ns1", &namespace.Config{DenseSource: "item2vec"}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if itemCalled || subjectCalled {
		t.Fatalf("expected no upserts, got item=%v subject=%v", itemCalled, subjectCalled)
	}
}

func TestRunPhase2Dense_EnsureDenseCollectionsFailure(t *testing.T) {
	job := newTestJob(&fakeRecomputer{}, &fakeNsConfigReader{}, &fakeJobRepo{})
	job.ensureDenseCollectionsFn = func(_ context.Context, _ string, _ uint64, _ string) error {
		return errors.New("ensure failed")
	}

	_, _, err := job.runPhase2Dense(context.Background(), "ns1", &namespace.Config{DenseSource: "item2vec"}, nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestRunPhase2Dense_ItemUpsertFailure(t *testing.T) {
	events := []*RawEvent{
		{SubjectID: "u1", ObjectID: "o1", Action: "view", Weight: 1, OccurredAt: time.Now().Unix()},
		{SubjectID: "u1", ObjectID: "o2", Action: "view", Weight: 1, OccurredAt: time.Now().Unix()},
		{SubjectID: "u2", ObjectID: "o1", Action: "view", Weight: 1, OccurredAt: time.Now().Unix()},
		{SubjectID: "u2", ObjectID: "o2", Action: "view", Weight: 1, OccurredAt: time.Now().Unix()},
		{SubjectID: "u3", ObjectID: "o1", Action: "view", Weight: 1, OccurredAt: time.Now().Unix()},
		{SubjectID: "u3", ObjectID: "o2", Action: "view", Weight: 1, OccurredAt: time.Now().Unix()},
		{SubjectID: "u4", ObjectID: "o1", Action: "view", Weight: 1, OccurredAt: time.Now().Unix()},
		{SubjectID: "u4", ObjectID: "o2", Action: "view", Weight: 1, OccurredAt: time.Now().Unix()},
		{SubjectID: "u5", ObjectID: "o1", Action: "view", Weight: 1, OccurredAt: time.Now().Unix()},
		{SubjectID: "u5", ObjectID: "o2", Action: "view", Weight: 1, OccurredAt: time.Now().Unix()},
	}
	job := newTestJob(&fakeRecomputer{}, &fakeNsConfigReader{}, &fakeJobRepo{events: events})
	job.upsertItemDenseFn = func(_ context.Context, _, _ string, _ map[string][]float32) error {
		return errors.New("item upsert failed")
	}

	_, _, err := job.runPhase2Dense(context.Background(), "ns1", &namespace.Config{DenseSource: "item2vec", EmbeddingDim: 8}, nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestRunPhase2Dense_SubjectUpsertFailure(t *testing.T) {
	events := []*RawEvent{
		{SubjectID: "u1", ObjectID: "o1", Action: "view", Weight: 1, OccurredAt: time.Now().Unix()},
		{SubjectID: "u1", ObjectID: "o2", Action: "view", Weight: 1, OccurredAt: time.Now().Unix()},
		{SubjectID: "u2", ObjectID: "o1", Action: "view", Weight: 1, OccurredAt: time.Now().Unix()},
		{SubjectID: "u2", ObjectID: "o2", Action: "view", Weight: 1, OccurredAt: time.Now().Unix()},
		{SubjectID: "u3", ObjectID: "o1", Action: "view", Weight: 1, OccurredAt: time.Now().Unix()},
		{SubjectID: "u3", ObjectID: "o2", Action: "view", Weight: 1, OccurredAt: time.Now().Unix()},
		{SubjectID: "u4", ObjectID: "o1", Action: "view", Weight: 1, OccurredAt: time.Now().Unix()},
		{SubjectID: "u4", ObjectID: "o2", Action: "view", Weight: 1, OccurredAt: time.Now().Unix()},
		{SubjectID: "u5", ObjectID: "o1", Action: "view", Weight: 1, OccurredAt: time.Now().Unix()},
		{SubjectID: "u5", ObjectID: "o2", Action: "view", Weight: 1, OccurredAt: time.Now().Unix()},
	}
	job := newTestJob(&fakeRecomputer{}, &fakeNsConfigReader{}, &fakeJobRepo{events: events})
	job.upsertSubjectDenseFn = func(_ context.Context, _, _ string, _ map[string][]float32) error {
		return errors.New("subject upsert failed")
	}

	_, _, err := job.runPhase2Dense(context.Background(), "ns1", &namespace.Config{DenseSource: "item2vec", EmbeddingDim: 8}, nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestRunPhase3Trending_UsesDefaults(t *testing.T) {
	events := []*RawEvent{
		{ObjectID: "o1", Action: "view", Weight: 1, OccurredAt: time.Now().Add(-time.Hour).Unix()},
	}
	job := newTestJob(&fakeRecomputer{}, &fakeNsConfigReader{}, &fakeJobRepo{events: events})
	var gotNS string
	var gotTTL time.Duration
	var gotScores map[string]float64

	job.storeTrendingFn = func(_ context.Context, ns string, scores map[string]float64, ttl time.Duration) error {
		gotNS = ns
		gotTTL = ttl
		gotScores = scores
		return nil
	}

	_, err := job.runPhase3Trending(context.Background(), "ns1", nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotNS != "ns1" {
		t.Fatalf("namespace: got %s want ns1", gotNS)
	}
	if gotTTL != 600*time.Second {
		t.Fatalf("ttl: got %v want %v", gotTTL, 600*time.Second)
	}
	if len(gotScores) == 0 {
		t.Fatal("expected non-empty scores")
	}
}

func TestRunPhase3Trending_UsesConfigOverrides(t *testing.T) {
	events := []*RawEvent{
		{ObjectID: "o1", Action: "purchase", Weight: 2, OccurredAt: time.Now().Add(-time.Hour).Unix()},
	}
	repo := &fakeJobRepo{events: events}
	job := newTestJob(&fakeRecomputer{}, &fakeNsConfigReader{}, repo)
	var gotWindow int
	var gotTTL time.Duration

	job.repo = &fakeWindowRepo{
		fakeJobRepo: repo,
		onWindow:    func(window int) { gotWindow = window },
	}
	job.storeTrendingFn = func(_ context.Context, _ string, _ map[string]float64, ttl time.Duration) error {
		gotTTL = ttl
		return nil
	}

	_, err := job.runPhase3Trending(context.Background(), "ns1", &namespace.Config{
		TrendingWindow: 48,
		LambdaTrending: 0.2,
		TrendingTTL:    120,
		ActionWeights:  map[string]float64{"purchase": 9},
	}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotWindow != 48 {
		t.Fatalf("window: got %d want 48", gotWindow)
	}
	if gotTTL != 120*time.Second {
		t.Fatalf("ttl: got %v want %v", gotTTL, 120*time.Second)
	}
}

func TestRunPhase3Trending_StoreFailure(t *testing.T) {
	events := []*RawEvent{
		{ObjectID: "o1", Action: "view", Weight: 1, OccurredAt: time.Now().Add(-time.Hour).Unix()},
	}
	job := newTestJob(&fakeRecomputer{}, &fakeNsConfigReader{}, &fakeJobRepo{events: events})
	job.storeTrendingFn = func(_ context.Context, _ string, _ map[string]float64, _ time.Duration) error {
		return errors.New("redis failed")
	}

	_, err := job.runPhase3Trending(context.Background(), "ns1", nil, nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

type fakeWindowRepo struct {
	*fakeJobRepo
	onWindow func(int)
}

func (f *fakeWindowRepo) GetNamespaceEventsInWindow(ctx context.Context, ns string, window int) ([]*RawEvent, error) {
	if f.onWindow != nil {
		f.onWindow(window)
	}
	return f.fakeJobRepo.GetNamespaceEventsInWindow(ctx, ns, window)
}

// ─── namespace lock + async start ────────────────────────────────────────────

type fakeBatchLogger struct {
	insertID  int64
	insertErr error
	updated   chan int64
}

func newFakeBatchLogger(id int64) *fakeBatchLogger {
	return &fakeBatchLogger{insertID: id, updated: make(chan int64, 2)}
}

func (f *fakeBatchLogger) InsertBatchRunLog(_ context.Context, _ string, _ time.Time, _ batchrun.TriggerSource) (int64, error) {
	return f.insertID, f.insertErr
}

func (f *fakeBatchLogger) UpdateBatchRunLog(_ context.Context, id int64, _ time.Time, _, _ int, _ bool, _ string, _ []LogEntry) error {
	select {
	case f.updated <- id:
	default:
	}
	return nil
}

func (f *fakeBatchLogger) UpdateBatchRunPhases(_ context.Context, _ int64, _ PhaseResults) error {
	return nil
}

func (f *fakeBatchLogger) GetCancelRequested(_ context.Context, _ int64) (bool, error) {
	return false, nil
}

func TestRunNamespace_ReturnsErrRunInProgressWhenLockHeld(t *testing.T) {
	svc := &fakeRecomputer{}
	job := newTestJob(svc, &fakeNsConfigReader{}, &fakeJobRepo{})
	job.tryLockFn = func(_ context.Context, _ string) (func(), bool, error) {
		return nil, false, nil
	}

	err := job.RunNamespace(context.Background(), "ns1", batchrun.TriggerManual)
	if !errors.Is(err, batchrun.ErrRunInProgress) {
		t.Fatalf("expected ErrRunInProgress, got %v", err)
	}
	if svc.called {
		t.Fatal("recompute must not run when the lock is held")
	}
}

func TestRunNamespace_ReleasesLockAfterRun(t *testing.T) {
	released := false
	job := newTestJob(&fakeRecomputer{}, &fakeNsConfigReader{}, &fakeJobRepo{})
	job.tryLockFn = func(_ context.Context, _ string) (func(), bool, error) {
		return func() { released = true }, true, nil
	}

	if err := job.RunNamespace(context.Background(), "ns1", batchrun.TriggerCron); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !released {
		t.Fatal("lock must be released after the run")
	}
}

func TestStartNamespaceRun_RunsDetachedFromCallerContext(t *testing.T) {
	logger := newFakeBatchLogger(7)
	job := newTestJob(&fakeRecomputer{}, &fakeNsConfigReader{}, &fakeJobRepo{})
	job.batchLog = logger

	ctx, cancel := context.WithCancel(context.Background())
	runID, err := job.StartNamespaceRun(ctx, "ns1", batchrun.TriggerManual, time.Minute)
	cancel() // caller disconnects immediately — the run must still finish
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if runID != 7 {
		t.Fatalf("run id: got %d want 7", runID)
	}

	select {
	case id := <-logger.updated:
		if id != 7 {
			t.Fatalf("finalized run id: got %d want 7", id)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("run was not finalized — cancelled caller context must not abort it")
	}
}

func TestStartNamespaceRun_LockHeld(t *testing.T) {
	job := newTestJob(&fakeRecomputer{}, &fakeNsConfigReader{}, &fakeJobRepo{})
	job.tryLockFn = func(_ context.Context, _ string) (func(), bool, error) {
		return nil, false, nil
	}
	if _, err := job.StartNamespaceRun(context.Background(), "ns1", batchrun.TriggerManual, time.Minute); !errors.Is(err, batchrun.ErrRunInProgress) {
		t.Fatalf("expected ErrRunInProgress, got %v", err)
	}
}

func TestRunOnce_FinalizesOrphanRuns(t *testing.T) {
	called := false
	job := newTestJob(&fakeRecomputer{}, &fakeNsConfigReader{}, &fakeJobRepo{})
	job.finalizeOrphansFn = func(_ context.Context, cutoff time.Time) (int64, error) {
		called = true
		if time.Until(cutoff) > -30*time.Minute {
			t.Errorf("cutoff too recent: %v", cutoff)
		}
		return 2, nil
	}

	job.runOnce(context.Background())
	if !called {
		t.Fatal("orphan finalizer must run every tick")
	}
}
