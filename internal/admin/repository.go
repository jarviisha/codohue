package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jarviisha/codohue/internal/core/batchrun"
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
	       seen_items_days, dense_source, embedding_dim, dense_distance,
	       trending_window, trending_ttl, lambda_trending,
	       api_key_hash IS NOT NULL AS has_api_key,
	       COALESCE(catalog_strategy_id, ''), COALESCE(catalog_strategy_version, ''),
	       updated_at
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
		&ns.MaxResults, &ns.SeenItemsDays, &ns.DenseSource, &ns.EmbeddingDim,
		&ns.DenseDistance, &ns.TrendingWindow, &ns.TrendingTTL, &ns.LambdaTrending,
		&ns.HasAPIKey,
		&ns.CatalogStrategyID, &ns.CatalogStrategyVersion,
		&ns.UpdatedAt,
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
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate namespace_configs: %w", err)
	}
	return out, nil
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

var statusFilter = map[string]string{
	"running": "completed_at IS NULL",
	"ok":      "success = TRUE",
	"failed":  "completed_at IS NOT NULL AND success = FALSE",
}

// kindFilter maps operator-facing kind names to the underlying batch_run_logs
// trigger_source values. "cf" covers both scheduled cron ticks and admin-
// triggered "Run batch now" actions (both produce phase1/2/3 data). "reembed"
// is the catalog re-embed orchestration, which writes a phase-less row.
//
// Empty kind (lookup miss) leaves the query unfiltered.
var kindFilter = map[string][]string{
	"cf":      {string(batchrun.TriggerCron), string(batchrun.TriggerManual)},
	"reembed": {string(batchrun.TriggerReembed)},
}

// GetBatchRunLogs returns recent batch run history.
// If namespace is non-empty, results are filtered to that namespace.
// Status filters by run state: "running", "ok", "failed", or "" for all.
// Kind filters by batch kind: "cf", "reembed", or "" for all.
// Limit is capped at 100.
func (r *Repository) GetBatchRunLogs(ctx context.Context, namespace, status, kind string, limit, offset int) ([]BatchRunLog, int, BatchRunStats, error) {
	if limit <= 0 || limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}

	// Scope predicates shared between the stats aggregate and the row query:
	// namespace + kind. Status is intentionally NOT here so the stats badges
	// (running/ok/failed) always reflect the full tab, not the inner filter.
	var scopeConds []string
	var scopeArgs []any
	if namespace != "" {
		scopeArgs = append(scopeArgs, namespace)
		scopeConds = append(scopeConds, fmt.Sprintf("namespace = $%d", len(scopeArgs)))
	}
	if triggers, ok := kindFilter[kind]; ok {
		scopeArgs = append(scopeArgs, triggers)
		scopeConds = append(scopeConds, fmt.Sprintf("trigger_source = ANY($%d::text[])", len(scopeArgs)))
	}
	scopeWhere := ""
	if len(scopeConds) > 0 {
		scopeWhere = " WHERE " + strings.Join(scopeConds, " AND ")
	}

	// Aggregate stats — scoped to (namespace, kind) but unfiltered by status.
	var stats BatchRunStats
	statsQuery := `
		SELECT
		  COUNT(*)                                                          AS total,
		  COUNT(*) FILTER (WHERE completed_at IS NULL)                     AS running,
		  COUNT(*) FILTER (WHERE success = TRUE)                           AS ok,
		  COUNT(*) FILTER (WHERE completed_at IS NOT NULL AND success = FALSE) AS failed
		FROM batch_run_logs` + scopeWhere
	if err := r.db.QueryRow(ctx, statsQuery, scopeArgs...).
		Scan(&stats.Total, &stats.Running, &stats.OK, &stats.Failed); err != nil {
		return nil, 0, BatchRunStats{}, fmt.Errorf("count batch_run_stats: %w", err)
	}

	// Build the row WHERE clause — scope + optional status.
	conds := append([]string(nil), scopeConds...)
	args := append([]any(nil), scopeArgs...)
	if clause, ok := statusFilter[status]; ok {
		conds = append(conds, clause)
	}

	where := ""
	if len(conds) > 0 {
		where = " WHERE " + strings.Join(conds, " AND ")
	}

	// Total for the current filter (for pagination).
	var total int
	if err := r.db.QueryRow(ctx, `SELECT COUNT(*) FROM batch_run_logs`+where, args...).Scan(&total); err != nil {
		return nil, 0, BatchRunStats{}, fmt.Errorf("count batch_run_logs: %w", err)
	}

	args = append(args, limit, offset)
	const sel = `
		SELECT id, namespace, started_at, completed_at, duration_ms,
		       entities_processed, success, error_message, trigger_source, cancel_requested, log_lines,
		       phase1_ok, phase1_duration_ms, phase1_subjects, phase1_objects, phase1_error,
		       phase2_ok, phase2_duration_ms, phase2_items,    phase2_subjects, phase2_error,
		       phase3_ok, phase3_duration_ms, phase3_items,    phase3_error,
		       target_strategy_id, target_strategy_version
		FROM batch_run_logs`
	rows, err := r.db.Query(ctx,
		sel+where+fmt.Sprintf(" ORDER BY started_at DESC LIMIT $%d OFFSET $%d", len(args)-1, len(args)),
		args...,
	)
	if err != nil {
		return nil, 0, BatchRunStats{}, fmt.Errorf("query batch_run_logs: %w", err)
	}
	defer rows.Close()

	var out []BatchRunLog
	for rows.Next() {
		b, err := scanBatchRunLog(rows.Scan)
		if err != nil {
			return nil, 0, BatchRunStats{}, fmt.Errorf("scan batch_run_log: %w", err)
		}
		out = append(out, b)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, BatchRunStats{}, fmt.Errorf("iterate batch_run_logs: %w", err)
	}
	return out, total, stats, nil
}

