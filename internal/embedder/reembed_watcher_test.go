package embedder

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jarviisha/codohue/internal/core/nslifecycle"
)

// fakeReembedRepo is a thread-safe in-memory ReembedWatcherRepo for tests.
type fakeReembedRepo struct {
	mu sync.Mutex

	openRuns        []ReembedRun
	listErr         error
	staleCount      map[string]int
	staleErr        error
	embeddedCount   map[string]int
	embeddedErr     error
	staleTargets    []string
	embeddedTargets []string
	completeErr     error
	completedCalls  []completeCall
}

type completeCall struct {
	id         int64
	processed  int
	success    bool
	errMessage string
	completed  time.Time
	duration   int
}

func (f *fakeReembedRepo) ListOpenReembedRuns(_ context.Context) ([]ReembedRun, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.listErr != nil {
		return nil, f.listErr
	}
	out := append([]ReembedRun(nil), f.openRuns...)
	return out, nil
}

func (f *fakeReembedRepo) CountStaleCatalogItems(_ context.Context, ns, strategyID, target string) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.staleTargets = append(f.staleTargets, strategyID+"/"+target)
	if f.staleErr != nil {
		return 0, f.staleErr
	}
	return f.staleCount[ns], nil
}

func (f *fakeReembedRepo) CountEmbeddedCatalogItems(_ context.Context, ns, strategyID, target string) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.embeddedTargets = append(f.embeddedTargets, strategyID+"/"+target)
	if f.embeddedErr != nil {
		return 0, f.embeddedErr
	}
	return f.embeddedCount[ns], nil
}

func (f *fakeReembedRepo) CompleteReembedRun(_ context.Context, id int64, processed int, success bool, msg string, completedAt time.Time, duration int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.completeErr != nil {
		return f.completeErr
	}
	f.completedCalls = append(f.completedCalls, completeCall{
		id:         id,
		processed:  processed,
		success:    success,
		errMessage: msg,
		completed:  completedAt,
		duration:   duration,
	})
	// Remove the run from open list to mimic real DB.
	out := f.openRuns[:0]
	for _, r := range f.openRuns {
		if r.ID != id {
			out = append(out, r)
		}
	}
	f.openRuns = out
	return nil
}

func TestReembedWatcher_CompletesWhenBacklogEmpty(t *testing.T) {
	startedAt := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)
	repo := &fakeReembedRepo{
		openRuns: []ReembedRun{
			{ID: 11, Namespace: "ns", TargetStrategyID: "model-a", TargetStrategyVersion: "v2", StartedAt: startedAt},
		},
		staleCount:    map[string]int{"ns": 0},
		embeddedCount: map[string]int{"ns": 25},
	}
	w := NewReembedWatcher(repo, 0)
	w.clock = func() time.Time { return startedAt.Add(3 * time.Second) }

	w.RunOnce(context.Background())

	if len(repo.completedCalls) != 1 {
		t.Fatalf("expected 1 complete call, got %d", len(repo.completedCalls))
	}
	got := repo.completedCalls[0]
	if got.id != 11 || got.processed != 25 || !got.success {
		t.Errorf("unexpected complete call: %+v", got)
	}
	if got.duration != 3000 {
		t.Errorf("expected duration_ms=3000, got %d", got.duration)
	}
	if len(repo.staleTargets) != 1 || repo.staleTargets[0] != "model-a/v2" ||
		len(repo.embeddedTargets) != 1 || repo.embeddedTargets[0] != "model-a/v2" {
		t.Fatalf("watcher did not use frozen target version: stale=%v embedded=%v", repo.staleTargets, repo.embeddedTargets)
	}
}

