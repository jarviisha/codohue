package embedder

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type rowScanner interface {
	Scan(dest ...any) error
}

// Repository performs catalog_items state-transition writes on behalf of
// the embedder worker. It deliberately exposes a small surface — load and
// the four state transitions — rather than reusing internal/catalog's
// repository, because the constitution forbids cross-domain imports
// between internal/catalog and internal/embedder.
type Repository struct {
	db         *pgxpool.Pool
	queryRowFn func(ctx context.Context, sql string, args ...any) rowScanner
	execFn     func(ctx context.Context, sql string, args ...any) (int64, error)
}

// NewRepository creates a new Repository with the given PostgreSQL connection pool.
func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{
		db: db,
		queryRowFn: func(ctx context.Context, sql string, args ...any) rowScanner {
			return db.QueryRow(ctx, sql, args...)
		},
		execFn: func(ctx context.Context, sql string, args ...any) (int64, error) {
			tag, err := db.Exec(ctx, sql, args...)
			if err != nil {
				return 0, fmt.Errorf("exec: %w", err)
			}
			return tag.RowsAffected(), nil
		},
	}
}

// LoadByID reads the catalog_items row identified by id, returning the
// projection the embedder service needs. Returns ErrItemNotFound when no
// row matches (likely a race with operator delete).
func (r *Repository) LoadByID(ctx context.Context, id int64) (*PendingItem, error) {
	var (
		item       PendingItem
		strategyID string
		strategyV  string
	)
	err := r.queryRowFn(ctx, `
		SELECT id, namespace, object_id, content, content_hash,
		       COALESCE(strategy_id, ''), COALESCE(strategy_version, ''),
		       attempt_count
		FROM catalog_items
		WHERE id = $1`,
		id,
	).Scan(
		&item.ID, &item.Namespace, &item.ObjectID, &item.Content, &item.ContentHash,
		&strategyID, &strategyV,
		&item.AttemptCount,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrItemNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("load catalog item %d: %w", id, err)
	}
	item.StrategyID = strategyID
	item.StrategyVersion = strategyV
	return &item, nil
}

// MarkInFlight transitions the row to state='in_flight' and increments
// attempt_count. Returns the new attempt_count so the caller can decide
// whether to bail to dead_letter once attempts exceed max.
func (r *Repository) MarkInFlight(ctx context.Context, id int64) (int, error) {
	var newAttempt int
	err := r.queryRowFn(ctx, `
		UPDATE catalog_items
		SET state = 'in_flight',
		    attempt_count = attempt_count + 1,
		    updated_at = NOW()
		WHERE id = $1
		RETURNING attempt_count`,
		id,
	).Scan(&newAttempt)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrItemNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("mark in_flight %d: %w", id, err)
	}
	return newAttempt, nil
}

// MarkEmbedded transitions the row to state='embedded', writing the
// strategy id+version under which the embedding was produced and
// embedded_at=NOW(). Clears last_error.
func (r *Repository) MarkEmbedded(ctx context.Context, id int64, strategyID, strategyVersion string, embeddedAt time.Time) error {
	rowsAffected, err := r.execFn(ctx, `
		UPDATE catalog_items
		SET state = 'embedded',
		    strategy_id = $2,
		    strategy_version = $3,
		    embedded_at = $4,
		    last_error = NULL,
		    updated_at = NOW()
		WHERE id = $1`,
		id, strategyID, strategyVersion, embeddedAt,
	)
	if err != nil {
		return fmt.Errorf("mark embedded %d: %w", id, err)
	}
	if rowsAffected == 0 {
		return ErrItemNotFound
	}
	return nil
}

// MarkFailed records a transient embedding failure. State transitions to
// 'failed' and last_error is updated. attempt_count is NOT touched here —
// MarkInFlight already incremented it before processing began.
func (r *Repository) MarkFailed(ctx context.Context, id int64, lastError string) error {
	rowsAffected, err := r.execFn(ctx, `
		UPDATE catalog_items
		SET state = 'failed',
		    last_error = $2,
		    updated_at = NOW()
		WHERE id = $1`,
		id, lastError,
	)
	if err != nil {
		return fmt.Errorf("mark failed %d: %w", id, err)
	}
	if rowsAffected == 0 {
		return ErrItemNotFound
	}
	return nil
}