// scanBatchRunLog scans one batch_run_logs row (all columns including phase breakdown).
func scanBatchRunLog(scan func(...any) error) (BatchRunLog, error) {
	var b BatchRunLog
	var rawLog json.RawMessage
	err := scan(
		&b.ID, &b.Namespace, &b.StartedAt, &b.CompletedAt,
		&b.DurationMs, &b.EntitiesProcessed, &b.Success, &b.ErrorMessage, &b.TriggerSource, &b.CancelRequested, &rawLog,
		&b.Phase1OK, &b.Phase1DurMs, &b.Phase1Subjects, &b.Phase1Objects, &b.Phase1Error,
		&b.Phase2OK, &b.Phase2DurMs, &b.Phase2Items, &b.Phase2Subjects, &b.Phase2Error,
		&b.Phase3OK, &b.Phase3DurMs, &b.Phase3Items, &b.Phase3Error,
		&b.TargetStrategyID, &b.TargetStrategyVersion,
	)
	if err != nil {
		return b, err
	}
	if len(rawLog) > 0 && string(rawLog) != "null" {
		if err := json.Unmarshal(rawLog, &b.LogLines); err != nil {
			return b, fmt.Errorf("unmarshal log_lines: %w", err)
		}
	}
	if b.LogLines == nil {
		b.LogLines = []LogEntry{}
	}
	return b, nil
}

// GetBatchRunByID fetches one batch run row by id. Returns (nil, nil) when
// the row does not exist; the service maps that to 404.
func (r *Repository) GetBatchRunByID(ctx context.Context, id int64) (*BatchRunLog, error) {
	const sel = `
		SELECT id, namespace, started_at, completed_at, duration_ms,
		       entities_processed, success, error_message, trigger_source, cancel_requested, log_lines,
		       phase1_ok, phase1_duration_ms, phase1_subjects, phase1_objects, phase1_error,
		       phase2_ok, phase2_duration_ms, phase2_items,    phase2_subjects, phase2_error,
		       phase3_ok, phase3_duration_ms, phase3_items,    phase3_error,
		       target_strategy_id, target_strategy_version
		FROM batch_run_logs WHERE id = $1`
	row := r.db.QueryRow(ctx, sel, id)
	b, err := scanBatchRunLog(row.Scan)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get batch_run_log %d: %w", id, err)
	}
	return &b, nil
}

// RequestCancelResult is the typed outcome of RequestCancel.
type RequestCancelResult int

const (
	// RequestCancelOK means the row was in-flight and is now flagged for cancel.
	RequestCancelOK RequestCancelResult = iota
	// RequestCancelAlreadyTerminal means the run is already completed — UI maps to 409.
	RequestCancelAlreadyTerminal
	// RequestCancelNotFound means the id does not exist — UI maps to 404.
	RequestCancelNotFound
)