func TestReembedWatcher_LeavesOpenWhenBacklogNonZero(t *testing.T) {
	repo := &fakeReembedRepo{
		openRuns: []ReembedRun{
			{ID: 12, Namespace: "ns", StartedAt: time.Now()},
		},
		staleCount:    map[string]int{"ns": 7},
		embeddedCount: map[string]int{"ns": 0},
	}
	w := NewReembedWatcher(repo, 0)

	w.RunOnce(context.Background())

	if len(repo.completedCalls) != 0 {
		t.Errorf("expected NO complete call while backlog>0, got %d", len(repo.completedCalls))
	}
}

func TestReembedWatcher_ToleratesListError(t *testing.T) {
	repo := &fakeReembedRepo{listErr: errors.New("db down")}
	w := NewReembedWatcher(repo, 0)

	// Must not panic; tick should swallow the error and log.
	w.RunOnce(context.Background())

	if len(repo.completedCalls) != 0 {
		t.Errorf("expected no completion calls on list error, got %d", len(repo.completedCalls))
	}
}

func TestReembedWatcher_ToleratesStaleCountError(t *testing.T) {
	repo := &fakeReembedRepo{
		openRuns: []ReembedRun{{ID: 1, Namespace: "ns"}},
		staleErr: errors.New("query failed"),
	}
	w := NewReembedWatcher(repo, 0)

	w.RunOnce(context.Background())

	if len(repo.completedCalls) != 0 {
		t.Errorf("expected no completion when stale-count fails, got %d", len(repo.completedCalls))
	}
}

func TestReembedWatcher_ToleratesEmbeddedCountError(t *testing.T) {
	repo := &fakeReembedRepo{
		openRuns:    []ReembedRun{{ID: 5, Namespace: "ns", StartedAt: time.Now()}},
		staleCount:  map[string]int{"ns": 0},
		embeddedErr: errors.New("query failed"),
	}
	w := NewReembedWatcher(repo, 0)

	w.RunOnce(context.Background())

	// Should still complete the row, with processed=0 fallback.
	if len(repo.completedCalls) != 1 {
		t.Fatalf("expected 1 completion despite embedded-count error, got %d", len(repo.completedCalls))
	}
	if repo.completedCalls[0].processed != 0 {
		t.Errorf("expected processed=0 fallback on count error, got %d", repo.completedCalls[0].processed)
	}
}

func TestReembedWatcher_ProcessesMultipleNamespacesPerTick(t *testing.T) {
	now := time.Now()
	repo := &fakeReembedRepo{
		openRuns: []ReembedRun{
			{ID: 1, Namespace: "ns1", StartedAt: now},
			{ID: 2, Namespace: "ns2", StartedAt: now},
			{ID: 3, Namespace: "ns3", StartedAt: now},
		},
		staleCount: map[string]int{
			"ns1": 0,
			"ns2": 5, // not yet done
			"ns3": 0,
		},
		embeddedCount: map[string]int{"ns1": 10, "ns3": 30},
	}
	w := NewReembedWatcher(repo, 0)

	w.RunOnce(context.Background())

	if len(repo.completedCalls) != 2 {
		t.Fatalf("expected 2 completed (ns1, ns3), got %d", len(repo.completedCalls))
	}
}

func TestReembedWatcher_RunStopsOnContextCancel(t *testing.T) {
	repo := &fakeReembedRepo{}
	w := NewReembedWatcher(repo, 1*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- w.Run(ctx)
	}()

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("expected nil on cancel, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after context cancel")
	}
}

