package embedder

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// reembedTriggerSource is the trigger_source value the admin orchestrator
// writes for re-embed runs. The watcher scopes its scans by this value so it
// only closes runs that it actually owns.
//
// Mirrored from internal/admin (cross-domain import forbidden) — the value
// is a stable contract on the batch_run_logs table.
const reembedTriggerSource = "admin_reembed"

// ReembedRun is the watcher's view of one open re-embed batch_run_logs row.
type ReembedRun struct {
	ID        int64
	Namespace string
	StartedAt time.Time
}

// ReembedWatcherRepo is the storage surface required by ReembedWatcher.
// Defined here so the watcher tests can mock storage without a real DB.
type ReembedWatcherRepo interface {
	ListOpenReembedRuns(ctx context.Context) ([]ReembedRun, error)
	CountStaleCatalogItems(ctx context.Context, namespace string) (int, error)
	CountEmbeddedCatalogItems(ctx context.Context, namespace string) (int, error)
	CompleteReembedRun(ctx context.Context, id int64, processed int, success bool, errorMessage string, completedAt time.Time, durationMs int) error
}

// ReembedWatcher closes catalog re-embed batch_run_logs rows once their
// namespace's catalog_items table has no rows left at a stale strategy
// version. Designed to run as a goroutine inside cmd/embedder.
//
// The watcher polls every Interval until ctx is cancelled. It tolerates
// transient repository errors by logging and retrying on the next tick.
type ReembedWatcher struct {
	repo     ReembedWatcherRepo
	interval time.Duration
	clock    func() time.Time
}

// NewReembedWatcher creates a new watcher with sane defaults. interval=0
// becomes 5 seconds (per spec T048).
func NewReembedWatcher(repo ReembedWatcherRepo, interval time.Duration) *ReembedWatcher {
	if interval <= 0 {
		interval = 5 * time.Second
	}
	return &ReembedWatcher{
		repo:     repo,
		interval: interval,
		clock:    time.Now,
	}
}

// Run blocks until ctx is cancelled, polling the repo every Interval.
// Returns nil on graceful shutdown (context.Canceled / DeadlineExceeded
// surfaced via the ticker path).
func (w *ReembedWatcher) Run(ctx context.Context) error {
	t := time.NewTicker(w.interval)
	defer t.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-t.C:
			w.tick(ctx)
		}
	}
}

// RunOnce performs a single tick of the watcher loop. Exposed for tests so
// they don't have to wait on real time.
func (w *ReembedWatcher) RunOnce(ctx context.Context) {
	w.tick(ctx)
}

func (w *ReembedWatcher) tick(ctx context.Context) {
	runs, err := w.repo.ListOpenReembedRuns(ctx)
	if err != nil {
		slog.WarnContext(ctx, "reembed watcher: list open runs failed",
			slog.String("error", err.Error()))
		return
	}

	for _, run := range runs {
		stale, err := w.repo.CountStaleCatalogItems(ctx, run.Namespace)
		if err != nil {
			slog.WarnContext(ctx, "reembed watcher: count stale items failed",
				slog.Int64("batch_run_id", run.ID),
				slog.String("namespace", run.Namespace),
				slog.String("error", err.Error()))
			continue
		}
		if stale > 0 {
			continue
		}

		processed, err := w.repo.CountEmbeddedCatalogItems(ctx, run.Namespace)
		if err != nil {
			slog.WarnContext(ctx, "reembed watcher: count embedded items failed",
				slog.Int64("batch_run_id", run.ID),
				slog.String("namespace", run.Namespace),
				slog.String("error", err.Error()))
			processed = 0
		}

		now := w.clock().UTC()
		duration := int(now.Sub(run.StartedAt) / time.Millisecond)
		if err := w.repo.CompleteReembedRun(ctx, run.ID, processed, true, "", now, duration); err != nil {
			slog.WarnContext(ctx, "reembed watcher: complete run failed",
				slog.Int64("batch_run_id", run.ID),
				slog.String("namespace", run.Namespace),
				slog.String("error", err.Error()))
			continue
		}
		slog.InfoContext(ctx, "reembed watcher: completed re-embed run",
			slog.Int64("batch_run_id", run.ID),
			slog.String("namespace", run.Namespace),
			slog.Int("processed", processed),
			slog.Int("duration_ms", duration))
	}
}

