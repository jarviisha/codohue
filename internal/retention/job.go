package retention

import (
	"context"
	"log/slog"
	"time"
)

// Repository is the storage surface the retention job needs. Defined as an
// interface so the tests can swap in a fake without a real database.
type Repository interface {
	// PruneBatchRunLogs deletes rows from batch_run_logs whose started_at is
	// older than cutoff. Returns the number of rows removed.
	PruneBatchRunLogs(ctx context.Context, cutoff time.Time) (int64, error)
	// PruneCatalogBacklogSamples deletes rows from catalog_backlog_samples
	// whose sampled_at is older than cutoff. Returns the number of rows
	// removed.
	PruneCatalogBacklogSamples(ctx context.Context, cutoff time.Time) (int64, error)
}

// Config holds the windows that the prune job enforces. Days are converted to
// time.Duration at tick time via 24h * Days; we don't try to align on calendar
// day boundaries because the cron tick already runs on an interval.
type Config struct {
	// BatchRunRetentionDays — rows older than this many days are deleted from
	// batch_run_logs. Zero or negative disables the prune for that table.
	BatchRunRetentionDays int
	// BacklogSamplesRetentionDays — same for catalog_backlog_samples.
	BacklogSamplesRetentionDays int
	// Interval is how often the prune fires. The job sleeps for Interval
	// between ticks; the first tick fires Interval after Run starts so we
	// don't hammer the DB right after boot.
	Interval time.Duration
}

// Job is the retention worker. Run blocks on a ticker until ctx is done.
type Job struct {
	repo  Repository
	cfg   Config
	clock func() time.Time
}

// NewJob constructs a Job. Interval=0 falls back to 1 hour.
func NewJob(repo Repository, cfg Config) *Job {
	if cfg.Interval <= 0 {
		cfg.Interval = time.Hour
	}
	return &Job{repo: repo, cfg: cfg, clock: time.Now}
}

// Run blocks until ctx is cancelled, prune-ticking every Interval. Returns
// nil on graceful shutdown.
func (j *Job) Run(ctx context.Context) error {
	t := time.NewTicker(j.cfg.Interval)
	defer t.Stop()

	slog.InfoContext(ctx, "retention job started",
		slog.Int("batch_run_days", j.cfg.BatchRunRetentionDays),
		slog.Int("backlog_samples_days", j.cfg.BacklogSamplesRetentionDays),
		slog.Duration("interval", j.cfg.Interval),
	)

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-t.C:
			j.tick(ctx)
		}
	}
}

// RunOnce performs a single tick. Exposed for tests so they don't wait on
// real-time tickers.
func (j *Job) RunOnce(ctx context.Context) { j.tick(ctx) }

func (j *Job) tick(ctx context.Context) {
	now := j.clock().UTC()

	if j.cfg.BatchRunRetentionDays > 0 {
		cutoff := now.Add(-time.Duration(j.cfg.BatchRunRetentionDays) * 24 * time.Hour)
		n, err := j.repo.PruneBatchRunLogs(ctx, cutoff)
		if err != nil {
			slog.WarnContext(ctx, "retention: prune batch_run_logs failed",
				slog.String("error", err.Error()),
				slog.Time("cutoff", cutoff))
		} else if n > 0 {
			slog.InfoContext(ctx, "retention: pruned batch_run_logs",
				slog.Int64("rows", n),
				slog.Time("cutoff", cutoff))
		}
	}

	if j.cfg.BacklogSamplesRetentionDays > 0 {
		cutoff := now.Add(-time.Duration(j.cfg.BacklogSamplesRetentionDays) * 24 * time.Hour)
		n, err := j.repo.PruneCatalogBacklogSamples(ctx, cutoff)
		if err != nil {
			slog.WarnContext(ctx, "retention: prune catalog_backlog_samples failed",
				slog.String("error", err.Error()),
				slog.Time("cutoff", cutoff))
		} else if n > 0 {
			slog.InfoContext(ctx, "retention: pruned catalog_backlog_samples",
				slog.Int64("rows", n),
				slog.Time("cutoff", cutoff))
		}
	}
}