// The watcher closes a run by writing to batch_run_logs, which is namespace
// state: a run must not be marked complete for a namespace that is being
// deleted, or the operator sees a successful re-embed for data that is gone.
func TestReembedWatcher_CompletionRunsUnderLifecycleLease(t *testing.T) {
	startedAt := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)
	newRepo := func() *fakeReembedRepo {
		return &fakeReembedRepo{
			openRuns:      []ReembedRun{{ID: 11, Namespace: "ns", TargetStrategyID: "model-a", TargetStrategyVersion: "v2", StartedAt: startedAt}},
			staleCount:    map[string]int{"ns": 0},
			embeddedCount: map[string]int{"ns": 25},
		}
	}

	repo := newRepo()
	w := NewReembedWatcher(repo, 0)
	w.clock = func() time.Time { return startedAt.Add(3 * time.Second) }
	lifecycle := &recordingLifecycleWriter{generation: 2}
	w.SetLifecycleWriter(lifecycle)

	w.RunOnce(context.Background())

	if lifecycle.calls != 1 {
		t.Errorf("expected exactly 1 lease acquisition, got %d", lifecycle.calls)
	}
	if len(repo.completedCalls) != 1 {
		t.Fatalf("expected the run to be completed under the lease, got %d calls", len(repo.completedCalls))
	}

	blocked := newRepo()
	w = NewReembedWatcher(blocked, 0)
	w.clock = func() time.Time { return startedAt.Add(3 * time.Second) }
	w.SetLifecycleWriter(&recordingLifecycleWriter{err: nslifecycle.ErrNamespaceNotActive})

	w.RunOnce(context.Background())

	if len(blocked.completedCalls) != 0 {
		t.Errorf("inactive namespace must not have its run completed, got %d calls", len(blocked.completedCalls))
	}
}

// Two strategies can publish the same version string ("v1" is not scarce), so
// a run's target is the whole (strategy_id, strategy_version) tuple. Counting
// stale rows on the version alone would let a switch between same-versioned
// strategies look like an instantly-complete re-embed.
func TestReembedWatcher_CountsAgainstTheWholeStrategyTuple(t *testing.T) {
	startedAt := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)
	repo := &fakeReembedRepo{
		openRuns: []ReembedRun{
			{ID: 11, Namespace: "ns", TargetStrategyID: "model-b", TargetStrategyVersion: "v1", StartedAt: startedAt},
		},
		staleCount:    map[string]int{"ns": 0},
		embeddedCount: map[string]int{"ns": 4},
	}
	w := NewReembedWatcher(repo, 0)
	w.clock = func() time.Time { return startedAt.Add(time.Second) }

	w.RunOnce(context.Background())

	if len(repo.staleTargets) != 1 || repo.staleTargets[0] != "model-b/v1" {
		t.Errorf("stale count target = %v, want [model-b/v1]", repo.staleTargets)
	}
	if len(repo.embeddedTargets) != 1 || repo.embeddedTargets[0] != "model-b/v1" {
		t.Errorf("embedded count target = %v, want [model-b/v1]", repo.embeddedTargets)
	}
}

// The run's target is frozen at trigger time. If the watcher re-read the
// namespace's current strategy instead, a config change mid-run would silently
// move the goalposts and close the run against a target nobody asked for.
func TestReembedWatcher_TargetIsFrozenPerRun(t *testing.T) {
	startedAt := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)
	repo := &fakeReembedRepo{
		openRuns: []ReembedRun{
			{ID: 11, Namespace: "ns", TargetStrategyID: "model-a", TargetStrategyVersion: "v1", StartedAt: startedAt},
			{ID: 12, Namespace: "other", TargetStrategyID: "model-a", TargetStrategyVersion: "v2", StartedAt: startedAt},
		},
		staleCount:    map[string]int{"ns": 0, "other": 3},
		embeddedCount: map[string]int{"ns": 1, "other": 1},
	}
	w := NewReembedWatcher(repo, 0)
	w.clock = func() time.Time { return startedAt.Add(time.Second) }

	w.RunOnce(context.Background())

	if len(repo.staleTargets) != 2 || repo.staleTargets[0] != "model-a/v1" || repo.staleTargets[1] != "model-a/v2" {
		t.Errorf("each run must use its own frozen target, got %v", repo.staleTargets)
	}
	// Only the drained run closes.
	if len(repo.completedCalls) != 1 || repo.completedCalls[0].id != 11 {
		t.Errorf("expected only run 11 to complete, got %+v", repo.completedCalls)
	}
}
