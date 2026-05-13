package embedder

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// fakeReembedRepo is a thread-safe in-memory ReembedWatcherRepo for tests.
type fakeReembedRepo struct {
	mu sync.Mutex

	openRuns       []ReembedRun
	listErr        error
	staleCount     map[string]int
	staleErr       error
	embeddedCount  map[string]int
	embeddedErr    error
	completeErr    error
	completedCalls []completeCall
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

func (f *fakeReembedRepo) CountStaleCatalogItems(_ context.Context, ns string) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.staleErr != nil {
		return 0, f.staleErr
	}
	return f.staleCount[ns], nil
}

func (f *fakeReembedRepo) CountEmbeddedCatalogItems(_ context.Context, ns string) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
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
			{ID: 11, Namespace: "ns", StartedAt: startedAt},
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
