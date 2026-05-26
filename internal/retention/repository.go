package retention

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// pgRepository implements Repository against pgxpool.
type pgRepository struct {
	db *pgxpool.Pool
}

// NewPgRepository wraps a pgxpool as a retention Repository.
func NewPgRepository(db *pgxpool.Pool) Repository {
	return &pgRepository{db: db}
}

func (r *pgRepository) PruneBatchRunLogs(ctx context.Context, cutoff time.Time) (int64, error) {
	tag, err := r.db.Exec(ctx, `DELETE FROM batch_run_logs WHERE started_at < $1`, cutoff)
	if err != nil {
		return 0, fmt.Errorf("prune batch_run_logs: %w", err)
	}
	return tag.RowsAffected(), nil
}

func (r *pgRepository) PruneCatalogBacklogSamples(ctx context.Context, cutoff time.Time) (int64, error) {
	tag, err := r.db.Exec(ctx, `DELETE FROM catalog_backlog_samples WHERE sampled_at < $1`, cutoff)
	if err != nil {
		return 0, fmt.Errorf("prune catalog_backlog_samples: %w", err)
	}
	return tag.RowsAffected(), nil
}