// RequestCancel sets cancel_requested on an in-flight batch run row. Cron sees
// the flag at the next phase boundary and tears down the run.
func (r *Repository) RequestCancel(ctx context.Context, id int64) (RequestCancelResult, error) {
	// Disambiguate not-found vs already-terminal so the handler can return
	// 404 vs 409 with confidence rather than guessing from RowsAffected==0.
	var completedAt *time.Time
	if err := r.db.QueryRow(ctx, `SELECT completed_at FROM batch_run_logs WHERE id = $1`, id).Scan(&completedAt); err != nil {
		if err == pgx.ErrNoRows {
			return RequestCancelNotFound, nil
		}
		return 0, fmt.Errorf("lookup batch run %d: %w", id, err)
	}
	if completedAt != nil {
		return RequestCancelAlreadyTerminal, nil
	}
	if _, err := r.db.Exec(ctx, `
		UPDATE batch_run_logs SET cancel_requested = TRUE
		WHERE id = $1 AND completed_at IS NULL
	`, id); err != nil {
		return 0, fmt.Errorf("update cancel_requested %d: %w", id, err)
	}
	return RequestCancelOK, nil
}

// BatchRunStatsBucket is one time-series row from GetBatchRunStats.
type BatchRunStatsBucket struct {
	Ts            time.Time `json:"ts"`
	OK            int       `json:"ok"`
	Failed        int       `json:"failed"`
	Cancelled     int       `json:"cancelled"`
	AvgDurationMs int       `json:"avg_duration_ms"`
}

// GetBatchRunStats aggregates terminal batch runs into buckets of bucketSeconds
// width across the given window ending now. The bucket boundary lives in SQL —
// using date_trunc would not give us arbitrary widths — so the math is done
// inline via div + multiplication on epoch seconds.
func (r *Repository) GetBatchRunStats(ctx context.Context, windowSeconds, bucketSeconds int) ([]BatchRunStatsBucket, error) {
	if windowSeconds <= 0 || bucketSeconds <= 0 {
		return nil, fmt.Errorf("invalid window/bucket: %d / %d", windowSeconds, bucketSeconds)
	}
	rows, err := r.db.Query(ctx, `
		SELECT
		    to_timestamp(floor(extract(epoch FROM started_at)::int / $1) * $1) AS bucket_ts,
		    COUNT(*) FILTER (WHERE success = TRUE)                              AS ok_count,
		    COUNT(*) FILTER (WHERE success = FALSE AND error_message = $3)      AS cancelled_count,
		    COUNT(*) FILTER (WHERE success = FALSE AND error_message IS DISTINCT FROM $3) AS failed_count,
		    COALESCE(AVG(duration_ms), 0)::int                                  AS avg_duration_ms
		FROM batch_run_logs
		WHERE completed_at IS NOT NULL
		  AND started_at > now() - make_interval(secs => $2)
		GROUP BY bucket_ts
		ORDER BY bucket_ts`,
		bucketSeconds, windowSeconds, operatorCancelledMessage,
	)
	if err != nil {
		return nil, fmt.Errorf("query batch_run_stats: %w", err)
	}
	defer rows.Close()

	var out []BatchRunStatsBucket
	for rows.Next() {
		var b BatchRunStatsBucket
		if err := rows.Scan(&b.Ts, &b.OK, &b.Cancelled, &b.Failed, &b.AvgDurationMs); err != nil {
			return nil, fmt.Errorf("scan stats bucket: %w", err)
		}
		out = append(out, b)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate stats: %w", err)
	}
	return out, nil
}

// operatorCancelledMessage is a local alias of [batchrun.OperatorCancelledMessage]
// so SQL fragments in this file stay terse.
const operatorCancelledMessage = batchrun.OperatorCancelledMessage

// GetLastBatchRunPerNamespace returns the most recent completed CF batch run
// per namespace, keyed by namespace name. Re-embed runs (trigger_source=
// 'admin_reembed') are excluded because they don't populate the per-phase
// columns the Overview "last batch run" panel renders — including them would
// surface a phase-less row that looks like an idle cron tick.
func (r *Repository) GetLastBatchRunPerNamespace(ctx context.Context) (map[string]BatchRunLog, error) {
	rows, err := r.db.Query(ctx, `
		SELECT DISTINCT ON (namespace)
		    id, namespace, started_at, completed_at, duration_ms,
		    entities_processed, success, error_message, trigger_source, cancel_requested, log_lines,
		    phase1_ok, phase1_duration_ms, phase1_subjects, phase1_objects, phase1_error,
		    phase2_ok, phase2_duration_ms, phase2_items,    phase2_subjects, phase2_error,
		    phase3_ok, phase3_duration_ms, phase3_items,    phase3_error,
		    target_strategy_id, target_strategy_version
		FROM batch_run_logs
		WHERE completed_at IS NOT NULL
		  AND trigger_source = ANY($1::text[])
		ORDER BY namespace, started_at DESC`,
		[]string{string(batchrun.TriggerCron), string(batchrun.TriggerManual)},
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
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate last batch runs: %w", err)
	}
	return out, nil
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
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate event counts: %w", err)
	}
	return out, nil
}

