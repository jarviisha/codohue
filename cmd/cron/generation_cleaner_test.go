package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jarviisha/codohue/internal/core/nslifecycle"
)

type fakeRedisCleaner struct {
	scanned   []string
	deleted   []string
	keysFound []string
	scanErr   error
	deleteErr error
}

func (f *fakeRedisCleaner) ScanKeys(_ context.Context, pattern string) ([]string, error) {
	f.scanned = append(f.scanned, pattern)
	return f.keysFound, f.scanErr
}

func (f *fakeRedisCleaner) DeleteKeys(_ context.Context, keys []string) error {
	f.deleted = append(f.deleted, keys...)
	return f.deleteErr
}

type fakeQdrantCleaner struct {
	existing  map[string]bool
	dropped   []string
	existsErr error
	deleteErr error
}

func (f *fakeQdrantCleaner) CollectionExists(_ context.Context, name string) (bool, error) {
	if f.existsErr != nil {
		return false, f.existsErr
	}
	return f.existing[name], nil
}

func (f *fakeQdrantCleaner) DeleteCollection(_ context.Context, name string) error {
	if f.deleteErr != nil {
		return f.deleteErr
	}
	f.dropped = append(f.dropped, name)
	return nil
}

func TestDeleteRedisGenerationRemovesGenerationQualifiedKeys(t *testing.T) {
	redis := &fakeRedisCleaner{keysFound: []string{"rec:v2:abc:sub-1:10"}}
	cleaner := &storeGenerationCleaner{redis: redis}

	candidate := nslifecycle.CleanupCandidate{Namespace: "shop", Generation: 2}
	if err := cleaner.DeleteRedisGeneration(context.Background(), candidate); err != nil {
		t.Fatalf("DeleteRedisGeneration: %v", err)
	}

	// Generation 2 must carry the qualifier; deleting the bare generation-1 key
	// would destroy the live namespace's data.
	want := map[string]bool{
		"trending:shop:g2":      true,
		"catalog:embed:shop:g2": true,
		"rec:v2:abc:sub-1:10":   true,
	}
	if len(redis.deleted) != len(want) {
		t.Fatalf("deleted %v, want %d keys", redis.deleted, len(want))
	}
	for _, key := range redis.deleted {
		if !want[key] {
			t.Errorf("unexpected key deleted: %q", key)
		}
	}
	cachePrefix := nslifecycle.MustPhysicalName(nslifecycle.KindRecommendationCache, "shop", 2)
	if len(redis.scanned) != 1 || redis.scanned[0] != cachePrefix+":*" {
		t.Errorf("scanned = %v, want %q", redis.scanned, cachePrefix+":*")
	}
}

func TestDeleteRedisGenerationPreservesGeneration1Names(t *testing.T) {
	redis := &fakeRedisCleaner{}
	cleaner := &storeGenerationCleaner{redis: redis}

	candidate := nslifecycle.CleanupCandidate{Namespace: "shop", Generation: 1}
	if err := cleaner.DeleteRedisGeneration(context.Background(), candidate); err != nil {
		t.Fatalf("DeleteRedisGeneration: %v", err)
	}
	for _, key := range redis.deleted {
		if key != "trending:shop" && key != "catalog:embed:shop" {
			t.Errorf("unexpected key deleted: %q", key)
		}
	}
}

func TestDeleteRedisGenerationSurfacesStoreFailures(t *testing.T) {
	scanFail := &storeGenerationCleaner{redis: &fakeRedisCleaner{scanErr: errors.New("boom")}}
	if err := scanFail.DeleteRedisGeneration(context.Background(), nslifecycle.CleanupCandidate{Namespace: "shop", Generation: 1}); err == nil {
		t.Fatal("expected scan failure to propagate")
	}

	delFail := &storeGenerationCleaner{redis: &fakeRedisCleaner{deleteErr: errors.New("boom")}}
	if err := delFail.DeleteRedisGeneration(context.Background(), nslifecycle.CleanupCandidate{Namespace: "shop", Generation: 1}); err == nil {
		t.Fatal("expected delete failure to propagate")
	}
}

func TestDeleteRedisGenerationWithoutRedisIsANoop(t *testing.T) {
	cleaner := &storeGenerationCleaner{}
	if err := cleaner.DeleteRedisGeneration(context.Background(), nslifecycle.CleanupCandidate{Namespace: "shop", Generation: 1}); err != nil {
		t.Fatalf("DeleteRedisGeneration: %v", err)
	}
}

