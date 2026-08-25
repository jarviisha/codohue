package catalog

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type rowScanner interface {
	Scan(dest ...any) error
}

// Repository writes catalog_items rows in PostgreSQL.
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

// ObjectRow is one row of the reconciliation read.
type ObjectRow struct {
	ID        int64
	ObjectID  string
	UpdatedAt time.Time
}

// ListObjects returns the object ids the namespace holds, ordered by
// updated_at ascending (stable id tie-break) so a repair pass can page
// forward and resume from the last updated_at it saw. changedSince nil means
// "everything".
func (r *Repository) ListObjects(ctx context.Context, namespace string, changedSince *time.Time, limit, offset int, cursor *objectCursor) ([]ObjectRow, int, error) {
	var total int
	err := r.queryRowFn(ctx, `
		SELECT COUNT(*) FROM catalog_items
		WHERE namespace = $1 AND ($2::timestamptz IS NULL OR updated_at > $2)`,
		namespace, changedSince,
	).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("count catalog objects: %w", err)
	}

	var cursorTime *time.Time
	var cursorID int64
	if cursor != nil {
		cursorTime, cursorID = &cursor.UpdatedAt, cursor.ID
	}
	rows, err := r.db.Query(ctx, `
		SELECT id, object_id, updated_at FROM catalog_items
		WHERE namespace = $1 AND ($2::timestamptz IS NULL OR updated_at > $2)
		  AND ($5::timestamptz IS NULL OR (updated_at, id) > ($5, $6))
		ORDER BY updated_at ASC, id ASC
		LIMIT $3 OFFSET $4`,
		namespace, changedSince, limit, offset, cursorTime, cursorID,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("list catalog objects: %w", err)
	}
	defer rows.Close()

	var out []ObjectRow
	for rows.Next() {
		var row ObjectRow
		if err := rows.Scan(&row.ID, &row.ObjectID, &row.UpdatedAt); err != nil {
			return nil, 0, fmt.Errorf("scan catalog object row: %w", err)
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate catalog objects: %w", err)
	}
	return out, total, nil
}

// UpsertResult bundles the row and an indicator of whether the caller should
// publish a new entry to the embedder stream.
type UpsertResult struct {
	Item         *Item
	NeedsPublish bool
}

// AttributionWriter records the object's author inside the same transaction as
// the content write. It takes the open transaction rather than a context so
// the objects domain — which owns that table — remains the only code that
// spells the statement; cmd/api supplies the implementation, because the
// import rule forbids reaching into a peer domain from here.
type AttributionWriter func(ctx context.Context, tx pgx.Tx) error

// UpsertWithAttribution persists the catalog row and the requested attribution
// in ONE transaction.
//
// Attribution used to be a best-effort write after the row landed, so a
// failing objects write returned 202 with the author silently dropped — the
// caller had no way to learn its attribution never happened. Committing both
// together means the request either records both facts or neither.
//
// writeAuthor nil means the request carried no attribution, which leaves any
// existing author alone (absence means "unspecified", not "clear it").
func (r *Repository) UpsertWithAttribution(
	ctx context.Context,
	namespace, objectID, content string,
	contentHash []byte,
	metadata map[string]any,
	writeAuthor AttributionWriter,
) (*UpsertResult, error) {
	if r.db == nil || writeAuthor == nil {
		res, err := r.Upsert(ctx, namespace, objectID, content, contentHash, metadata)
		if err != nil || writeAuthor == nil {
			return res, err
		}
		// No pool (unit tests drive the seams directly): still run the hook so
		// the ordering is exercised, just without a real transaction.
		if err := writeAuthor(ctx, nil); err != nil {
			return nil, err
		}
		return res, nil
	}

	var result *UpsertResult
	if err := pgx.BeginFunc(ctx, r.db, func(tx pgx.Tx) error {
		txRepo := &Repository{queryRowFn: func(ctx context.Context, sql string, args ...any) rowScanner {
			return tx.QueryRow(ctx, sql, args...)
		}}
		var err error
		result, err = txRepo.Upsert(ctx, namespace, objectID, content, contentHash, metadata)
		if err != nil {
			return err
		}
		return writeAuthor(ctx, tx)
	}); err != nil {
		return nil, fmt.Errorf("catalog ingest transaction: %w", err)
	}
	return result, nil
}

