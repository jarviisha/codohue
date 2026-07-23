package idmap

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/jackc/pgx/v5/pgxpool"
)

type rowScanner interface {
	Scan(dest ...any) error
}

// rowsIterator is the subset of pgx.Rows the multi-row queries use; the seam
// lets GetOrCreateBatch be unit-tested without a live DB.
type rowsIterator interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
	Close()
}

// Repository manages the mapping from string IDs to numeric IDs in the id_mappings table.
type Repository struct {
	db         *pgxpool.Pool
	queryRowFn func(ctx context.Context, sql string, args ...any) rowScanner
	queryFn    func(ctx context.Context, sql string, args ...any) (rowsIterator, error)
}

// NewRepository creates a new Repository with the given PostgreSQL connection pool.
func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{
		db: db,
		queryRowFn: func(ctx context.Context, sql string, args ...any) rowScanner {
			return db.QueryRow(ctx, sql, args...)
		},
		queryFn: func(ctx context.Context, sql string, args ...any) (rowsIterator, error) {
			// The sole caller (GetOrCreateBatch) defers rows.Close(); this is
			// byte-identical to internal/compute's queryFn seam, which passes
			// the same check. sqlclosecheck false-positives on the
			// single-caller field-indirection shape here.
			return db.Query(ctx, sql, args...) //nolint:sqlclosecheck // caller defers rows.Close()
		},
	}
}

// GetOrCreate returns the numeric_id for the given string_id, inserting a new row if absent.
func (r *Repository) GetOrCreate(ctx context.Context, stringID, namespace, entityType string) (uint64, error) {
	var numID int64
	err := r.queryRowFn(ctx, `
		INSERT INTO id_mappings (string_id, namespace, entity_type)
		VALUES ($1, $2, $3)
		ON CONFLICT (namespace, entity_type, string_id) DO UPDATE SET string_id = EXCLUDED.string_id
		RETURNING numeric_id`,
		stringID, namespace, entityType,
	).Scan(&numID)
	if err != nil {
		return 0, fmt.Errorf("get or create id mapping for %q: %w", stringID, err)
	}
	return uint64(numID), nil
}

// Lookup returns the numeric id for stringID without creating one. found is
// false when no mapping exists. Read/delete paths use this instead of
// GetOrCreate so they stop writing junk rows for ids that were never
// ingested (ranking 500 unknown candidates used to mint 500 mappings).
func (r *Repository) Lookup(ctx context.Context, stringID, namespace, entityType string) (numericID uint64, found bool, err error) {
	var numID int64
	err = r.queryRowFn(ctx, `
		SELECT numeric_id FROM id_mappings
		WHERE namespace = $1 AND entity_type = $2 AND string_id = $3`,
		namespace, entityType, stringID,
	).Scan(&numID)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("lookup id mapping for %q: %w", stringID, err)
	}
	return uint64(numID), true, nil
}

// GetOrCreateBatch resolves many string ids in one round-trip. With the
// exclude_authored cap at 5000, the per-id variant cost ~5000 sequential
// queries per uncached recommendation request.
func (r *Repository) GetOrCreateBatch(ctx context.Context, stringIDs []string, namespace, entityType string) (map[string]uint64, error) {
	if len(stringIDs) == 0 {
		return map[string]uint64{}, nil
	}
	// Deduplicate before unnest: ON CONFLICT DO UPDATE errors with "cannot
	// affect row a second time" if the same key appears twice in one INSERT,
	// and callers (e.g. Rank candidates) may legitimately pass duplicates.
	distinct := make([]string, 0, len(stringIDs))
	seen := make(map[string]struct{}, len(stringIDs))
	for _, id := range stringIDs {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		distinct = append(distinct, id)
	}
	rows, err := r.queryFn(ctx, `
		INSERT INTO id_mappings (string_id, namespace, entity_type)
		SELECT unnest($1::text[]), $2, $3
		ON CONFLICT (namespace, entity_type, string_id) DO UPDATE SET string_id = EXCLUDED.string_id
		RETURNING string_id, numeric_id`,
		distinct, namespace, entityType,
	)
	if err != nil {
		return nil, fmt.Errorf("batch get or create id mappings: %w", err)
	}
	defer rows.Close()

	out := make(map[string]uint64, len(stringIDs))
	for rows.Next() {
		var sid string
		var numID int64
		if err := rows.Scan(&sid, &numID); err != nil {
			return nil, fmt.Errorf("scan id mapping: %w", err)
		}
		out[sid] = uint64(numID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate id mappings: %w", err)
	}
	return out, nil
}