// subjectOrderBy maps a validated sort key to its ORDER BY clause. subject_id
// is always the final tiebreaker so pagination stays stable across pages when
// last_seen / interaction counts collide.
var subjectOrderBy = map[string]string{
	SubjectSortLastSeen:     "last_seen DESC, subject_id ASC",
	SubjectSortInteractions: "interaction_count DESC, subject_id ASC",
	SubjectSortID:           "subject_id ASC",
}

// escapeLikePrefix neutralises the LIKE wildcards in a user-supplied prefix so
// a subject id containing % or _ matches literally. Pairs with ESCAPE '\'.
func escapeLikePrefix(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return r.Replace(s)
}

// ListSubjects returns a paginated aggregate of the subjects that have events
// in ns. prefix optionally narrows to subject ids starting with it; sort must
// be one of the SubjectSort* constants (the caller validates). Both queries
// ride idx_events_ns_subject_occurred.
//
// Returns the page, the number of distinct subjects matching the filter, and
// any error.
func (r *Repository) ListSubjects(ctx context.Context, ns, prefix, sort string, limit, offset int) ([]SubjectListItem, int, error) {
	orderBy, ok := subjectOrderBy[sort]
	if !ok {
		orderBy = subjectOrderBy[SubjectSortLastSeen]
	}

	// The prefix predicate is appended rather than guarded by a runtime
	// `$n = ''` check: an always-true LIKE '%' would hide the range from the
	// planner, and a bare `$n = ''` gives pgx no column to infer the param
	// type from.
	where := "namespace = $1"
	args := []any{ns}
	if prefix != "" {
		args = append(args, escapeLikePrefix(prefix)+"%")
		where += fmt.Sprintf(` AND subject_id LIKE $%d ESCAPE '\'`, len(args))
	}

	var total int
	if err := r.db.QueryRow(ctx,
		`SELECT COUNT(*) FROM (
		     SELECT 1 FROM events
		     WHERE `+where+`
		     GROUP BY subject_id
		 ) s`,
		args...,
	).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count subjects: %w", err)
	}

	pageArgs := append(append([]any{}, args...), limit, offset)
	rows, err := r.db.Query(ctx,
		fmt.Sprintf(
			`SELECT subject_id, COUNT(*) AS interaction_count, MAX(occurred_at) AS last_seen
			 FROM events
			 WHERE %s
			 GROUP BY subject_id
			 ORDER BY %s
			 LIMIT $%d OFFSET $%d`,
			where, orderBy, len(args)+1, len(args)+2,
		),
		pageArgs...,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("query subjects: %w", err)
	}
	defer rows.Close()

	out := []SubjectListItem{}
	for rows.Next() {
		var it SubjectListItem
		var lastSeen time.Time
		if err := rows.Scan(&it.SubjectID, &it.InteractionCount, &lastSeen); err != nil {
			return nil, 0, fmt.Errorf("scan subject: %w", err)
		}
		it.LastSeen = lastSeen.UTC().Format(time.RFC3339)
		out = append(out, it)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate subjects: %w", err)
	}
	return out, total, nil
}

