package admin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// triggerReembed is the trigger_source value written to batch_run_logs by
// the catalog re-embed orchestrator. Distinct from the "admin" value used by
// CreateBatchRun so the watcher and `running` check can scope precisely to
// re-embed activity rather than every operator-initiated batch.
const triggerReembed = "admin_reembed"

// FindRunningReembedRun returns the active re-embed batch_run_logs row for
// the namespace, or (nil, nil) when none is running. A re-embed is "active"
// when trigger_source='admin_reembed' and completed_at IS NULL.
func (r *Repository) FindRunningReembedRun(ctx context.Context, namespace string) (*BatchRunLog, error) {
	row := r.db.QueryRow(ctx, `
		SELECT id, namespace, started_at, completed_at, duration_ms,
		       subjects_processed, success, error_message, trigger_source, log_lines,
		       phase1_ok, phase1_duration_ms, phase1_subjects, phase1_objects, phase1_error,
		       phase2_ok, phase2_duration_ms, phase2_items,    phase2_subjects, phase2_error,
		       phase3_ok, phase3_duration_ms, phase3_items,    phase3_error
		FROM batch_run_logs
		WHERE namespace = $1
		  AND trigger_source = $2
		  AND completed_at IS NULL
		ORDER BY started_at DESC
		LIMIT 1`,
		namespace, triggerReembed,
	)
	b, err := scanBatchRunLog(row.Scan)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find running reembed: %w", err)
	}
	return &b, nil
}

// InsertReembedRun creates a new batch_run_logs row representing a catalog
// re-embed orchestration. The row is open (completed_at NULL); the watcher
// goroutine in cmd/embedder closes it when the backlog drains.
//
// The error_message column is repurposed to encode the (strategy_id, strategy_version)
// of the re-embed target so the watcher can later derive which version is the
// "new" tag; we use a dedicated prefix "reembed:" to keep the encoding obvious.
func (r *Repository) InsertReembedRun(ctx context.Context, namespace, strategyID, strategyVersion string, startedAt time.Time) (int64, error) {
	target := fmt.Sprintf("reembed:%s/%s", strategyID, strategyVersion)
	var id int64
	err := r.db.QueryRow(ctx, `
		INSERT INTO batch_run_logs (
			namespace, started_at, subjects_processed, success,
			error_message, trigger_source, log_lines
		)
		VALUES ($1, $2, 0, FALSE, $3, $4, '[]'::jsonb)
		RETURNING id`,
		namespace, startedAt, target, triggerReembed,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("insert reembed run: %w", err)
	}
	return id, nil
}

// CompleteReembedRun closes a re-embed batch_run_logs row. success=true clears
// error_message; success=false writes the supplied message.
func (r *Repository) CompleteReembedRun(ctx context.Context, id int64, processed int, success bool, errorMessage string, completedAt time.Time, durationMs int) error {
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
		WHERE id = $1`,
		id, completedAt, durationMs, processed, success, errPtr,
	)
	if err != nil {
		return fmt.Errorf("complete reembed run: %w", err)
	}
	return nil
}

// SelectAndResetStaleCatalogItems atomically marks every catalog item whose
// strategy_version differs from the supplied target back to 'pending' (with
// attempt_count=0, last_error=NULL) and returns their ids and object_ids. The
// IDs are then enqueued to Redis by the caller so the embedder workers pick
// them up under the new strategy version.
//
// Items already in state='pending' or 'in_flight' are left alone — they are
// already on the embedder's path. Items with NULL strategy_version are
// included so first-time embed candidates are not lost.
func (r *Repository) SelectAndResetStaleCatalogItems(ctx context.Context, namespace, targetStrategyVersion string) ([]CatalogReembedTarget, error) {
	rows, err := r.db.Query(ctx, `
		UPDATE catalog_items
		SET state = 'pending',
		    attempt_count = 0,
		    last_error = NULL,
		    updated_at = NOW()
		WHERE namespace = $1
		  AND state IN ('embedded', 'failed', 'dead_letter')
		  AND (strategy_version IS NULL OR strategy_version <> $2)
		RETURNING id, object_id`,
		namespace, targetStrategyVersion,
	)
	if err != nil {
		return nil, fmt.Errorf("reset stale catalog items: %w", err)
	}
	defer rows.Close()

	var out []CatalogReembedTarget
	for rows.Next() {
		var t CatalogReembedTarget
		if err := rows.Scan(&t.ID, &t.ObjectID); err != nil {
			return nil, fmt.Errorf("scan stale catalog item: %w", err)
		}
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate stale catalog items: %w", err)
	}
	return out, nil
}

// CountStaleCatalogItems reports how many catalog_items rows still need to
// finish processing under the target strategy version. Used by the watcher
// in cmd/embedder to detect re-embed completion.
func (r *Repository) CountStaleCatalogItems(ctx context.Context, namespace, targetStrategyVersion string) (int, error) {
	var n int
	err := r.db.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM catalog_items
		WHERE namespace = $1
		  AND state IN ('pending', 'in_flight', 'failed')
		  AND (strategy_version IS NULL OR strategy_version <> $2)`,
		namespace, targetStrategyVersion,
	).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("count stale catalog items: %w", err)
	}
	return n, nil
}

