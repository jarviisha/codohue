package objects

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type rowScanner interface {
	Scan(dest ...any) error
}

// Repository reads and writes the objects table in PostgreSQL.
type Repository struct {
	db         *pgxpool.Pool
	queryRowFn func(ctx context.Context, sql string, args ...any) rowScanner
	execFn     func(ctx context.Context, sql string, args ...any) error
}

// NewRepository creates a new Repository with the given connection pool.
func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{
		db: db,
		queryRowFn: func(ctx context.Context, sql string, args ...any) rowScanner {
			return db.QueryRow(ctx, sql, args...)
		},
		execFn: func(ctx context.Context, sql string, args ...any) error {
			_, err := db.Exec(ctx, sql, args...)
			if err != nil {
				return fmt.Errorf("exec objects statement: %w", err)
			}
			return nil
		},
	}
}

// Upsert creates or updates the metadata row for (namespace, object_id).
// An empty authorSubjectID is stored as NULL, which is how attribution is
// cleared.
func (r *Repository) Upsert(ctx context.Context, namespace, objectID, authorSubjectID string) (*Object, error) {
	var obj Object
	err := r.queryRowFn(ctx, `
		INSERT INTO objects (namespace, object_id, author_subject_id, created_at, updated_at)
		VALUES ($1, $2, NULLIF($3, ''), NOW(), NOW())
		ON CONFLICT (namespace, object_id) DO UPDATE
		SET author_subject_id = NULLIF($3, ''),
		    updated_at        = NOW()
		RETURNING namespace, object_id, COALESCE(author_subject_id, ''), created_at, updated_at`,
		namespace, objectID, authorSubjectID,
	).Scan(&obj.Namespace, &obj.ObjectID, &obj.AuthorSubjectID, &obj.CreatedAt, &obj.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("upsert object: %w", err)
	}
	return &obj, nil
}

// Get returns the metadata row, or (nil, nil) when the object has none.
func (r *Repository) Get(ctx context.Context, namespace, objectID string) (*Object, error) {
	var obj Object
	err := r.queryRowFn(ctx, `
		SELECT namespace, object_id, COALESCE(author_subject_id, ''), created_at, updated_at
		FROM objects
		WHERE namespace = $1 AND object_id = $2`,
		namespace, objectID,
	).Scan(&obj.Namespace, &obj.ObjectID, &obj.AuthorSubjectID, &obj.CreatedAt, &obj.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get object: %w", err)
	}
	return &obj, nil
}

// Delete removes the metadata row. Idempotent — deleting an object that was
// never attributed is not an error.
func (r *Repository) Delete(ctx context.Context, namespace, objectID string) error {
	if err := r.execFn(ctx,
		`DELETE FROM objects WHERE namespace = $1 AND object_id = $2`,
		namespace, objectID,
	); err != nil {
		return fmt.Errorf("delete object: %w", err)
	}
	return nil
}