// GetRecentEvents returns a paginated list of events for a namespace, newest first.
// subjectID is optional — pass empty string to return all subjects.
// Returns the events slice, the total count matching the filter, and any error.
func (r *Repository) GetRecentEvents(ctx context.Context, ns string, limit, offset int, subjectID string) ([]EventSummary, int, error) {
	var total int
	if err := r.db.QueryRow(ctx,
		`SELECT COUNT(*) FROM events WHERE namespace = $1 AND ($2 = '' OR subject_id = $2)`,
		ns, subjectID,
	).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count events: %w", err)
	}

	rows, err := r.db.Query(ctx,
		`SELECT id, namespace, subject_id, object_id, action, weight, occurred_at
		 FROM events
		 WHERE namespace = $1 AND ($2 = '' OR subject_id = $2)
		 ORDER BY occurred_at DESC
		 LIMIT $3 OFFSET $4`,
		ns, subjectID, limit, offset,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("query events: %w", err)
	}
	defer rows.Close()

	var out []EventSummary
	for rows.Next() {
		var e EventSummary
		var occurredAt time.Time
		if err := rows.Scan(&e.ID, &e.Namespace, &e.SubjectID, &e.ObjectID, &e.Action, &e.Weight, &occurredAt); err != nil {
			return nil, 0, fmt.Errorf("scan event: %w", err)
		}
		e.OccurredAt = occurredAt.UTC().Format(time.RFC3339)
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate events: %w", err)
	}
	if out == nil {
		out = []EventSummary{}
	}
	return out, total, nil
}

// GetEventsSummary aggregates events for ns over the trailing windowSecs. It
// returns the total count, per-action counts, and a time series with one entry
// per non-empty bucketSecs bucket (sparse — empty buckets are omitted; the SPA
// fills gaps). Both queries ride the occurred_at index.
func (r *Repository) GetEventsSummary(ctx context.Context, ns string, windowSecs, bucketSecs int) (total int, byAction map[string]int, series []EventsSummaryBucket, err error) {
	byAction = make(map[string]int)

	actionRows, err := r.db.Query(ctx, `
		SELECT action, COUNT(*) AS cnt
		FROM events
		WHERE namespace = $1 AND occurred_at > NOW() - make_interval(secs => $2)
		GROUP BY action`,
		ns, windowSecs,
	)
	if err != nil {
		return 0, nil, nil, fmt.Errorf("query events by action: %w", err)
	}
	defer actionRows.Close()
	for actionRows.Next() {
		var action string
		var cnt int
		if err := actionRows.Scan(&action, &cnt); err != nil {
			return 0, nil, nil, fmt.Errorf("scan action count: %w", err)
		}
		byAction[action] = cnt
		total += cnt
	}
	if err := actionRows.Err(); err != nil {
		return 0, nil, nil, fmt.Errorf("iterate action counts: %w", err)
	}

	seriesRows, err := r.db.Query(ctx, `
		SELECT to_timestamp(floor(extract(epoch FROM occurred_at) / $2) * $2) AS bucket, COUNT(*) AS cnt
		FROM events
		WHERE namespace = $1 AND occurred_at > NOW() - make_interval(secs => $3)
		GROUP BY bucket
		ORDER BY bucket`,
		ns, bucketSecs, windowSecs,
	)
	if err != nil {
		return 0, nil, nil, fmt.Errorf("query events series: %w", err)
	}
	defer seriesRows.Close()
	for seriesRows.Next() {
		var bucket time.Time
		var cnt int
		if err := seriesRows.Scan(&bucket, &cnt); err != nil {
			return 0, nil, nil, fmt.Errorf("scan series bucket: %w", err)
		}
		series = append(series, EventsSummaryBucket{
			Ts:    bucket.UTC().Format(time.RFC3339),
			Count: cnt,
		})
	}
	if err := seriesRows.Err(); err != nil {
		return 0, nil, nil, fmt.Errorf("iterate series: %w", err)
	}
	if series == nil {
		series = []EventsSummaryBucket{}
	}
	return total, byAction, series, nil
}