// CatalogItemStateCounts is the Postgres-side breakdown returned by
// CountCatalogItemStates. Counts are exact (single GROUP BY query) and
// scoped to one namespace. Any state not present in catalog_items for the
// namespace stays at zero.
type CatalogItemStateCounts struct {
	Pending    int
	InFlight   int
	Embedded   int
	Failed     int
	DeadLetter int
}

// CountCatalogItemStates returns the per-state row count breakdown of
// catalog_items for one namespace. Used by the admin backlog panel; the
// query is index-served by idx_catalog_items_ns_state.
func (r *Repository) CountCatalogItemStates(ctx context.Context, namespace string) (CatalogItemStateCounts, error) {
	rows, err := r.db.Query(ctx, `
		SELECT state, COUNT(*)
		FROM catalog_items
		WHERE namespace = $1
		GROUP BY state`,
		namespace,
	)
	if err != nil {
		return CatalogItemStateCounts{}, fmt.Errorf("count catalog item states: %w", err)
	}
	defer rows.Close()

	var out CatalogItemStateCounts
	for rows.Next() {
		var (
			state string
			n     int
		)
		if err := rows.Scan(&state, &n); err != nil {
			return CatalogItemStateCounts{}, fmt.Errorf("scan catalog state row: %w", err)
		}
		switch state {
		case "pending":
			out.Pending = n
		case "in_flight":
			out.InFlight = n
		case "embedded":
			out.Embedded = n
		case "failed":
			out.Failed = n
		case "dead_letter":
			out.DeadLetter = n
		}
	}
	if err := rows.Err(); err != nil {
		return CatalogItemStateCounts{}, fmt.Errorf("iterate catalog state rows: %w", err)
	}
	return out, nil
}

// CountEmbeddedAtVersion reports how many catalog_items are in state='embedded'
// at the target strategy_version. Used by the watcher to record progress on
// the batch_run_logs row when it completes.
func (r *Repository) CountEmbeddedAtVersion(ctx context.Context, namespace, targetStrategyVersion string) (int, error) {
	var n int
	err := r.db.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM catalog_items
		WHERE namespace = $1
		  AND state = 'embedded'
		  AND strategy_version = $2`,
		namespace, targetStrategyVersion,
	).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("count embedded items at version: %w", err)
	}
	return n, nil
}

// CatalogReembedTarget is a (id, object_id) pair returned by the stale-item
// query. The orchestration service iterates these to publish XADD entries.
type CatalogReembedTarget struct {
	ID       int64
	ObjectID string
}

// OpenReembedRun is the watcher's view of an active re-embed batch row:
// trigger_source='admin_reembed' AND completed_at IS NULL. The watcher
// goroutine reads this list each tick to decide which namespaces to inspect.
type OpenReembedRun struct {
	ID                    int64
	Namespace             string
	StartedAt             time.Time
	TargetStrategyID      string
	TargetStrategyVersion string
}

// ListOpenReembedRuns returns every namespace currently in the middle of a
// re-embed batch run. The error_message column carries the target strategy
// id+version encoded as "reembed:<id>/<version>" — see InsertReembedRun.
func (r *Repository) ListOpenReembedRuns(ctx context.Context) ([]OpenReembedRun, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, namespace, started_at, error_message
		FROM batch_run_logs
		WHERE trigger_source = $1
		  AND completed_at IS NULL`,
		triggerReembed,
	)
	if err != nil {
		return nil, fmt.Errorf("list open reembed runs: %w", err)
	}
	defer rows.Close()

	var out []OpenReembedRun
	for rows.Next() {
		var (
			row    OpenReembedRun
			target *string
		)
		if err := rows.Scan(&row.ID, &row.Namespace, &row.StartedAt, &target); err != nil {
			return nil, fmt.Errorf("scan open reembed run: %w", err)
		}
		row.TargetStrategyID, row.TargetStrategyVersion = parseReembedTarget(target)
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate open reembed runs: %w", err)
	}
	return out, nil
}

