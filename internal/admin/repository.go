package admin

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository holds prepared queries against PostgreSQL.
type Repository struct {
	db *pgxpool.Pool
}

// NewRepository creates a new Repository backed by the given connection pool.
func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

const nsSelectCols = `
	SELECT namespace, action_weights, time_decay_factor, gamma, alpha, max_results,
	       seen_items_days, dense_strategy, embedding_dim, dense_distance,
	       trending_window, trending_ttl, lambda_trending,
	       api_key_hash IS NOT NULL AS has_api_key, updated_at
	FROM namespace_configs`

// scanNamespaceConfigRow scans one namespace_configs row.
// action_weights is JSONB — pgx returns it as []byte, so we unmarshal manually.
func scanNamespaceConfigRow(scan func(...any) error) (*NamespaceConfig, error) {
	var (
		ns          NamespaceConfig
		weightsJSON []byte
	)
	err := scan(
		&ns.Namespace, &weightsJSON, &ns.Lambda, &ns.Gamma, &ns.Alpha,
		&ns.MaxResults, &ns.SeenItemsDays, &ns.DenseStrategy, &ns.EmbeddingDim,
		&ns.DenseDistance, &ns.TrendingWindow, &ns.TrendingTTL, &ns.LambdaTrending,
		&ns.HasAPIKey, &ns.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	if len(weightsJSON) > 0 {
		if err := json.Unmarshal(weightsJSON, &ns.ActionWeights); err != nil {
			return nil, fmt.Errorf("unmarshal action_weights: %w", err)
		}
	}
	return &ns, nil
}

// ListNamespaces returns all namespace configurations ordered alphabetically.
func (r *Repository) ListNamespaces(ctx context.Context) ([]NamespaceConfig, error) {
	rows, err := r.db.Query(ctx, nsSelectCols+` ORDER BY namespace`)
	if err != nil {
		return nil, fmt.Errorf("query namespace_configs: %w", err)
	}
	defer rows.Close()

	var out []NamespaceConfig
	for rows.Next() {
		ns, err := scanNamespaceConfigRow(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("scan namespace_config: %w", err)
		}
		out = append(out, *ns)
	}
	return out, rows.Err()
}

// GetNamespace returns a single namespace configuration, or nil if not found.
func (r *Repository) GetNamespace(ctx context.Context, namespace string) (*NamespaceConfig, error) {
	row := r.db.QueryRow(ctx, nsSelectCols+` WHERE namespace = $1`, namespace)
	ns, err := scanNamespaceConfigRow(row.Scan)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("query namespace_config %q: %w", namespace, err)
	}
	return ns, nil
}

// GetSubjectStats returns interaction count, seen items, and the Qdrant numeric
// ID (if one exists in id_mappings) for the given subject.
func (r *Repository) GetSubjectStats(ctx context.Context, namespace, subjectID string, seenItemsDays int) (*SubjectStats, error) {
	var stats SubjectStats

	if err := r.db.QueryRow(ctx,
		`SELECT COUNT(*) FROM events WHERE subject_id = $1 AND namespace = $2`,
		subjectID, namespace,
	).Scan(&stats.InteractionCount); err != nil {
		return nil, fmt.Errorf("count interactions: %w", err)
	}

	rows, err := r.db.Query(ctx,
		`SELECT DISTINCT object_id FROM events
		 WHERE subject_id  = $1
		   AND namespace   = $2
		   AND occurred_at > NOW() - ($3 * INTERVAL '1 day')`,
		subjectID, namespace, seenItemsDays,
	)
	if err != nil {
		return nil, fmt.Errorf("query seen items: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan seen item: %w", err)
		}
		stats.SeenItems = append(stats.SeenItems, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate seen items: %w", err)
	}

	var numericID uint64
	err = r.db.QueryRow(ctx,
		`SELECT numeric_id FROM id_mappings
		 WHERE namespace = $1 AND entity_type = 'subject' AND string_id = $2`,
		namespace, subjectID,
	).Scan(&numericID)
	if err != nil && err != pgx.ErrNoRows {
		return nil, fmt.Errorf("lookup id_mapping: %w", err)
	}
	if err == nil {
		stats.NumericID = &numericID
	}

	return &stats, nil
}

