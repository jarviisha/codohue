package retention

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeRepo struct {
	batchCalls   []time.Time
	backlogCalls []time.Time
	batchN       int64
	backlogN     int64
	batchErr     error
	backlogErr   error
}

func (f *fakeRepo) PruneBatchRunLogs(_ context.Context, cutoff time.Time) (int64, error) {
	f.batchCalls = append(f.batchCalls, cutoff)
	return f.batchN, f.batchErr
}

func (f *fakeRepo) PruneCatalogBacklogSamples(_ context.Context, cutoff time.Time) (int64, error) {
	f.backlogCalls = append(f.backlogCalls, cutoff)
	return f.backlogN, f.backlogErr
}

func TestRunOnce_PrunesBothTablesWithExpectedCutoffs(t *testing.T) {
	repo := &fakeRepo{batchN: 12, backlogN: 35}
	now := time.Date(2026, 5, 26, 10, 0, 0, 0, time.UTC)
	job := NewJob(repo, Config{
		BatchRunRetentionDays:       30,
		BacklogSamplesRetentionDays: 7,
		Interval:                    time.Hour,
	})
	job.clock = func() time.Time { return now }

	job.RunOnce(context.Background())

	if len(repo.batchCalls) != 1 {
		t.Fatalf("expected 1 batch_run_logs prune call, got %d", len(repo.batchCalls))
	}
	wantBatchCutoff := now.Add(-30 * 24 * time.Hour)
	if !repo.batchCalls[0].Equal(wantBatchCutoff) {
		t.Errorf("batch cutoff: got %v, want %v", repo.batchCalls[0], wantBatchCutoff)
	}

	if len(repo.backlogCalls) != 1 {
		t.Fatalf("expected 1 catalog_backlog_samples prune call, got %d", len(repo.backlogCalls))
	}
	wantBacklogCutoff := now.Add(-7 * 24 * time.Hour)
	if !repo.backlogCalls[0].Equal(wantBacklogCutoff) {
		t.Errorf("backlog cutoff: got %v, want %v", repo.backlogCalls[0], wantBacklogCutoff)
	}
}

func TestRunOnce_SkipsPruneWhenDaysIsZeroOrNegative(t *testing.T) {
	repo := &fakeRepo{}
	job := NewJob(repo, Config{
		BatchRunRetentionDays:       0,
		BacklogSamplesRetentionDays: -1,
	})
	job.clock = time.Now

	job.RunOnce(context.Background())

	if len(repo.batchCalls) != 0 {
		t.Errorf("expected no batch prune calls, got %d", len(repo.batchCalls))
	}
	if len(repo.backlogCalls) != 0 {
		t.Errorf("expected no backlog prune calls, got %d", len(repo.backlogCalls))
	}
}

func TestRunOnce_ContinuesAfterRepoError(t *testing.T) {
	repo := &fakeRepo{batchErr: errors.New("boom"), backlogN: 4}
	job := NewJob(repo, Config{
		BatchRunRetentionDays:       7,
		BacklogSamplesRetentionDays: 7,
	})
	job.clock = time.Now

	// Should not panic and should still attempt the backlog prune even
	// though the batch prune errored.
	job.RunOnce(context.Background())

	if len(repo.batchCalls) != 1 {
		t.Errorf("batch prune should have been attempted once, got %d", len(repo.batchCalls))
	}
	if len(repo.backlogCalls) != 1 {
		t.Errorf("backlog prune should still run after batch error, got %d", len(repo.backlogCalls))
	}
}

func TestNewJob_DefaultsIntervalWhenZero(t *testing.T) {
	job := NewJob(&fakeRepo{}, Config{Interval: 0})
	if job.cfg.Interval != time.Hour {
		t.Errorf("default interval: got %v, want 1h", job.cfg.Interval)
	}
}

func TestRun_ReturnsNilOnContextCancel(t *testing.T) {
	job := NewJob(&fakeRepo{}, Config{Interval: 10 * time.Millisecond})
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() { done <- job.Run(ctx) }()

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Run returned %v, want nil on cancel", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Run did not return after context cancel")
	}
}
