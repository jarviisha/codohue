package recommend

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	infraredis "github.com/jarviisha/codohue/internal/infra/redis"
	"github.com/jarviisha/codohue/internal/nsconfig"
	"github.com/qdrant/go-client/qdrant"
	"google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"
)

// ─── fakes ───────────────────────────────────────────────────────────────────

type fakeRepo struct {
	count        int
	countErr     error
	seenItems    []string
	seenItemsErr error
	popularItems []string
	popularErr   error
}

func (f *fakeRepo) CountInteractions(_ context.Context, _, _ string) (int, error) {
	return f.count, f.countErr
}

func (f *fakeRepo) GetSeenItems(_ context.Context, _, _ string, _ int) ([]string, error) {
	return f.seenItems, f.seenItemsErr
}

func (f *fakeRepo) GetPopularItems(_ context.Context, _ string, _ int) ([]string, error) {
	return f.popularItems, f.popularErr
}

type fakeNsConfig struct {
	cfg *nsconfig.NamespaceConfig
	err error
}

func (f *fakeNsConfig) Get(_ context.Context, _ string) (*nsconfig.NamespaceConfig, error) {
	return f.cfg, f.err
}

type fakeIDMapper struct {
	subjectID  uint64
	subjectErr error
	nextID     uint64
	ids        map[string]uint64
	objectErrs map[string]error
}

func newFakeIDMapper() *fakeIDMapper {
	return &fakeIDMapper{subjectID: 1, nextID: 10, ids: make(map[string]uint64), objectErrs: make(map[string]error)}
}

func (f *fakeIDMapper) GetOrCreateSubjectID(_ context.Context, _, _ string) (uint64, error) {
	return f.subjectID, f.subjectErr
}

func (f *fakeIDMapper) GetOrCreateObjectID(_ context.Context, id, _ string) (uint64, error) {
	if err, ok := f.objectErrs[id]; ok {
		return 0, err
	}
	if v, ok := f.ids[id]; ok {
		return v, nil
	}
	f.nextID++
	f.ids[id] = f.nextID
	return f.nextID, nil
}

// newTestService builds a Service with all infra replaced by no-ops / fakes.
func newTestService(repo recommendRepo, ns recommendNsConfig, idmap recommendIDMapper) *Service {
	s := &Service{
		repo:        repo,
		nsConfigSvc: ns,
		idmapSvc:    idmap,
		qdrant:      nil,
	}
	// Cache always misses by default.
	s.getCacheFn = func(_ context.Context, _ string) (string, error) {
		return "", errors.New("cache miss")
	}
	s.setCacheFn = func(_ context.Context, _, _ string, _ time.Duration) {}
	// Trending empty by default.
	s.getTrendingFn = func(_ context.Context, _ string, _, _ int) ([]infraredis.TrendingEntry, error) {
		return nil, nil
	}
	// No subject vector by default → CF falls back to popular.
	s.fetchSubjectVecFn = func(_ context.Context, _ string, _ uint64) (*qdrant.SparseVector, error) {
		return nil, nil
	}
	s.fetchSubjectDenseVecFn = func(_ context.Context, _ string, _ uint64) ([]float32, error) {
		return nil, nil
	}
	s.searchObjectsFn = func(_ context.Context, _ string, _ *qdrant.SparseVector, _ *qdrant.Filter, _ uint64) ([]*qdrant.ScoredPoint, error) {
		return nil, nil
	}
	s.searchObjectsDenseFn = func(_ context.Context, _ string, _ []float32, _ *qdrant.Filter, _ uint64) ([]*qdrant.ScoredPoint, error) {
		return nil, nil
	}
	s.deleteFromCollectionFn = func(_ context.Context, _ string, _ []*qdrant.PointId) error {
		return nil
	}
	s.qdrantGetFn = func(_ context.Context, _ *qdrant.GetPoints) ([]*qdrant.RetrievedPoint, error) {
		return nil, nil
	}
	s.qdrantSearchFn = func(_ context.Context, _ *qdrant.SearchPoints) ([]*qdrant.ScoredPoint, error) {
		return nil, nil
	}
	s.qdrantQueryFn = func(_ context.Context, _ *qdrant.QueryPoints) ([]*qdrant.ScoredPoint, error) {
		return nil, nil
	}
	s.qdrantUpsertFn = func(_ context.Context, _ *qdrant.UpsertPoints) error {
		return errors.New("qdrant error")
	}
	s.qdrantDeleteFn = func(_ context.Context, _ *qdrant.DeletePoints) error {
		return nil
	}
	// EnsureDenseCollections is a no-op by default; tests that reach qdrant get an error.
	s.ensureDenseCollectionsFn = func(_ context.Context, _ string, _ uint64, _ string) error {
		return errors.New("qdrant error")
	}
	return s
}

func TestNewService(t *testing.T) {
	svc := NewService(nil, nil, nil, nil, nil)

	if svc == nil {
		t.Fatal("expected non-nil service")
	}
	if svc.getCacheFn == nil || svc.setCacheFn == nil {
		t.Fatal("expected cache hooks to be initialized")
	}
	if svc.getTrendingFn == nil || svc.fetchSubjectVecFn == nil || svc.fetchSubjectDenseVecFn == nil {
		t.Fatal("expected query hooks to be initialized")
	}
	if svc.qdrantGetFn == nil || svc.qdrantSearchFn == nil || svc.qdrantQueryFn == nil || svc.qdrantUpsertFn == nil || svc.qdrantDeleteFn == nil {
		t.Fatal("expected qdrant hooks to be initialized")
	}
	if svc.ensureDenseCollectionsFn == nil || svc.deleteFromCollectionFn == nil {
		t.Fatal("expected collection hooks to be initialized")
	}
}

// ─── Recommend: cache hit ────────────────────────────────────────────────────

