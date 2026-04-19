package idmap

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

type rowScanner interface {
	Scan(dest ...any) error
}

// Repository manages the mapping from string IDs to numeric IDs in the id_mappings table.
type Repository struct {
	db         *pgxpool.Pool
	queryRowFn func(ctx context.Context, sql string, args ...any) rowScanner
}

// NewRepository creates a new Repository with the given PostgreSQL connection pool.
func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{
		db: db,
		queryRowFn: func(ctx context.Context, sql string, args ...any) rowScanner {
			return db.QueryRow(ctx, sql, args...)
		},
	}
}

// GetOrCreate returns the numeric_id for the given string_id, inserting a new row if absent.
func (r *Repository) GetOrCreate(ctx context.Context, stringID, namespace, entityType string) (uint64, error) {
	var numID int64
	err := r.queryRowFn(ctx, `
		INSERT INTO id_mappings (string_id, namespace, entity_type)
		VALUES ($1, $2, $3)
		ON CONFLICT (string_id) DO UPDATE SET string_id = EXCLUDED.string_id
		RETURNING numeric_id`,
		stringID, namespace, entityType,
	).Scan(&numID)
	if err != nil {
		return 0, fmt.Errorf("get or create id mapping for %q: %w", stringID, err)
	}
	return uint64(numID), nil
}
