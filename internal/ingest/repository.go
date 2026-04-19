package ingest

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository writes events to the events table in PostgreSQL.
type Repository struct {
	db     *pgxpool.Pool
	execFn func(ctx context.Context, sql string, arguments ...any) error
}

// NewRepository creates a new Repository with the given PostgreSQL connection pool.
func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{
		db: db,
		execFn: func(ctx context.Context, sql string, arguments ...any) error {
			_, err := db.Exec(ctx, sql, arguments...)
			if err != nil {
				return fmt.Errorf("exec insert event: %w", err)
			}
			return nil
		},
	}
}

// Insert persists a single event to the database.
func (r *Repository) Insert(ctx context.Context, e *Event) error {
	err := r.execFn(ctx, `
		INSERT INTO events (namespace, subject_id, object_id, action, weight, occurred_at, object_created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		e.Namespace, e.SubjectID, e.ObjectID, string(e.Action), e.Weight, e.OccurredAt, e.ObjectCreatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert event: %w", err)
	}
	return nil
}