func TestRecommend_CacheHit(t *testing.T) {
	s := newTestService(&fakeRepo{}, &fakeNsConfig{}, newFakeIDMapper())
	s.getCacheFn = func(_ context.Context, _ string) (string, error) {
		return `{"subject_id":"u1","namespace":"ns","items":[{"object_id":"cached-item","score":0,"rank":1}],"source":"cf","limit":10,"offset":0,"total":1,"generated_at":"2024-01-01T00:00:00Z"}`, nil
	}

	resp, err := s.Recommend(context.Background(), &Request{SubjectID: "u1", Namespace: "ns", Limit: 10})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Items) != 1 || resp.Items[0].ObjectID != "cached-item" {
		t.Errorf("expected cached-item, got %v", resp.Items)
	}
}

// ─── doRecommend: cold start (count=0) ───────────────────────────────────────

func TestDoRecommend_ColdStart_NoTrending_FallsBackToPopular(t *testing.T) {
	repo := &fakeRepo{count: 0, popularItems: []string{"popular-1", "popular-2"}}
	s := newTestService(repo, &fakeNsConfig{}, newFakeIDMapper())
	// getTrendingFn returns empty → falls back to popular

	resp, err := s.Recommend(context.Background(), &Request{SubjectID: "u1", Namespace: "ns", Limit: 5})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Source != SourceFallbackPopular {
		t.Errorf("source: got %q, want %q", resp.Source, SourceFallbackPopular)
	}
	if len(resp.Items) == 0 {
		t.Error("expected popular items, got none")
	}
}

func TestDoRecommend_ColdStart_UsesTrendingCache(t *testing.T) {
	repo := &fakeRepo{count: 0}
	s := newTestService(repo, &fakeNsConfig{}, newFakeIDMapper())
	s.getTrendingFn = func(_ context.Context, _ string, _, _ int) ([]infraredis.TrendingEntry, error) {
		return []infraredis.TrendingEntry{
			{ObjectID: "trending-1", Score: 10.0},
			{ObjectID: "trending-2", Score: 8.0},
		}, nil
	}

	resp, err := s.Recommend(context.Background(), &Request{SubjectID: "u1", Namespace: "ns", Limit: 5})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Source != SourceFallbackPopular {
		t.Errorf("source: got %q, want %q", resp.Source, SourceFallbackPopular)
	}
	if len(resp.Items) != 2 || resp.Items[0].ObjectID != "trending-1" {
		t.Errorf("items: got %v", resp.Items)
	}
}

// ─── doRecommend: CF (count>=5) falls back to popular when no subject vector ─

func TestDoRecommend_CF_NoSubjectVector_FallsBackToPopular(t *testing.T) {
	repo := &fakeRepo{count: 10, popularItems: []string{"pop-1"}}
	s := newTestService(repo, &fakeNsConfig{}, newFakeIDMapper())
	// fetchSubjectVecFn returns nil by default → falls back to popular

	resp, err := s.Recommend(context.Background(), &Request{SubjectID: "u1", Namespace: "ns", Limit: 5})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Source != SourceFallbackPopular {
		t.Errorf("source: got %q, want %q", resp.Source, SourceFallbackPopular)
	}
}

func TestDoRecommend_CF_SubjectIDError_FallsBackToPopular(t *testing.T) {
	repo := &fakeRepo{count: 10, popularItems: []string{"pop-1"}}
	idmap := newFakeIDMapper()
	idmap.subjectErr = errors.New("idmap failure")
	s := newTestService(repo, &fakeNsConfig{}, idmap)

	resp, err := s.Recommend(context.Background(), &Request{SubjectID: "u1", Namespace: "ns", Limit: 5})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Source != SourceFallbackPopular {
		t.Errorf("source: got %q, want %q", resp.Source, SourceFallbackPopular)
	}
}

func TestCollaborativeFiltering_SeenItemsError_StillQueriesAndReturnsCF(t *testing.T) {
	repo := &fakeRepo{count: 10, seenItemsErr: errors.New("seen lookup failed")}
	s := newTestService(repo, &fakeNsConfig{cfg: &nsconfig.NamespaceConfig{Gamma: 0}}, newFakeIDMapper())
	s.fetchSubjectVecFn = func(_ context.Context, _ string, _ uint64) (*qdrant.SparseVector, error) {
		return &qdrant.SparseVector{Indices: []uint32{1}, Values: []float32{1}}, nil
	}
	s.searchObjectsFn = func(_ context.Context, _ string, _ *qdrant.SparseVector, filter *qdrant.Filter, _ uint64) ([]*qdrant.ScoredPoint, error) {
		if filter != nil {
			t.Fatalf("expected nil filter when seen-items lookup fails, got %#v", filter)
		}
		return []*qdrant.ScoredPoint{
			{Score: 3, Payload: map[string]*qdrant.Value{"object_id": qdrant.NewValueString("obj-1")}},
		}, nil
	}

	resp, err := s.Recommend(context.Background(), &Request{SubjectID: "u1", Namespace: "ns", Limit: 5})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Source != SourceCollaborativeFiltering {
		t.Fatalf("source: got %q, want %q", resp.Source, SourceCollaborativeFiltering)
	}
	if len(resp.Items) != 1 || resp.Items[0].ObjectID != "obj-1" {
		t.Fatalf("unexpected items: %v", resp.Items)
	}
}

func TestCollaborativeFiltering_UsesSeenItemsDaysFromConfig(t *testing.T) {
	repo := &fakeRepo{count: 10, seenItems: []string{"seen-1"}}
	idmap := newFakeIDMapper()
	var gotDays int
	s := newTestService(repo, &fakeNsConfig{cfg: &nsconfig.NamespaceConfig{SeenItemsDays: 14, Gamma: 0}}, idmap)
	s.fetchSubjectVecFn = func(_ context.Context, _ string, _ uint64) (*qdrant.SparseVector, error) {
		return &qdrant.SparseVector{Indices: []uint32{1}, Values: []float32{1}}, nil
	}
	s.repo = recommendRepo(&fakeRepoWithSeenDays{
		fakeRepo: repo,
		onSeen:   func(days int) { gotDays = days },
	})
	s.searchObjectsFn = func(_ context.Context, _ string, _ *qdrant.SparseVector, filter *qdrant.Filter, _ uint64) ([]*qdrant.ScoredPoint, error) {
		if filter == nil || len(filter.MustNot) == 0 {
			t.Fatal("expected seen-items filter to be built")
		}
		return []*qdrant.ScoredPoint{
			{Score: 3, Payload: map[string]*qdrant.Value{"object_id": qdrant.NewValueString("obj-1")}},
		}, nil
	}

	resp, err := s.Recommend(context.Background(), &Request{SubjectID: "u1", Namespace: "ns", Limit: 5})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotDays != 14 {
		t.Fatalf("GetSeenItems called with %d days, want 14", gotDays)
	}
	if resp.Source != SourceCollaborativeFiltering {
		t.Fatalf("source: got %q, want %q", resp.Source, SourceCollaborativeFiltering)
	}
}