// GetBatchRunLogs returns recent batch run history.
// If namespace is non-empty, results are filtered to that namespace.
// Limit is capped at 50.
func (r *Repository) GetBatchRunLogs(ctx context.Context, namespace string, limit int) ([]BatchRunLog, error) {
	if limit <= 0 || limit > 50 {
		limit = 50
	}

	const base = `
		SELECT id, namespace, started_at, completed_at, duration_ms,
		       subjects_processed, success, error_message,
		       phase1_ok, phase1_duration_ms, phase1_subjects, phase1_objects, phase1_error,
		       phase2_ok, phase2_duration_ms, phase2_items,    phase2_subjects, phase2_error,
		       phase3_ok, phase3_duration_ms, phase3_items,    phase3_error
		FROM batch_run_logs`

	var (
		rows pgx.Rows
		err  error
	)

	if namespace != "" {
		rows, err = r.db.Query(ctx, base+` WHERE namespace = $1 ORDER BY started_at DESC LIMIT $2`, namespace, limit)
	} else {
		rows, err = r.db.Query(ctx, base+` ORDER BY started_at DESC LIMIT $1`, limit)
	}
	if err != nil {
		return nil, fmt.Errorf("query batch_run_logs: %w", err)
	}
	defer rows.Close()

	var out []BatchRunLog
	for rows.Next() {
		b, err := scanBatchRunLog(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("scan batch_run_log: %w", err)
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// scanBatchRunLog scans one batch_run_logs row (all columns including phase breakdown).
func scanBatchRunLog(scan func(...any) error) (BatchRunLog, error) {
	var b BatchRunLog
	err := scan(
		&b.ID, &b.Namespace, &b.StartedAt, &b.CompletedAt,
		&b.DurationMs, &b.SubjectsProcessed, &b.Success, &b.ErrorMessage,
		&b.Phase1OK, &b.Phase1DurMs, &b.Phase1Subjects, &b.Phase1Objects, &b.Phase1Error,
		&b.Phase2OK, &b.Phase2DurMs, &b.Phase2Items, &b.Phase2Subjects, &b.Phase2Error,
		&b.Phase3OK, &b.Phase3DurMs, &b.Phase3Items, &b.Phase3Error,
	)
	return b, err
}

// GetLastBatchRunPerNamespace returns the most recent completed batch run for each
// namespace, keyed by namespace name.
func (r *Repository) GetLastBatchRunPerNamespace(ctx context.Context) (map[string]BatchRunLog, error) {
	rows, err := r.db.Query(ctx, `
		SELECT DISTINCT ON (namespace)
		    id, namespace, started_at, completed_at, duration_ms,
		    subjects_processed, success, error_message,
		    phase1_ok, phase1_duration_ms, phase1_subjects, phase1_objects, phase1_error,
		    phase2_ok, phase2_duration_ms, phase2_items,    phase2_subjects, phase2_error,
		    phase3_ok, phase3_duration_ms, phase3_items,    phase3_error
		FROM batch_run_logs
		WHERE completed_at IS NOT NULL
		ORDER BY namespace, started_at DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("query last batch runs: %w", err)
	}
	defer rows.Close()

	out := make(map[string]BatchRunLog)
	for rows.Next() {
		b, err := scanBatchRunLog(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("scan last batch run: %w", err)
		}
		out[b.Namespace] = b
	}
	return out, rows.Err()
}

// GetRecentEventCounts returns the number of events ingested in the last windowHours
// hours, grouped by namespace.
func (r *Repository) GetRecentEventCounts(ctx context.Context, windowHours int) (map[string]int, error) {
	rows, err := r.db.Query(ctx, `
		SELECT namespace, COUNT(*) AS cnt
		FROM events
		WHERE occurred_at > NOW() - make_interval(hours => $1)
		GROUP BY namespace`,
		windowHours,
	)
	if err != nil {
		return nil, fmt.Errorf("query recent event counts: %w", err)
	}
	defer rows.Close()

	out := make(map[string]int)
	for rows.Next() {
		var ns string
		var cnt int
		if err := rows.Scan(&ns, &cnt); err != nil {
			return nil, fmt.Errorf("scan event count: %w", err)
		}
		out[ns] = cnt
	}
	return out, rows.Err()
}