// MarkDeadLetter records a terminal failure. State transitions to
// 'dead_letter' and last_error is updated. The operator must explicitly
// re-drive (admin endpoint) to retry a dead-lettered item.
func (r *Repository) MarkDeadLetter(ctx context.Context, id int64, lastError string) error {
	rowsAffected, err := r.execFn(ctx, `
		UPDATE catalog_items
		SET state = 'dead_letter',
		    last_error = $2,
		    updated_at = NOW()
		WHERE id = $1`,
		id, lastError,
	)
	if err != nil {
		return fmt.Errorf("mark dead_letter %d: %w", id, err)
	}
	if rowsAffected == 0 {
		return ErrItemNotFound
	}
	return nil
}

// BacklogStateCounts holds per-state catalog_items counts the sampler writes
// to catalog_backlog_samples each tick. Only the operationally interesting
// states are tracked — `embedded` rows are deliberately omitted because the
// backlog timeline is about work-still-to-do, not throughput history.
type BacklogStateCounts struct {
	Pending    int
	InFlight   int
	Failed     int
	DeadLetter int
}

// CountBacklogStates returns the live count of catalog_items in each non-
// embedded state for a namespace. Wraps the same group-by-state aggregate
// internal/admin uses but lives here so the architecture rule's cross-domain
// import ban is honoured.
func (r *Repository) CountBacklogStates(ctx context.Context, namespace string) (BacklogStateCounts, error) {
	rows, err := r.db.Query(ctx, `
		SELECT state, COUNT(*)
		FROM catalog_items
		WHERE namespace = $1
		  AND state IN ('pending', 'in_flight', 'failed', 'dead_letter')
		GROUP BY state`,
		namespace,
	)
	if err != nil {
		return BacklogStateCounts{}, fmt.Errorf("count backlog states: %w", err)
	}
	defer rows.Close()

	var out BacklogStateCounts
	for rows.Next() {
		var state string
		var n int
		if err := rows.Scan(&state, &n); err != nil {
			return BacklogStateCounts{}, fmt.Errorf("scan backlog row: %w", err)
		}
		switch state {
		case "pending":
			out.Pending = n
		case "in_flight":
			out.InFlight = n
		case "failed":
			out.Failed = n
		case "dead_letter":
			out.DeadLetter = n
		}
	}
	if err := rows.Err(); err != nil {
		return BacklogStateCounts{}, fmt.Errorf("iterate backlog rows: %w", err)
	}
	return out, nil
}

// InsertBacklogSample appends one row to catalog_backlog_samples (migration
// 014). The (namespace, sampled_at) PRIMARY KEY makes repeated inserts at
// the same instant a hard error — callers must space samples at least one
// microsecond apart, which the sampler trivially does (30s cadence).
func (r *Repository) InsertBacklogSample(
	ctx context.Context,
	namespace string,
	sampledAt time.Time,
	pending, inFlight, failed, deadLetter, streamLen int,
) error {
	if _, err := r.execFn(ctx, `
		INSERT INTO catalog_backlog_samples
		    (namespace, sampled_at, pending, in_flight, failed, dead_letter, stream_len)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		namespace, sampledAt, pending, inFlight, failed, deadLetter, streamLen,
	); err != nil {
		return fmt.Errorf("insert backlog sample for %s: %w", namespace, err)
	}
	return nil
}

// LatestBacklogSample returns the most recently recorded sample for a
// namespace plus whether one exists. The sampler uses this to skip writing
// duplicate snapshots when no field changed since last tick (BUILD_PLAN §8
// "Sampler skip rule" — keeps the table from bloating during idle hours).
func (r *Repository) LatestBacklogSample(ctx context.Context, namespace string) (BacklogStateCounts, int, bool, error) {
	row := r.queryRowFn(ctx, `
		SELECT pending, in_flight, failed, dead_letter, stream_len
		FROM catalog_backlog_samples
		WHERE namespace = $1
		ORDER BY sampled_at DESC
		LIMIT 1`,
		namespace,
	)
	var counts BacklogStateCounts
	var streamLen int
	if err := row.Scan(&counts.Pending, &counts.InFlight, &counts.Failed, &counts.DeadLetter, &streamLen); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return BacklogStateCounts{}, 0, false, nil
		}
		return BacklogStateCounts{}, 0, false, fmt.Errorf("latest backlog sample for %s: %w", namespace, err)
	}
	return counts, streamLen, true, nil
}