// ─── doRecommend: popular error ──────────────────────────────────────────────

func TestDoRecommend_ColdStart_PopularError_ReturnsError(t *testing.T) {
	repo := &fakeRepo{count: 0, popularErr: errors.New("db error")}
	s := newTestService(repo, &fakeNsConfig{}, newFakeIDMapper())

	_, err := s.Recommend(context.Background(), &Request{SubjectID: "u1", Namespace: "ns", Limit: 5})
	if err == nil {
		t.Error("expected error when popular items fail, got nil")
	}
}

// ─── GetTrending: window resolution ─────────────────────────────────────────

func TestGetTrending_UsesParamWindow(t *testing.T) {
	s := newTestService(&fakeRepo{}, &fakeNsConfig{
		cfg: &nsconfig.NamespaceConfig{TrendingWindow: 48},
	}, newFakeIDMapper())

	resp, err := s.GetTrending(context.Background(), "ns", 10, 0, 12) // param=12 overrides config=48
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.WindowHours != 12 {
		t.Errorf("window: got %d, want 12", resp.WindowHours)
	}
}

func TestGetTrending_UsesConfigWindow(t *testing.T) {
	s := newTestService(&fakeRepo{}, &fakeNsConfig{
		cfg: &nsconfig.NamespaceConfig{TrendingWindow: 72},
	}, newFakeIDMapper())

	resp, err := s.GetTrending(context.Background(), "ns", 10, 0, 0) // param=0 → use config
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.WindowHours != 72 {
		t.Errorf("window: got %d, want 72", resp.WindowHours)
	}
}