func parseReembedTarget(target *string) (strategyID, strategyVersion string) {
	if target == nil {
		return "", ""
	}
	const prefix = "reembed:"
	s := *target
	if !strings.HasPrefix(s, prefix) {
		return "", ""
	}
	body := strings.TrimPrefix(s, prefix)
	idx := strings.Index(body, "/")
	if idx < 0 {
		return body, ""
	}
	return body[:idx], body[idx+1:]
}

// catalogItemSelectCols is the projection used by both the list and detail
// endpoints so the column order stays consistent across both readers.
const catalogItemSelectCols = `
	SELECT id, namespace, object_id, content, content_hash, metadata,
	       state, COALESCE(strategy_id, ''), COALESCE(strategy_version, ''),
	       embedded_at, attempt_count, COALESCE(last_error, ''),
	       created_at, updated_at
	FROM catalog_items`

// ListCatalogItems returns a paginated browse of catalog items. The state
// filter accepts the canonical state names ("pending", "in_flight",
// "embedded", "failed", "dead_letter") or "all" / "" to disable filtering.
// objectIDFilter is an optional substring match over object_id.
func (r *Repository) ListCatalogItems(ctx context.Context, namespace, state string, limit, offset int, objectIDFilter string) ([]CatalogItemSummary, int, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}
	if offset < 0 {
		offset = 0
	}

	args := []any{namespace}
	conds := []string{"namespace = $1"}
	if state != "" && state != "all" {
		args = append(args, state)
		conds = append(conds, fmt.Sprintf("state = $%d", len(args)))
	}
	if objectIDFilter != "" {
		args = append(args, "%"+objectIDFilter+"%")
		conds = append(conds, fmt.Sprintf("object_id ILIKE $%d", len(args)))
	}
	where := " WHERE " + strings.Join(conds, " AND ")

	var total int
	if err := r.db.QueryRow(ctx, `SELECT COUNT(*) FROM catalog_items`+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count catalog items: %w", err)
	}

	args = append(args, limit, offset)
	rows, err := r.db.Query(ctx, `
		SELECT id, object_id, state,
		       COALESCE(strategy_id, ''), COALESCE(strategy_version, ''),
		       attempt_count, COALESCE(last_error, ''),
		       embedded_at, updated_at
		FROM catalog_items`+where+
		fmt.Sprintf(" ORDER BY updated_at DESC LIMIT $%d OFFSET $%d", len(args)-1, len(args)),
		args...,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("list catalog items: %w", err)
	}
	defer rows.Close()

	out := make([]CatalogItemSummary, 0)
	for rows.Next() {
		var it CatalogItemSummary
		if err := rows.Scan(
			&it.ID, &it.ObjectID, &it.State,
			&it.StrategyID, &it.StrategyVersion,
			&it.AttemptCount, &it.LastError,
			&it.EmbeddedAt, &it.UpdatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("scan catalog item summary: %w", err)
		}
		out = append(out, it)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate catalog items: %w", err)
	}
	return out, total, nil
}

// GetCatalogItem returns the full catalog_items record for (namespace, id).
// Returns (nil, nil) when no row matches so the caller can map to 404.
func (r *Repository) GetCatalogItem(ctx context.Context, namespace string, id int64) (*CatalogItemDetail, error) {
	var (
		d           CatalogItemDetail
		metadataRaw []byte
		contentHash []byte
	)
	err := r.db.QueryRow(ctx, catalogItemSelectCols+`
		WHERE namespace = $1 AND id = $2`,
		namespace, id,
	).Scan(
		&d.ID, &d.Namespace, &d.ObjectID, &d.Content, &contentHash, &metadataRaw,
		&d.State, &d.StrategyID, &d.StrategyVersion,
		&d.EmbeddedAt, &d.AttemptCount, &d.LastError,
		&d.CreatedAt, &d.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get catalog item: %w", err)
	}
	if len(metadataRaw) > 0 && string(metadataRaw) != "null" {
		if err := json.Unmarshal(metadataRaw, &d.Metadata); err != nil {
			return nil, fmt.Errorf("unmarshal metadata: %w", err)
		}
		if len(d.Metadata) == 0 {
			d.Metadata = nil
		}
	}
	return &d, nil
}