// ─── Postgres-backed repository ───────────────────────────────────────────────

// pgReembedRepo implements ReembedWatcherRepo against pgxpool.
type pgReembedRepo struct {
	db *pgxpool.Pool
	mu sync.Mutex
}

// NewPgReembedRepo creates a Postgres-backed repository for the watcher.
func NewPgReembedRepo(db *pgxpool.Pool) ReembedWatcherRepo {
	return &pgReembedRepo{db: db}
}

// ListOpenReembedRuns returns every batch_run_logs row written by the admin
// re-embed orchestrator that has not yet been closed.
func (r *pgReembedRepo) ListOpenReembedRuns(ctx context.Context) ([]ReembedRun, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, namespace, started_at
		FROM batch_run_logs
		WHERE trigger_source = $1
		  AND completed_at IS NULL`,
		reembedTriggerSource,
	)
	if err != nil {
		return nil, fmt.Errorf("list open reembed runs: %w", err)
	}
	defer rows.Close()

	var out []ReembedRun
	for rows.Next() {
		var run ReembedRun
		if err := rows.Scan(&run.ID, &run.Namespace, &run.StartedAt); err != nil {
			return nil, fmt.Errorf("scan reembed run: %w", err)
		}
		out = append(out, run)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate reembed runs: %w", err)
	}
	return out, nil
}

// CountStaleCatalogItems returns the number of catalog_items in the namespace
// that still need processing under the namespace's currently configured
// catalog_strategy_version. Items in 'pending', 'in_flight', or 'failed' state
// AND whose strategy_version is NULL or differs from the namespace's
// current target are counted.
//
// Items in 'dead_letter' are NOT counted — they require explicit operator
// redrive via the admin API and should not block re-embed completion (per R6).
// Items in 'embedded' state at the right version are also excluded.
func (r *pgReembedRepo) CountStaleCatalogItems(ctx context.Context, namespace string) (int, error) {
	var n int
	err := r.db.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM catalog_items ci
		JOIN namespace_configs nc ON nc.namespace = ci.namespace
		WHERE ci.namespace = $1
		  AND ci.state IN ('pending', 'in_flight', 'failed')
		  AND (ci.strategy_version IS NULL
		       OR ci.strategy_version <> nc.catalog_strategy_version)`,
		namespace,
	).Scan(&n)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("count stale catalog items: %w", err)
	}
	return n, nil
}

// CountEmbeddedCatalogItems returns the number of catalog_items currently in
// 'embedded' state at the namespace's active strategy version. Used as the
// "subjects_processed" tally written to batch_run_logs on completion.
func (r *pgReembedRepo) CountEmbeddedCatalogItems(ctx context.Context, namespace string) (int, error) {
	var n int
	err := r.db.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM catalog_items ci
		JOIN namespace_configs nc ON nc.namespace = ci.namespace
		WHERE ci.namespace = $1
		  AND ci.state = 'embedded'
		  AND ci.strategy_version = nc.catalog_strategy_version`,
		namespace,
	).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("count embedded catalog items: %w", err)
	}
	return n, nil
}

// CompleteReembedRun closes a batch_run_logs row written by the admin
// re-embed orchestrator. success=true clears error_message; success=false
// writes the supplied message.
func (r *pgReembedRepo) CompleteReembedRun(ctx context.Context, id int64, processed int, success bool, errorMessage string, completedAt time.Time, durationMs int) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	var errPtr *string
	if !success {
		msg := errorMessage
		errPtr = &msg
	}
	_, err := r.db.Exec(ctx, `
		UPDATE batch_run_logs
		SET completed_at       = $2,
		    duration_ms        = $3,
		    subjects_processed = $4,
		    success            = $5,
		    error_message      = $6
		WHERE id = $1
		  AND trigger_source = $7
		  AND completed_at IS NULL`,
		id, completedAt, durationMs, processed, success, errPtr, reembedTriggerSource,
	)
	if err != nil {
		return fmt.Errorf("complete reembed run: %w", err)
	}
	return nil
}