func TestGetTrending_DefaultWindow(t *testing.T) {
	s := newTestService(&fakeRepo{}, &fakeNsConfig{cfg: nil}, newFakeIDMapper())

	resp, err := s.GetTrending(context.Background(), "ns", 10, 0, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.WindowHours != 24 {
		t.Errorf("window: got %d, want 24 (default)", resp.WindowHours)
	}
}

func TestGetTrending_ReturnsItems(t *testing.T) {
	s := newTestService(&fakeRepo{}, &fakeNsConfig{}, newFakeIDMapper())
	s.getTrendingFn = func(_ context.Context, _ string, _, _ int) ([]infraredis.TrendingEntry, error) {
		return []infraredis.TrendingEntry{
			{ObjectID: "item-1", Score: 9.5},
			{ObjectID: "item-2", Score: 7.0},
		}, nil
	}

	resp, err := s.GetTrending(context.Background(), "ns", 10, 0, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Items) != 2 {
		t.Errorf("expected 2 items, got %d", len(resp.Items))
	}
	if resp.Items[0].ObjectID != "item-1" || resp.Items[0].Score != 9.5 {
		t.Errorf("item[0]: got %+v", resp.Items[0])
	}
}

func TestGetTrending_NormalizesLimitAndOffset(t *testing.T) {
	s := newTestService(&fakeRepo{}, &fakeNsConfig{}, newFakeIDMapper())
	s.getTrendingFn = func(_ context.Context, _ string, offset, limit int) ([]infraredis.TrendingEntry, error) {
		if offset != 0 {
			t.Fatalf("expected normalized offset 0, got %d", offset)
		}
		if limit != 50 {
			t.Fatalf("expected normalized limit 50, got %d", limit)
		}
		return nil, nil
	}

	resp, err := s.GetTrending(context.Background(), "ns", 0, -3, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Items) != 0 {
		t.Fatalf("expected no items, got %v", resp.Items)
	}
}

func TestGetTrending_ConfigAndRedisErrorsStillReturnResponse(t *testing.T) {
	s := newTestService(&fakeRepo{}, &fakeNsConfig{err: errors.New("config failed")}, newFakeIDMapper())
	s.getTrendingFn = func(_ context.Context, _ string, _, _ int) ([]infraredis.TrendingEntry, error) {
		return nil, errors.New("redis failed")
	}

	resp, err := s.GetTrending(context.Background(), "ns", 10, 0, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.WindowHours != 24 {
		t.Fatalf("expected default window 24, got %d", resp.WindowHours)
	}
	if len(resp.Items) != 0 {
		t.Fatalf("expected empty items on redis error, got %v", resp.Items)
	}
}

// ─── storeEmbedding: dimension validation ────────────────────────────────────

func TestStoreEmbedding_DimMismatch_ReturnsError(t *testing.T) {
	s := newTestService(&fakeRepo{}, &fakeNsConfig{
		cfg: &nsconfig.NamespaceConfig{EmbeddingDim: 64},
	}, newFakeIDMapper())

	// Send a 128-dim vector when config expects 64.
	vector := make([]float32, 128)
	err := s.StoreObjectEmbedding(context.Background(), "ns", "obj-1", vector)
	if err == nil {
		t.Fatal("expected dim mismatch error, got nil")
	}
	if !isDimMismatch(err) {
		t.Errorf("expected dim mismatch error, got: %v", err)
	}
}

func TestStoreEmbedding_NsConfigError_ReturnsError(t *testing.T) {
	s := newTestService(&fakeRepo{}, &fakeNsConfig{
		err: errors.New("db error"),
	}, newFakeIDMapper())

	err := s.StoreObjectEmbedding(context.Background(), "ns", "obj-1", []float32{0.1, 0.2})
	if err == nil {
		t.Error("expected error from nsconfig.Get, got nil")
	}
}

func TestStoreEmbedding_NoDimConfig_NoDimCheck(t *testing.T) {
	// When config has EmbeddingDim=0, any dimension is accepted — no error before qdrant.
	s := newTestService(&fakeRepo{}, &fakeNsConfig{
		cfg: &nsconfig.NamespaceConfig{EmbeddingDim: 0},
	}, newFakeIDMapper())

	// We expect an error from qdrant (nil client), NOT from dim validation.
	err := s.StoreObjectEmbedding(context.Background(), "ns", "obj-1", []float32{0.1, 0.2})
	if isDimMismatch(err) {
		t.Error("expected NO dim mismatch error when EmbeddingDim=0, but got one")
	}
}

func TestStoreSubjectEmbedding_DimMismatch_ReturnsError(t *testing.T) {
	s := newTestService(&fakeRepo{}, &fakeNsConfig{
		cfg: &nsconfig.NamespaceConfig{EmbeddingDim: 2},
	}, newFakeIDMapper())

	err := s.StoreSubjectEmbedding(context.Background(), "ns", "sub-1", []float32{0.1, 0.2, 0.3})
	if err == nil {
		t.Fatal("expected dim mismatch error, got nil")
	}
	if !isDimMismatch(err) {
		t.Fatalf("expected dim mismatch error, got %v", err)
	}
}

func TestStoreEmbedding_Success(t *testing.T) {
	idmap := newFakeIDMapper()
	s := newTestService(&fakeRepo{}, &fakeNsConfig{
		cfg: &nsconfig.NamespaceConfig{EmbeddingDim: 3, DenseDistance: "dot"},
	}, idmap)
	s.ensureDenseCollectionsFn = func(_ context.Context, ns string, dim uint64, distance string) error {
		if ns != "ns" || dim != 3 || distance != "dot" {
			t.Fatalf("unexpected ensure args ns=%s dim=%d distance=%s", ns, dim, distance)
		}
		return nil
	}
	called := false
	s.qdrantUpsertFn = func(_ context.Context, points *qdrant.UpsertPoints) error {
		called = true
		if points.CollectionName != "ns_objects_dense" {
			t.Fatalf("unexpected collection: %s", points.CollectionName)
		}
		return nil
	}

	if err := s.StoreObjectEmbedding(context.Background(), "ns", "obj-1", []float32{0.1, 0.2, 0.3}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Fatal("expected qdrant upsert to be called")
	}
}

func TestStoreSubjectEmbedding_Success(t *testing.T) {
	idmap := newFakeIDMapper()
	s := newTestService(&fakeRepo{}, &fakeNsConfig{
		cfg: &nsconfig.NamespaceConfig{EmbeddingDim: 2},
	}, idmap)
	s.ensureDenseCollectionsFn = func(_ context.Context, ns string, dim uint64, distance string) error {
		if ns != "ns" || dim != 2 || distance != "cosine" {
			t.Fatalf("unexpected ensure args ns=%s dim=%d distance=%s", ns, dim, distance)
		}
		return nil
	}
	s.qdrantUpsertFn = func(_ context.Context, points *qdrant.UpsertPoints) error {
		if points.CollectionName != "ns_subjects_dense" {
			t.Fatalf("unexpected collection: %s", points.CollectionName)
		}
		if len(points.Points) != 1 {
			t.Fatalf("expected one point, got %d", len(points.Points))
		}
		return nil
	}

	if err := s.StoreSubjectEmbedding(context.Background(), "ns", "sub-1", []float32{0.1, 0.2}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestStoreEmbedding_EnsureDenseCollectionsError(t *testing.T) {
	s := newTestService(&fakeRepo{}, &fakeNsConfig{
		cfg: &nsconfig.NamespaceConfig{EmbeddingDim: 2},
	}, newFakeIDMapper())
	s.ensureDenseCollectionsFn = func(_ context.Context, _ string, _ uint64, _ string) error {
		return errors.New("ensure failed")
	}

	err := s.StoreObjectEmbedding(context.Background(), "ns", "obj-1", []float32{0.1, 0.2})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

type fakeRepoWithSeenDays struct {
	*fakeRepo
	onSeen func(days int)
}

func (f *fakeRepoWithSeenDays) GetSeenItems(ctx context.Context, namespace, subjectID string, days int) ([]string, error) {
	if f.onSeen != nil {
		f.onSeen(days)
	}
	return f.fakeRepo.GetSeenItems(ctx, namespace, subjectID, days)
}

// ─── rankFallback ────────────────────────────────────────────────────────────

func TestRankFallback(t *testing.T) {
	svc := &Service{}
	req := &RankRequest{
		SubjectID:  "user_a",
		Namespace:  "ns_feed",
		Candidates: []string{"post_1", "post_2", "post_3"},
	}

	resp := svc.rankFallback(req)

	if resp.Source != SourceHybridRank {
		t.Errorf("source = %q, want %q", resp.Source, SourceHybridRank)
	}
	if len(resp.Items) != len(req.Candidates) {
		t.Fatalf("items length = %d, want %d", len(resp.Items), len(req.Candidates))
	}
	for i, item := range resp.Items {
		if item.ObjectID != req.Candidates[i] {
			t.Errorf("items[%d].ObjectID = %q, want %q", i, item.ObjectID, req.Candidates[i])
		}
		if item.Score != 0 {
			t.Errorf("items[%d].Score = %f, want 0", i, item.Score)
		}
		if item.Rank != i+1 {
			t.Errorf("items[%d].Rank = %d, want %d", i, item.Rank, i+1)
		}
	}
}

func TestRankFallbackIsolation(t *testing.T) {
	svc := &Service{}
	req := &RankRequest{Candidates: []string{"post_x", "post_y"}}
	resp := svc.rankFallback(req)
	resp.Items[0] = RankedItem{ObjectID: "mutated"}
	if req.Candidates[0] == "mutated" {
		t.Error("rankFallback shares backing array with Candidates")
	}
}

// ─── pure helpers (kept for regression) ─────────────────────────────────────

func TestRerank(t *testing.T) {
	now := time.Now().UTC()
	old := now.Add(-100 * 24 * time.Hour)
	recent := now.Add(-1 * 24 * time.Hour)

	points := []*qdrant.ScoredPoint{
		{Score: 10.0, Payload: map[string]*qdrant.Value{
			"object_id":  qdrant.NewValueString("obj-old"),
			"created_at": qdrant.NewValueString(old.Format(time.RFC3339)),
		}},
		{Score: 5.0, Payload: map[string]*qdrant.Value{
			"object_id":  qdrant.NewValueString("obj-recent"),
			"created_at": qdrant.NewValueString(recent.Format(time.RFC3339)),
		}},
	}

	result := rerank(points, 0.02, 10)
	if len(result) != 2 {
		t.Fatalf("expected 2 results, got %d", len(result))
	}
	if result[0] != "obj-recent" {
		t.Errorf("expected obj-recent first, got %q", result[0])
	}
}

func TestRerankLimit(t *testing.T) {
	points := make([]*qdrant.ScoredPoint, 10)
	for i := range points {
		points[i] = &qdrant.ScoredPoint{
			Score:   float32(10 - i),
			Payload: map[string]*qdrant.Value{"object_id": qdrant.NewValueString("obj")},
		}
	}
	if len(rerank(points, 0, 3)) != 3 {
		t.Error("expected 3 results with limit=3")
	}
}

func TestRerankNoCreatedAt(t *testing.T) {
	points := []*qdrant.ScoredPoint{
		{Score: 7.0, Payload: map[string]*qdrant.Value{
			"object_id": qdrant.NewValueString("obj-no-time"),
		}},
	}
	result := rerank(points, 0.02, 10)
	if len(result) != 1 || result[0] != "obj-no-time" {
		t.Errorf("expected [obj-no-time], got %v", result)
	}
}

func TestBlendItems(t *testing.T) {
	popular := []string{"p1", "p2", "p3", "p4", "p5", "p6", "p7"}
	cf := []string{"c1", "c2", "c3", "p1", "c4"}

	result := blendItems(popular, cf, 0.7, 10)
	if len(result) > 10 {
		t.Errorf("result length %d exceeds limit 10", len(result))
	}
	seen := make(map[string]bool)
	for _, item := range result {
		if seen[item] {
			t.Errorf("duplicate item %q in blend result", item)
		}
		seen[item] = true
	}
}

func TestBlendItemsExactRatio(t *testing.T) {
	popular := make([]string, 20)
	cf := make([]string, 20)
	for i := range popular {
		popular[i] = "p" + string(rune('A'+i))
		cf[i] = "c" + string(rune('A'+i))
	}

	limit := 10
	result := blendItems(popular, cf, 0.7, limit)
	if len(result) != limit {
		t.Errorf("expected %d items, got %d", limit, len(result))
	}
	popularCount := 0
	for _, item := range result {
		if item[0] == 'p' {
			popularCount++
		}
	}
	wantPopular := int(math.Round(float64(limit) * 0.7))
	if popularCount != wantPopular {
		t.Errorf("expected %d popular items, got %d", wantPopular, popularCount)
	}
}

func TestNormalizeScores(t *testing.T) {
	scores := map[string]float64{"a": 10, "b": 0, "c": 5}
	norm := normalizeScores(scores)
	if math.Abs(norm["a"]-1.0) > 1e-6 {
		t.Errorf("max item = %f, want ~1.0", norm["a"])
	}
	if math.Abs(norm["b"]-0.0) > 1e-4 {
		t.Errorf("min item = %f, want ~0.0", norm["b"])
	}
}

func TestNormalizeScoresAllEqual(t *testing.T) {
	norm := normalizeScores(map[string]float64{"x": 7, "y": 7, "z": 7})
	for id, v := range norm {
		if math.Abs(v-1.0) > 1e-6 {
			t.Errorf("%q = %f, want 1.0", id, v)
		}
	}
}

func TestNormalizeScoresEmpty(t *testing.T) {
	if result := normalizeScores(nil); len(result) != 0 {
		t.Error("expected empty result for nil input")
	}
}

func TestHybridRecommend_BlendsSparseAndDense(t *testing.T) {
	s := newTestService(&fakeRepo{}, &fakeNsConfig{}, newFakeIDMapper())
	s.searchObjectsFn = func(_ context.Context, _ string, _ *qdrant.SparseVector, _ *qdrant.Filter, _ uint64) ([]*qdrant.ScoredPoint, error) {
		return []*qdrant.ScoredPoint{
			{Score: 10, Payload: map[string]*qdrant.Value{"object_id": qdrant.NewValueString("obj-sparse"), "created_at": qdrant.NewValueString(time.Now().UTC().Format(time.RFC3339))}},
			{Score: 5, Payload: map[string]*qdrant.Value{"object_id": qdrant.NewValueString("obj-both"), "created_at": qdrant.NewValueString(time.Now().UTC().Format(time.RFC3339))}},
		}, nil
	}
	s.searchObjectsDenseFn = func(_ context.Context, _ string, _ []float32, _ *qdrant.Filter, _ uint64) ([]*qdrant.ScoredPoint, error) {
		return []*qdrant.ScoredPoint{
			{Score: 20, Payload: map[string]*qdrant.Value{"object_id": qdrant.NewValueString("obj-dense"), "created_at": qdrant.NewValueString(time.Now().UTC().Format(time.RFC3339))}},
			{Score: 15, Payload: map[string]*qdrant.Value{"object_id": qdrant.NewValueString("obj-both"), "created_at": qdrant.NewValueString(time.Now().UTC().Format(time.RFC3339))}},
		}, nil
	}

	resp, err := s.hybridRecommend(context.Background(), &Request{SubjectID: "u1", Namespace: "ns"}, 3, &nsconfig.NamespaceConfig{Alpha: 0.8, Gamma: 0}, &qdrant.SparseVector{}, []float32{1, 2}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Source != SourceHybrid {
		t.Fatalf("source: got %q want %q", resp.Source, SourceHybrid)
	}
	if len(resp.Items) != 3 {
		t.Fatalf("items length: got %d want 3", len(resp.Items))
	}
	if resp.Items[0].ObjectID != "obj-sparse" {
		t.Fatalf("expected obj-sparse first, got %v", resp.Items)
	}
}

func TestHybridRecommend_AppliesFreshnessDecay(t *testing.T) {
	s := newTestService(&fakeRepo{}, &fakeNsConfig{}, newFakeIDMapper())
	old := time.Now().UTC().Add(-30 * 24 * time.Hour).Format(time.RFC3339)
	now := time.Now().UTC().Format(time.RFC3339)
	s.searchObjectsFn = func(_ context.Context, _ string, _ *qdrant.SparseVector, _ *qdrant.Filter, _ uint64) ([]*qdrant.ScoredPoint, error) {
		return []*qdrant.ScoredPoint{
			{Score: 10, Payload: map[string]*qdrant.Value{"object_id": qdrant.NewValueString("old"), "created_at": qdrant.NewValueString(old)}},
			{Score: 10, Payload: map[string]*qdrant.Value{"object_id": qdrant.NewValueString("fresh"), "created_at": qdrant.NewValueString(now)}},
		}, nil
	}
	s.searchObjectsDenseFn = func(_ context.Context, _ string, _ []float32, _ *qdrant.Filter, _ uint64) ([]*qdrant.ScoredPoint, error) {
		return nil, nil
	}

	resp, err := s.hybridRecommend(context.Background(), &Request{SubjectID: "u1", Namespace: "ns"}, 2, &nsconfig.NamespaceConfig{Alpha: 1, Gamma: 0.2}, &qdrant.SparseVector{}, []float32{1}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Items[0].ObjectID != "fresh" {
		t.Fatalf("expected fresh item first after decay, got %v", resp.Items)
	}
}

func TestHybridRecommend_FallsBackWhenBothSearchesEmpty(t *testing.T) {
	s := newTestService(&fakeRepo{popularItems: []string{"popular-1"}}, &fakeNsConfig{}, newFakeIDMapper())
	s.searchObjectsFn = func(_ context.Context, _ string, _ *qdrant.SparseVector, _ *qdrant.Filter, _ uint64) ([]*qdrant.ScoredPoint, error) {
		return nil, nil
	}
	s.searchObjectsDenseFn = func(_ context.Context, _ string, _ []float32, _ *qdrant.Filter, _ uint64) ([]*qdrant.ScoredPoint, error) {
		return nil, nil
	}

	resp, err := s.hybridRecommend(context.Background(), &Request{SubjectID: "u1", Namespace: "ns"}, 2, &nsconfig.NamespaceConfig{Alpha: 0.5}, &qdrant.SparseVector{}, []float32{1}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Source != SourceFallbackPopular || resp.Items[0].ObjectID != "popular-1" {
		t.Fatalf("unexpected fallback response: %+v", resp)
	}
}

func TestBuildSeenItemsFilter_SkipsUnmappableIDs(t *testing.T) {
	idmap := newFakeIDMapper()
	idmap.objectErrs["seen-bad"] = errors.New("mapping failed")
	s := newTestService(&fakeRepo{}, &fakeNsConfig{}, idmap)

	filter := s.buildSeenItemsFilter(context.Background(), "ns", []string{"seen-good", "seen-bad"})
	if filter == nil {
		t.Fatal("expected non-nil filter")
	}
	if len(filter.MustNot) != 1 {
		t.Fatalf("MustNot length: got %d want 1", len(filter.MustNot))
	}
}

func TestExtractScores_IgnoresPointsWithoutObjectID(t *testing.T) {
	got := extractScores([]*qdrant.ScoredPoint{
		{Score: 5, Payload: map[string]*qdrant.Value{"object_id": qdrant.NewValueString("obj-1")}},
		{Score: 9, Payload: map[string]*qdrant.Value{}},
	})
	if len(got) != 1 || got["obj-1"] != 5 {
		t.Fatalf("unexpected scores: %+v", got)
	}
}

func TestBuildCreatedAtLookup_IgnoresBadTimestampAndDuplicates(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	got := buildCreatedAtLookup(
		[]*qdrant.ScoredPoint{
			{Payload: map[string]*qdrant.Value{"object_id": qdrant.NewValueString("obj-1"), "created_at": qdrant.NewValueString(now.Format(time.RFC3339))}},
			{Payload: map[string]*qdrant.Value{"object_id": qdrant.NewValueString("obj-2"), "created_at": qdrant.NewValueString("bad-time")}},
		},
		[]*qdrant.ScoredPoint{
			{Payload: map[string]*qdrant.Value{"object_id": qdrant.NewValueString("obj-1"), "created_at": qdrant.NewValueString(now.Add(-time.Hour).Format(time.RFC3339))}},
		},
	)
	if len(got) != 1 {
		t.Fatalf("unexpected lookup size: %d", len(got))
	}
	if !got["obj-1"].Equal(now) {
		t.Fatalf("unexpected created_at for obj-1: %v", got["obj-1"])
	}
}

func TestFetchSubjectVector_Success(t *testing.T) {
	s := newTestService(&fakeRepo{}, &fakeNsConfig{}, newFakeIDMapper())
	s.qdrantGetFn = func(_ context.Context, points *qdrant.GetPoints) ([]*qdrant.RetrievedPoint, error) {
		if points.CollectionName != "ns_subjects" {
			t.Fatalf("unexpected collection: %s", points.CollectionName)
		}
		return []*qdrant.RetrievedPoint{{
			Vectors: &qdrant.VectorsOutput{
				VectorsOptions: &qdrant.VectorsOutput_Vectors{
					Vectors: &qdrant.NamedVectorsOutput{
						Vectors: map[string]*qdrant.VectorOutput{
							sparseVectorName: {Vector: &qdrant.VectorOutput_Sparse{Sparse: &qdrant.SparseVector{Indices: []uint32{1}, Values: []float32{2}}}},
						},
					},
				},
			},
		}}, nil
	}

	vec, err := s.fetchSubjectVector(context.Background(), "ns", 7)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if vec == nil || len(vec.Indices) != 1 || vec.Indices[0] != 1 {
		t.Fatalf("unexpected vector: %+v", vec)
	}
}

func TestFetchSubjectDenseVector_Success(t *testing.T) {
	s := newTestService(&fakeRepo{}, &fakeNsConfig{}, newFakeIDMapper())
	s.qdrantGetFn = func(_ context.Context, points *qdrant.GetPoints) ([]*qdrant.RetrievedPoint, error) {
		if points.CollectionName != "ns_subjects_dense" {
			t.Fatalf("unexpected collection: %s", points.CollectionName)
		}
		return []*qdrant.RetrievedPoint{{
			Vectors: &qdrant.VectorsOutput{
				VectorsOptions: &qdrant.VectorsOutput_Vectors{
					Vectors: &qdrant.NamedVectorsOutput{
						Vectors: map[string]*qdrant.VectorOutput{
							denseVectorName: {Vector: &qdrant.VectorOutput_Dense{Dense: &qdrant.DenseVector{Data: []float32{0.1, 0.2}}}},
						},
					},
				},
			},
		}}, nil
	}

	vec, err := s.fetchSubjectDenseVector(context.Background(), "ns", 7)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(vec) != 2 || vec[0] != 0.1 {
		t.Fatalf("unexpected vector: %+v", vec)
	}
}

func TestSearchObjectsDense_QueryError(t *testing.T) {
	s := newTestService(&fakeRepo{}, &fakeNsConfig{}, newFakeIDMapper())
	s.qdrantQueryFn = func(_ context.Context, _ *qdrant.QueryPoints) ([]*qdrant.ScoredPoint, error) {
		return nil, errors.New("query failed")
	}

	if _, err := s.searchObjectsDense(context.Background(), "ns", []float32{0.1}, nil, 5); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestSearchObjects_Success(t *testing.T) {
	s := newTestService(&fakeRepo{}, &fakeNsConfig{}, newFakeIDMapper())
	s.qdrantSearchFn = func(_ context.Context, points *qdrant.SearchPoints) ([]*qdrant.ScoredPoint, error) {
		if points.CollectionName != "ns_objects" {
			t.Fatalf("unexpected collection: %s", points.CollectionName)
		}
		if points.VectorName == nil || *points.VectorName != "sparse_interactions" {
			t.Fatalf("unexpected vector name: %#v", points.VectorName)
		}
		return []*qdrant.ScoredPoint{
			{Score: 1, Payload: map[string]*qdrant.Value{"object_id": qdrant.NewValueString("obj-1")}},
		}, nil
	}

	res, err := s.searchObjects(context.Background(), "ns", &qdrant.SparseVector{Indices: []uint32{1}, Values: []float32{2}}, nil, 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res) != 1 || res[0].Payload["object_id"].GetStringValue() != "obj-1" {
		t.Fatalf("unexpected results: %+v", res)
	}
}

func TestRank_UsesSearchResults(t *testing.T) {
	s := newTestService(&fakeRepo{}, &fakeNsConfig{cfg: &nsconfig.NamespaceConfig{Gamma: 0}}, newFakeIDMapper())
	s.fetchSubjectVecFn = func(_ context.Context, _ string, _ uint64) (*qdrant.SparseVector, error) {
		return &qdrant.SparseVector{Indices: []uint32{1}, Values: []float32{1}}, nil
	}
	s.searchObjectsFn = func(_ context.Context, _ string, _ *qdrant.SparseVector, filter *qdrant.Filter, topK uint64) ([]*qdrant.ScoredPoint, error) {
		if filter == nil || len(filter.Must) != 1 {
			t.Fatalf("expected candidate filter, got %#v", filter)
		}
		if topK != 2 {
			t.Fatalf("topK: got %d want 2", topK)
		}
		return []*qdrant.ScoredPoint{
			{Score: 1, Payload: map[string]*qdrant.Value{"object_id": qdrant.NewValueString("obj-2")}},
			{Score: 2, Payload: map[string]*qdrant.Value{"object_id": qdrant.NewValueString("obj-1")}},
		}, nil
	}

	resp, err := s.Rank(context.Background(), &RankRequest{SubjectID: "u1", Namespace: "ns", Candidates: []string{"obj-1", "obj-2"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Items) != 2 || resp.Items[0].ObjectID != "obj-1" {
		t.Fatalf("unexpected rank response: %+v", resp.Items)
	}
}

func TestRank_FallsBackWhenSubjectVectorMissing(t *testing.T) {
	s := newTestService(&fakeRepo{}, &fakeNsConfig{}, newFakeIDMapper())
	resp, err := s.Rank(context.Background(), &RankRequest{SubjectID: "u1", Namespace: "ns", Candidates: []string{"obj-1", "obj-2"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Items) != 2 || resp.Items[0].ObjectID != "obj-1" || resp.Items[1].ObjectID != "obj-2" {
		t.Fatalf("unexpected fallback ranking: %+v", resp.Items)
	}
}

func TestRank_FallsBackWhenAllCandidateIDsFail(t *testing.T) {
	idmap := newFakeIDMapper()
	idmap.objectErrs["obj-1"] = errors.New("map failed")
	idmap.objectErrs["obj-2"] = errors.New("map failed")
	s := newTestService(&fakeRepo{}, &fakeNsConfig{}, idmap)
	s.fetchSubjectVecFn = func(_ context.Context, _ string, _ uint64) (*qdrant.SparseVector, error) {
		return &qdrant.SparseVector{Indices: []uint32{1}, Values: []float32{1}}, nil
	}

	resp, err := s.Rank(context.Background(), &RankRequest{SubjectID: "u1", Namespace: "ns", Candidates: []string{"obj-1", "obj-2"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Items) != 2 || resp.Items[0].ObjectID != "obj-1" || resp.Items[1].ObjectID != "obj-2" {
		t.Fatalf("unexpected fallback ranking: %+v", resp.Items)
	}
}

func TestHybridCold_ReturnsBlendedResults(t *testing.T) {
	repo := &fakeRepo{count: 3}
	s := newTestService(repo, &fakeNsConfig{cfg: &nsconfig.NamespaceConfig{Gamma: 0}}, newFakeIDMapper())
	s.fetchSubjectVecFn = func(_ context.Context, _ string, _ uint64) (*qdrant.SparseVector, error) {
		return &qdrant.SparseVector{Indices: []uint32{1}, Values: []float32{1}}, nil
	}
	s.searchObjectsFn = func(_ context.Context, _ string, _ *qdrant.SparseVector, _ *qdrant.Filter, _ uint64) ([]*qdrant.ScoredPoint, error) {
		return []*qdrant.ScoredPoint{
			{Score: 4, Payload: map[string]*qdrant.Value{"object_id": qdrant.NewValueString("cf-1")}},
			{Score: 3, Payload: map[string]*qdrant.Value{"object_id": qdrant.NewValueString("cf-2")}},
		}, nil
	}
	s.getTrendingFn = func(_ context.Context, _ string, _, _ int) ([]infraredis.TrendingEntry, error) {
		return []infraredis.TrendingEntry{
			{ObjectID: "pop-1", Score: 10},
			{ObjectID: "pop-2", Score: 9},
		}, nil
	}

	resp, err := s.hybridCold(context.Background(), &Request{SubjectID: "u1", Namespace: "ns"}, 4, &nsconfig.NamespaceConfig{Gamma: 0})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Source != SourceHybridCold {
		t.Fatalf("source: got %q want %q", resp.Source, SourceHybridCold)
	}
	if len(resp.Items) != 4 {
		t.Fatalf("expected 4 items, got %d", len(resp.Items))
	}
}

func TestHybridCold_WhenPopularFailsReturnsCFWithHybridColdSource(t *testing.T) {
	repo := &fakeRepo{count: 3, popularErr: errors.New("popular failed")}
	s := newTestService(repo, &fakeNsConfig{cfg: &nsconfig.NamespaceConfig{Gamma: 0}}, newFakeIDMapper())
	s.fetchSubjectVecFn = func(_ context.Context, _ string, _ uint64) (*qdrant.SparseVector, error) {
		return &qdrant.SparseVector{Indices: []uint32{1}, Values: []float32{1}}, nil
	}
	s.searchObjectsFn = func(_ context.Context, _ string, _ *qdrant.SparseVector, _ *qdrant.Filter, _ uint64) ([]*qdrant.ScoredPoint, error) {
		return []*qdrant.ScoredPoint{
			{Score: 4, Payload: map[string]*qdrant.Value{"object_id": qdrant.NewValueString("cf-1")}},
		}, nil
	}
	s.getTrendingFn = func(_ context.Context, _ string, _, _ int) ([]infraredis.TrendingEntry, error) {
		return nil, errors.New("redis failed")
	}

	resp, err := s.hybridCold(context.Background(), &Request{SubjectID: "u1", Namespace: "ns"}, 2, &nsconfig.NamespaceConfig{Gamma: 0})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Source != SourceHybridCold || len(resp.Items) != 1 || resp.Items[0].ObjectID != "cf-1" {
		t.Fatalf("unexpected response: %+v", resp)
	}
}

func TestHybridCold_WhenCFEmptyReturnsPopular(t *testing.T) {
	s := newTestService(&fakeRepo{count: 3}, &fakeNsConfig{}, newFakeIDMapper())
	s.getTrendingFn = func(_ context.Context, _ string, _, _ int) ([]infraredis.TrendingEntry, error) {
		return []infraredis.TrendingEntry{{ObjectID: "pop-1", Score: 10}}, nil
	}

	resp, err := s.hybridCold(context.Background(), &Request{SubjectID: "u1", Namespace: "ns"}, 2, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Source != SourceFallbackPopular || len(resp.Items) != 1 || resp.Items[0].ObjectID != "pop-1" {
		t.Fatalf("unexpected response: %+v", resp)
	}
}

func TestDeleteObject_DeletesSparseAndDense(t *testing.T) {
	var collections []string
	s := newTestService(&fakeRepo{}, &fakeNsConfig{}, newFakeIDMapper())
	s.deleteFromCollectionFn = func(_ context.Context, collection string, ids []*qdrant.PointId) error {
		if len(ids) != 1 {
			t.Fatalf("expected 1 point id, got %d", len(ids))
		}
		collections = append(collections, collection)
		return nil
	}

	if err := s.DeleteObject(context.Background(), "ns", "obj-1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(collections) != 2 || collections[0] != "ns_objects" || collections[1] != "ns_objects_dense" {
		t.Fatalf("unexpected collections: %v", collections)
	}
}

func TestDeleteObject_IgnoresDenseCleanupFailure(t *testing.T) {
	var collections []string
	s := newTestService(&fakeRepo{}, &fakeNsConfig{}, newFakeIDMapper())
	s.deleteFromCollectionFn = func(_ context.Context, collection string, _ []*qdrant.PointId) error {
		collections = append(collections, collection)
		if collection == "ns_objects_dense" {
			return errors.New("dense cleanup failed")
		}
		return nil
	}

	if err := s.DeleteObject(context.Background(), "ns", "obj-1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(collections) != 2 {
		t.Fatalf("unexpected delete calls: %v", collections)
	}
}

func TestDeleteFromCollection_NotFoundIsNoOp(t *testing.T) {
	s := newTestService(&fakeRepo{}, &fakeNsConfig{}, newFakeIDMapper())
	s.qdrantDeleteFn = func(_ context.Context, _ *qdrant.DeletePoints) error {
		return grpcstatus.Error(codes.NotFound, "missing")
	}
	if err := s.deleteFromCollection(context.Background(), "ns_objects", []*qdrant.PointId{qdrant.NewIDNum(1)}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDeleteFromCollection_Error(t *testing.T) {
	s := newTestService(&fakeRepo{}, &fakeNsConfig{}, newFakeIDMapper())
	s.qdrantDeleteFn = func(_ context.Context, _ *qdrant.DeletePoints) error {
		return errors.New("delete failed")
	}
	if err := s.deleteFromCollection(context.Background(), "ns_objects", []*qdrant.PointId{qdrant.NewIDNum(1)}); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestRecCacheKey(t *testing.T) {
	key := recCacheKey("ns_feed", "user123", 20, 0)
	want := "rec:ns_feed:user123:limit=20:offset=0"
	if key != want {
		t.Errorf("got %q, want %q", key, want)
	}

	keyWithOffset := recCacheKey("ns_feed", "user123", 20, 10)
	wantWithOffset := "rec:ns_feed:user123:limit=20:offset=10"
	if keyWithOffset != wantWithOffset {
		t.Errorf("got %q, want %q", keyWithOffset, wantWithOffset)
	}
}