// RedriveCatalogItem resets a single catalog item's state to 'pending' so the
// embedder picks it up again. Only items in state 'failed' or 'dead_letter'
// are eligible — operators must not redrive an in-flight item or one that
// is already pending.
//
// Returns:
//   - (item, nil) on success
//   - (nil, nil) when the row is not found OR is in a non-redrivable state
//     (the handler maps both to 404)
func (r *Repository) RedriveCatalogItem(ctx context.Context, namespace string, id int64) (*CatalogItemDetail, error) {
	row := r.db.QueryRow(ctx, `
		UPDATE catalog_items
		SET state = 'pending',
		    attempt_count = 0,
		    last_error = NULL,
		    updated_at = NOW()
		WHERE namespace = $1
		  AND id = $2
		  AND state IN ('failed', 'dead_letter')
		RETURNING id, namespace, object_id, content, content_hash, metadata,
		          state, COALESCE(strategy_id, ''), COALESCE(strategy_version, ''),
		          embedded_at, attempt_count, COALESCE(last_error, ''),
		          created_at, updated_at`,
		namespace, id,
	)
	var (
		d           CatalogItemDetail
		metadataRaw []byte
		contentHash []byte
	)
	err := row.Scan(
		&d.ID, &d.Namespace, &d.ObjectID, &d.Content, &contentHash, &metadataRaw,
		&d.State, &d.StrategyID, &d.StrategyVersion,
		&d.EmbeddedAt, &d.AttemptCount, &d.LastError,
		&d.CreatedAt, &d.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("redrive catalog item: %w", err)
	}
	if len(metadataRaw) > 0 && string(metadataRaw) != "null" {
		if err := json.Unmarshal(metadataRaw, &d.Metadata); err != nil {
			return nil, fmt.Errorf("unmarshal metadata: %w", err)
		}
		if len(d.Metadata) == 0 {
			d.Metadata = nil
		}
	}
	return &d, nil
}

// BulkRedriveDeadletter resets every dead_letter row in the namespace to
// 'pending' and returns their (id, object_id) pairs. The caller publishes
// each pair to the embed stream so the workers process them.
func (r *Repository) BulkRedriveDeadletter(ctx context.Context, namespace string) ([]CatalogReembedTarget, error) {
	rows, err := r.db.Query(ctx, `
		UPDATE catalog_items
		SET state = 'pending',
		    attempt_count = 0,
		    last_error = NULL,
		    updated_at = NOW()
		WHERE namespace = $1
		  AND state = 'dead_letter'
		RETURNING id, object_id`,
		namespace,
	)
	if err != nil {
		return nil, fmt.Errorf("bulk redrive dead-letter: %w", err)
	}
	defer rows.Close()

	var out []CatalogReembedTarget
	for rows.Next() {
		var t CatalogReembedTarget
		if err := rows.Scan(&t.ID, &t.ObjectID); err != nil {
			return nil, fmt.Errorf("scan dead-letter row: %w", err)
		}
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate dead-letter rows: %w", err)
	}
	return out, nil
}

// DeleteCatalogItem removes a catalog_items row and returns the deleted
// object_id so the caller can also remove the matching Qdrant point. found=false
// when no row matched (handler maps to 404 only when neither postgres nor
// qdrant has the object — the actual implementation treats DELETE as
// idempotent and returns 204 in both cases per FR-017).
func (r *Repository) DeleteCatalogItem(ctx context.Context, namespace string, id int64) (objectID string, found bool, err error) {
	err = r.db.QueryRow(ctx, `
		DELETE FROM catalog_items
		WHERE namespace = $1 AND id = $2
		RETURNING object_id`,
		namespace, id,
	).Scan(&objectID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("delete catalog item: %w", err)
	}
	return objectID, true, nil
}

// LookupNumericObjectID returns the BIGSERIAL numeric_id assigned to a string
// object_id under (namespace, entity_type='object'). When no mapping exists
// (the object was never seen by the recommend service), found=false.
func (r *Repository) LookupNumericObjectID(ctx context.Context, namespace, objectID string) (numericID uint64, found bool, err error) {
	err = r.db.QueryRow(ctx, `
		SELECT numeric_id FROM id_mappings
		WHERE namespace = $1 AND entity_type = 'object' AND string_id = $2`,
		namespace, objectID,
	).Scan(&numericID)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("lookup numeric object id: %w", err)
	}
	return numericID, true, nil
}