// SeedDemoEvents replaces the events for the demo namespace with the bundled fixture.
func (r *Repository) SeedDemoEvents(ctx context.Context, namespace string, events []demoEvent, now time.Time) (int, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin seed demo tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // commit path below owns successful completion

	if _, err := tx.Exec(ctx, `DELETE FROM events WHERE namespace = $1`, namespace); err != nil {
		return 0, fmt.Errorf("delete existing demo events: %w", err)
	}

	for _, e := range events {
		if _, err := tx.Exec(ctx, `
			INSERT INTO events (namespace, subject_id, object_id, action, weight, occurred_at, object_created_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			namespace,
			e.SubjectID,
			e.ObjectID,
			e.Action,
			e.Weight,
			demoOccurredAt(now, e.DaysAgo),
			demoOccurredAt(now, e.DaysAgo+14),
		); err != nil {
			return 0, fmt.Errorf("insert demo event: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit seed demo tx: %w", err)
	}
	return len(events), nil
}

// SeedDemoCatalogItems replaces the catalog_items rows for the demo
// namespace with the bundled fixture. Items are inserted in state='pending'
// with the canonical sha256 content_hash so a downstream embedder run picks
// them up the same way a normal data-plane ingest would.
func (r *Repository) SeedDemoCatalogItems(ctx context.Context, namespace string, items []demoCatalogItem, _ time.Time) (int, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin seed catalog tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // commit path below owns successful completion

	if _, err := tx.Exec(ctx, `DELETE FROM catalog_items WHERE namespace = $1`, namespace); err != nil {
		return 0, fmt.Errorf("delete existing catalog items: %w", err)
	}

	for _, it := range items {
		metaBytes, err := json.Marshal(it.Metadata)
		if err != nil {
			return 0, fmt.Errorf("marshal demo catalog metadata for %s: %w", it.ObjectID, err)
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO catalog_items (
				namespace, object_id, content, content_hash, metadata,
				state, attempt_count, created_at, updated_at
			)
			VALUES ($1, $2, $3, $4, $5, 'pending', 0, NOW(), NOW())`,
			namespace, it.ObjectID, it.Content, demoContentHash(it.Content), metaBytes,
		); err != nil {
			return 0, fmt.Errorf("insert demo catalog item %s: %w", it.ObjectID, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit seed catalog tx: %w", err)
	}
	return len(items), nil
}

// TruncateAllNamespaceData wipes every namespace-scoped row from the five
// data tables in one transaction, regardless of whether a matching
// namespace_configs row still exists. Used by the app-wide reset so orphan
// rows (events / id_mappings / batch_run_logs for a namespace whose config
// row was deleted earlier) also get cleaned up. Returns (events_deleted,
// namespaces_deleted).
func (r *Repository) TruncateAllNamespaceData(ctx context.Context) (eventsDeleted, namespacesDeleted int, err error) {
	tx, txErr := r.db.Begin(ctx)
	if txErr != nil {
		return 0, 0, fmt.Errorf("begin reset tx: %w", txErr)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // commit path below owns successful completion

	eventsTag, execErr := tx.Exec(ctx, `DELETE FROM events`)
	if execErr != nil {
		return 0, 0, fmt.Errorf("delete events: %w", execErr)
	}
	if _, execErr := tx.Exec(ctx, `DELETE FROM catalog_items`); execErr != nil {
		return 0, 0, fmt.Errorf("delete catalog_items: %w", execErr)
	}
	if _, execErr := tx.Exec(ctx, `DELETE FROM batch_run_logs`); execErr != nil {
		return 0, 0, fmt.Errorf("delete batch_run_logs: %w", execErr)
	}
	if _, execErr := tx.Exec(ctx, `DELETE FROM id_mappings`); execErr != nil {
		return 0, 0, fmt.Errorf("delete id_mappings: %w", execErr)
	}
	nsTag, execErr := tx.Exec(ctx, `DELETE FROM namespace_configs`)
	if execErr != nil {
		return 0, 0, fmt.Errorf("delete namespace_configs: %w", execErr)
	}

	if commitErr := tx.Commit(ctx); commitErr != nil {
		return 0, 0, fmt.Errorf("commit reset tx: %w", commitErr)
	}
	return int(eventsTag.RowsAffected()), int(nsTag.RowsAffected()), nil
}

// ClearNamespaceData removes all PostgreSQL-owned state for a namespace.
func (r *Repository) ClearNamespaceData(ctx context.Context, namespace string) (int, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin clear namespace tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // commit path below owns successful completion

	tag, err := tx.Exec(ctx, `DELETE FROM events WHERE namespace = $1`, namespace)
	if err != nil {
		return 0, fmt.Errorf("delete events: %w", err)
	}
	eventsDeleted := int(tag.RowsAffected())

	if _, err := tx.Exec(ctx, `DELETE FROM catalog_items WHERE namespace = $1`, namespace); err != nil {
		return 0, fmt.Errorf("delete catalog items: %w", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM batch_run_logs WHERE namespace = $1`, namespace); err != nil {
		return 0, fmt.Errorf("delete batch run logs: %w", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM id_mappings WHERE namespace = $1`, namespace); err != nil {
		return 0, fmt.Errorf("delete id mappings: %w", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM namespace_configs WHERE namespace = $1`, namespace); err != nil {
		return 0, fmt.Errorf("delete namespace config: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit clear namespace tx: %w", err)
	}
	return eventsDeleted, nil
}
