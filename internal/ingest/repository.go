package ingest

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// rowQuerier is the subset of pgxpool.Pool the repository uses. Production
// passes the pool; tests pass a fake.
type rowQuerier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// Repository writes events to the events table in PostgreSQL.
type Repository struct {
	db rowQuerier
}

// NewRepository creates a new Repository with the given PostgreSQL handle
// (a *pgxpool.Pool in production).
func NewRepository(db rowQuerier) *Repository {
	return &Repository{db: db}
}

// Insert persists a single event and stamps e.ID with the generated row id.
func (r *Repository) Insert(ctx context.Context, e *Event) error {
	var id int64
	if err := r.db.QueryRow(ctx, `
		INSERT INTO events (namespace, subject_id, object_id, action, weight, occurred_at, object_created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id`,
		e.Namespace, e.SubjectID, e.ObjectID, string(e.Action), e.Weight, e.OccurredAt, e.ObjectCreatedAt,
	).Scan(&id); err != nil {
		return fmt.Errorf("insert event: %w", err)
	}
	e.ID = id
	return nil
}
