package ingest

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository writes events to the events table in PostgreSQL.
type Repository struct {
	db       *pgxpool.Pool
	insertFn func(ctx context.Context, sql string, arguments ...any) (int64, error)
}

// NewRepository creates a new Repository with the given PostgreSQL connection pool.
func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{
		db: db,
		insertFn: func(ctx context.Context, sql string, arguments ...any) (int64, error) {
			var id int64
			if err := db.QueryRow(ctx, sql, arguments...).Scan(&id); err != nil {
				return 0, fmt.Errorf("exec insert event: %w", err)
			}
			return id, nil
		},
	}
}

// Insert persists a single event and stamps e.ID with the generated row id.
func (r *Repository) Insert(ctx context.Context, e *Event) error {
	id, err := r.insertFn(ctx, `
		INSERT INTO events (namespace, subject_id, object_id, action, weight, occurred_at, object_created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id`,
		e.Namespace, e.SubjectID, e.ObjectID, string(e.Action), e.Weight, e.OccurredAt, e.ObjectCreatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert event: %w", err)
	}
	e.ID = id
	return nil
}
