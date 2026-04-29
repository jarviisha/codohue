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
		       subjects_processed, success, error_message
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
		var b BatchRunLog
		if err := rows.Scan(
			&b.ID, &b.Namespace, &b.StartedAt, &b.CompletedAt,
			&b.DurationMs, &b.SubjectsProcessed, &b.Success, &b.ErrorMessage,
		); err != nil {
			return nil, fmt.Errorf("scan batch_run_log: %w", err)
		}
		out = append(out, b)
	}
	return out, rows.Err()
}