// Upsert creates or updates a catalog_items row keyed by (namespace, object_id).
//
// Behaviour (per data-model.md §1 lifecycle and FR-002 idempotency):
//
//   - Fresh row: INSERT with state='pending', NeedsPublish=true.
//   - Existing row, same content_hash: metadata and updated_at are refreshed
//     but state, attempt_count, last_error are left untouched. NeedsPublish=false
//     so a re-ingest with identical content does not redo embedding work.
//   - Existing row, different content_hash: state reset to 'pending',
//     attempt_count=0, last_error=NULL; content + content_hash + metadata
//     all replaced. NeedsPublish=true so the embedder picks up the new content.
//
// strategy_id and strategy_version are NEVER touched by this method — they
// are only written by the embedder on a successful embed.
//
// Attribution is deliberately absent: author lives in the objects table
// (migration 021) so it works under every dense_source, not only here.
func (r *Repository) Upsert(ctx context.Context, namespace, objectID, content string, contentHash []byte, metadata map[string]any) (*UpsertResult, error) {
	metaBytes, err := marshalMetadata(metadata)
	if err != nil {
		return nil, err
	}

	var (
		item         Item
		metaRaw      []byte
		needsPublish bool
		// embedded_at can be NULL; pgx scans NULL into a nil *time.Time.
	)

	// CTE pre-reads the existing content_hash so we can decide whether the
	// upsert needs to publish to the stream. If the row is fresh,
	// existing.content_hash is NULL and needs_publish is true.
	err = r.queryRowFn(ctx, `
		WITH existing AS (
			SELECT content_hash FROM catalog_items
			WHERE namespace = $1 AND object_id = $2
		),
		upserted AS (
			INSERT INTO catalog_items (
				namespace, object_id, content, content_hash, metadata,
				state, attempt_count, last_error, created_at, updated_at
			)
			VALUES ($1, $2, $3, $4, $5, 'pending', 0, NULL, NOW(), NOW())
			ON CONFLICT (namespace, object_id) DO UPDATE
			SET content           = EXCLUDED.content,
			    content_hash      = EXCLUDED.content_hash,
			    metadata          = EXCLUDED.metadata,
			    state         = CASE WHEN catalog_items.content_hash = EXCLUDED.content_hash
			                         THEN catalog_items.state ELSE 'pending' END,
			    attempt_count = CASE WHEN catalog_items.content_hash = EXCLUDED.content_hash
			                         THEN catalog_items.attempt_count ELSE 0 END,
			    last_error    = CASE WHEN catalog_items.content_hash = EXCLUDED.content_hash
			                         THEN catalog_items.last_error ELSE NULL END,
			    updated_at    = NOW()
			RETURNING
				id, namespace, object_id, content, content_hash, metadata,
				state,
				COALESCE(strategy_id, '')      AS strategy_id,
				COALESCE(strategy_version, '') AS strategy_version,
				embedded_at, attempt_count,
				COALESCE(last_error, '')       AS last_error,
				created_at, updated_at
		)
		SELECT
			u.id, u.namespace, u.object_id, u.content, u.content_hash, u.metadata,
			u.state, u.strategy_id, u.strategy_version,
			u.embedded_at, u.attempt_count, u.last_error,
			u.created_at, u.updated_at,
			(NOT EXISTS (SELECT 1 FROM existing) OR (SELECT content_hash FROM existing) <> u.content_hash) AS needs_publish
		FROM upserted u`,
		namespace, objectID, content, contentHash, metaBytes,
	).Scan(
		&item.ID, &item.Namespace, &item.ObjectID, &item.Content, &item.ContentHash, &metaRaw,
		&item.State, &item.StrategyID, &item.StrategyVersion,
		&item.EmbeddedAt, &item.AttemptCount, &item.LastError,
		&item.CreatedAt, &item.UpdatedAt,
		&needsPublish,
	)
	if err != nil {
		return nil, fmt.Errorf("upsert catalog item: %w", err)
	}

	meta, err := unmarshalMetadata(metaRaw)
	if err != nil {
		return nil, err
	}
	item.Metadata = meta

	return &UpsertResult{Item: &item, NeedsPublish: needsPublish}, nil
}

func marshalMetadata(m map[string]any) ([]byte, error) {
	if m == nil {
		return []byte("{}"), nil
	}
	b, err := json.Marshal(m)
	if err != nil {
		return nil, fmt.Errorf("marshal metadata: %w", err)
	}
	return b, nil
}

func unmarshalMetadata(raw []byte) (map[string]any, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("unmarshal metadata: %w", err)
	}
	if len(m) == 0 {
		return nil, nil
	}
	return m, nil
}