func TestDeleteQdrantGenerationDropsOnlyExistingCollections(t *testing.T) {
	qdrant := &fakeQdrantCleaner{existing: map[string]bool{
		"shop_g2_subjects":      true,
		"shop_g2_objects_dense": true,
	}}
	cleaner := &storeGenerationCleaner{qdrant: qdrant}

	candidate := nslifecycle.CleanupCandidate{Namespace: "shop", Generation: 2}
	if err := cleaner.DeleteQdrantGeneration(context.Background(), candidate); err != nil {
		t.Fatalf("DeleteQdrantGeneration: %v", err)
	}
	if len(qdrant.dropped) != 2 {
		t.Fatalf("dropped = %v, want the two existing collections", qdrant.dropped)
	}

	// A second pass finds nothing left and must still succeed — the janitor
	// re-lists the same candidates on every tick.
	qdrant.existing = map[string]bool{}
	qdrant.dropped = nil
	if err := cleaner.DeleteQdrantGeneration(context.Background(), candidate); err != nil {
		t.Fatalf("second DeleteQdrantGeneration: %v", err)
	}
	if len(qdrant.dropped) != 0 {
		t.Fatalf("dropped = %v on a clean generation", qdrant.dropped)
	}
}

func TestDeleteQdrantGenerationSurfacesStoreFailures(t *testing.T) {
	existsFail := &storeGenerationCleaner{qdrant: &fakeQdrantCleaner{existsErr: errors.New("boom")}}
	if err := existsFail.DeleteQdrantGeneration(context.Background(), nslifecycle.CleanupCandidate{Namespace: "shop", Generation: 1}); err == nil {
		t.Fatal("expected exists failure to propagate")
	}

	deleteFail := &storeGenerationCleaner{qdrant: &fakeQdrantCleaner{
		existing:  map[string]bool{"shop_subjects": true},
		deleteErr: errors.New("boom"),
	}}
	if err := deleteFail.DeleteQdrantGeneration(context.Background(), nslifecycle.CleanupCandidate{Namespace: "shop", Generation: 1}); err == nil {
		t.Fatal("expected delete failure to propagate")
	}
}

func TestNewStoreGenerationCleanerToleratesNilClients(t *testing.T) {
	cleaner := newStoreGenerationCleaner(nil, nil)
	candidate := nslifecycle.CleanupCandidate{Namespace: "shop", Generation: 1}
	if err := cleaner.DeleteRedisGeneration(context.Background(), candidate); err != nil {
		t.Fatalf("DeleteRedisGeneration: %v", err)
	}
	if err := cleaner.DeleteQdrantGeneration(context.Background(), candidate); err != nil {
		t.Fatalf("DeleteQdrantGeneration: %v", err)
	}
}

type stubJanitor struct {
	results []struct {
		cleaned int
		err     error
	}
	calls int
	limit int
	done  chan struct{}
}

func (s *stubJanitor) RunOnce(_ context.Context, limit int) (int, error) {
	s.limit = limit
	idx := s.calls
	s.calls++
	if s.done != nil && s.calls == len(s.results) {
		defer close(s.done)
	}
	if idx >= len(s.results) {
		return 0, nil
	}
	return s.results[idx].cleaned, s.results[idx].err
}

func TestGenerationCleanupLoopRunsImmediatelyAndStopsOnCancel(t *testing.T) {
	janitor := &stubJanitor{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	runGenerationCleanupLoop(ctx, time.Hour, janitor)

	// The first pass happens before the ticker, so a cancelled context still
	// produces exactly one attempt rather than none.
	if janitor.calls != 1 {
		t.Fatalf("calls = %d, want 1", janitor.calls)
	}
	if janitor.limit != generationCleanupBatch {
		t.Errorf("limit = %d, want %d", janitor.limit, generationCleanupBatch)
	}
}

func TestGenerationCleanupLoopKeepsRunningAfterFailures(t *testing.T) {
	done := make(chan struct{})
	janitor := &stubJanitor{done: done, results: []struct {
		cleaned int
		err     error
	}{
		{0, nslifecycle.ErrLegacyEnvelopesOpen},
		{0, errors.New("redis down")},
		{3, nil},
	}}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go runGenerationCleanupLoop(ctx, time.Millisecond, janitor)

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("loop did not reach the third pass")
	}
	cancel()

	// An open gate and a store failure must both leave the loop alive; neither
	// is a reason to stop reclaiming on later ticks.
	if janitor.calls < 3 {
		t.Fatalf("calls = %d, want at least 3", janitor.calls)
	}
}

func TestGenerationCleanupLoopDefaultsANonPositiveInterval(t *testing.T) {
	janitor := &stubJanitor{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// A zero interval would panic time.NewTicker; the loop must clamp it.
	runGenerationCleanupLoop(ctx, 0, janitor)

	if janitor.calls != 1 {
		t.Fatalf("calls = %d, want 1", janitor.calls)
	}
}
