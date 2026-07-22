package admin

import "time"

// NamespaceConfig is the admin view of a namespace configuration.
//
// Catalog fields surface the catalog auto-embedding state so the admin UI
// can keep the config form in sync with the constraints enforced by the
// backend (see CatalogStrategyConflict and CatalogDimensionMismatch). The
// authoritative catalog configuration still lives behind the dedicated
// /catalog endpoint — these fields are read-only mirrors used to drive UX
// decisions (which dense_source options to disable, whether to surface
// the "managed by catalog" hint on embedding_dim, etc.).
type NamespaceConfig struct {
	Namespace              string             `json:"namespace"`
	ActionWeights          map[string]float64 `json:"action_weights"`
	Lambda                 float64            `json:"lambda"`
	Gamma                  float64            `json:"gamma"`
	Alpha                  float64            `json:"alpha"`
	MaxResults             int                `json:"max_results"`
	SeenItemsDays          int                `json:"seen_items_days"`
	ExcludeAuthored        bool               `json:"exclude_authored"`
	DenseSource            string             `json:"dense_source"`
	EmbeddingDim           int                `json:"embedding_dim"`
	DenseDistance          string             `json:"dense_distance"`
	TrendingWindow         int                `json:"trending_window"`
	TrendingTTL            int                `json:"trending_ttl"`
	LambdaTrending         float64            `json:"lambda_trending"`
	HasAPIKey              bool               `json:"has_api_key"`
	CatalogStrategyID      string             `json:"catalog_strategy_id,omitempty"`
	CatalogStrategyVersion string             `json:"catalog_strategy_version,omitempty"`
	UpdatedAt              time.Time          `json:"updated_at"`
}

// NamespaceCatalogConfig is the per-namespace catalog auto-embedding state.
type NamespaceCatalogConfig struct {
	Namespace       string         `json:"namespace"`
	Enabled         bool           `json:"enabled"`
	StrategyID      string         `json:"strategy_id,omitempty"`
	StrategyVersion string         `json:"strategy_version,omitempty"`
	Params          map[string]any `json:"params,omitempty"`
	EmbeddingDim    int            `json:"embedding_dim"`
	MaxAttempts     int            `json:"max_attempts"`
	MaxContentBytes int            `json:"max_content_bytes"`
	UpdatedAt       time.Time      `json:"updated_at"`
}

// CatalogStrategyDescriptor is one entry in the available_strategies list
// returned to the admin UI. Mirrors embedstrategy.StrategyDescriptor but
// stays in the admin domain so handlers don't need to import embedstrategy
// in their JSON wire types.
type CatalogStrategyDescriptor struct {
	ID            string `json:"id"`
	Version       string `json:"version"`
	Dim           int    `json:"dim"`
	MaxInputBytes int    `json:"max_input_bytes,omitempty"`
	Description   string `json:"description,omitempty"`
}

// CatalogBacklog is a snapshot of operational counts surfaced by
// GET .../catalog. Counts are approximate (sampled, not transactional).
type CatalogBacklog struct {
	Pending    int `json:"pending"`
	InFlight   int `json:"in_flight"`
	Embedded   int `json:"embedded"`
	Failed     int `json:"failed"`
	DeadLetter int `json:"dead_letter"`
	StreamLen  int `json:"stream_len"`
	// ConsumerLag is the embedder consumer-group PEL depth on
	// catalog:embed:{ns} — how many delivered-but-unacked items the worker
	// is still processing or crashed mid-batch on. 0 when Redis is
	// unreachable or the group doesn't exist yet.
	ConsumerLag int `json:"consumer_lag"`
}

// CatalogBacklogSample is one row of catalog_backlog_samples — written by
// cmd/embedder's sampler, read by GET /catalog/backlog-history. Embedded is
// deliberately omitted (the sampler doesn't track it: the timeline is about
// work-still-to-do, not throughput history).
type CatalogBacklogSample struct {
	SampledAt  time.Time `json:"sampled_at"`
	Pending    int       `json:"pending"`
	InFlight   int       `json:"in_flight"`
	Failed     int       `json:"failed"`
	DeadLetter int       `json:"dead_letter"`
	StreamLen  int       `json:"stream_len"`
}

// CatalogBacklogHistoryResponse is the body of
// GET /api/admin/v1/namespaces/{ns}/catalog/backlog-history.
type CatalogBacklogHistoryResponse struct {
	Namespace     string                 `json:"namespace"`
	WindowSeconds int                    `json:"window_seconds"`
	Samples       []CatalogBacklogSample `json:"samples"`
}

// CatalogFailureReason is one bucket of "items that failed for this reason
// in the requested window" — used by the catalog Status page to surface the
// top causes operators should investigate first.
type CatalogFailureReason struct {
	Reason         string `json:"reason"`
	Count          int    `json:"count"`
	SampleObjectID string `json:"sample_object_id,omitempty"`
}

// CatalogFailuresSummaryResponse is the body of
// GET /api/admin/v1/namespaces/{ns}/catalog/failures-summary.
type CatalogFailuresSummaryResponse struct {
	Namespace     string                 `json:"namespace"`
	WindowSeconds int                    `json:"window_seconds"`
	Reasons       []CatalogFailureReason `json:"reasons"`
}

// NamespaceCatalogResponse is the body of GET /api/admin/v1/namespaces/{ns}/catalog.
// available_strategies is filtered to descriptors whose Dim matches the
// namespace's embedding_dim so the operator UI only shows admissible options.
//
// last_embedded_at and last_re_embed are best-effort signals for the Status
// tab; backend read failures surface as nil so the panel still renders.
type NamespaceCatalogResponse struct {
	Catalog             NamespaceCatalogConfig      `json:"catalog"`
	AvailableStrategies []CatalogStrategyDescriptor `json:"available_strategies"`
	Backlog             CatalogBacklog              `json:"backlog"`
	LastEmbeddedAt      *time.Time                  `json:"last_embedded_at,omitempty"`
	LastReEmbed         *CatalogReEmbedSummary      `json:"last_re_embed,omitempty"`
}

// CatalogReEmbedSummary is a compact view of the most recent admin-triggered
// re-embed batch run for a namespace, surfaced on the catalog Status tab so
// operators can confirm their action landed without leaving the page.
//
// Status derivation rules:
//   - completed_at IS NULL                  → "running"
//   - completed_at IS NOT NULL, success=true  → "success"
//   - completed_at IS NOT NULL, success=false → "failed"
//
// strategy_id / strategy_version come from the dedicated target_strategy_*
// columns on batch_run_logs (migration 012). error_message holds only the
// failure reason when success=false.
type CatalogReEmbedSummary struct {
	BatchRunID      int64      `json:"batch_run_id"`
	Status          string     `json:"status"`
	StrategyID      string     `json:"strategy_id,omitempty"`
	StrategyVersion string     `json:"strategy_version,omitempty"`
	StartedAt       time.Time  `json:"started_at"`
	CompletedAt     *time.Time `json:"completed_at,omitempty"`
	DurationMs      int        `json:"duration_ms,omitempty"`
	ProcessedItems  int        `json:"processed_items"`
	ErrorMessage    string     `json:"error_message,omitempty"`
}

// NamespaceCatalogUpdateRequest is the body of PUT /api/admin/v1/namespaces/{ns}/catalog.
// All optional pointer fields preserve the existing value on nil; only Enabled
// is required. When Enabled flips to true, StrategyID and StrategyVersion
// MUST be present (the service rejects with 400 otherwise).
type NamespaceCatalogUpdateRequest struct {
	Enabled         bool           `json:"enabled"`
	StrategyID      *string        `json:"strategy_id,omitempty"`
	StrategyVersion *string        `json:"strategy_version,omitempty"`
	Params          map[string]any `json:"params,omitempty"`
	MaxAttempts     *int           `json:"max_attempts,omitempty"`
	MaxContentBytes *int           `json:"max_content_bytes,omitempty"`
}

// CatalogDimensionMismatch is the typed error the service returns when the
// chosen strategy's natural output dimension does not equal the namespace's
// embedding_dim. The handler maps it to 400 with both numbers in the body.
type CatalogDimensionMismatch struct {
	StrategyDim           int
	NamespaceEmbeddingDim int
}

func (e *CatalogDimensionMismatch) Error() string {
	return "catalog strategy dimension mismatch"
}

// Re-embed state filters accepted in the POST .../catalog/re-embed body.
// An empty value keeps the default: only rows whose strategy_version differs
// from the target. Naming a state re-drives those rows regardless of version,
// which is what makes "rebuild after Qdrant data loss" possible.
const (
	ReembedOnlyStateAll      = "all"
	ReembedOnlyStateEmbedded = "embedded"
	ReembedOnlyStateFailed   = "failed"
)

// CatalogReEmbedRequest is the optional body of POST .../catalog/re-embed.
type CatalogReEmbedRequest struct {
	OnlyState string `json:"only_state,omitempty"`
}

// Valid reports whether OnlyState is empty or one of the accepted filters.
func (r CatalogReEmbedRequest) Valid() bool {
	switch r.OnlyState {
	case "", ReembedOnlyStateAll, ReembedOnlyStateEmbedded, ReembedOnlyStateFailed:
		return true
	default:
		return false
	}
}

// CatalogReEmbedResponse is the body returned by POST .../catalog/re-embed.
// 202 Accepted; the operator can poll batch_run_logs by ID for progress.
type CatalogReEmbedResponse struct {
	BatchRunID      int64     `json:"batch_run_id"`
	Namespace       string    `json:"namespace"`
	StrategyID      string    `json:"strategy_id"`
	StrategyVersion string    `json:"strategy_version"`
	StaleItems      int       `json:"stale_items"`
	OnlyState       string    `json:"only_state,omitempty"`
	StartedAt       time.Time `json:"started_at"`
}

// CatalogItemSummary is the projection returned in the items list endpoint.
// Includes a bounded content preview for table scanning. Operators fetch the
// full record via GET .../catalog/items/{id} for full content and metadata.
type CatalogItemSummary struct {
	ID              int64      `json:"id"`
	ObjectID        string     `json:"object_id"`
	AuthorSubjectID string     `json:"author_subject_id,omitempty"`
	ContentPreview  string     `json:"content_preview,omitempty"`
	State           string     `json:"state"`
	StrategyID      string     `json:"strategy_id,omitempty"`
	StrategyVersion string     `json:"strategy_version,omitempty"`
	AttemptCount    int        `json:"attempt_count"`
	LastError       string     `json:"last_error,omitempty"`
	EmbeddedAt      *time.Time `json:"embedded_at,omitempty"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

// CatalogItemDetail is the full catalog_items row including content+metadata.
type CatalogItemDetail struct {
	CatalogItemSummary
	Namespace string         `json:"namespace"`
	Content   string         `json:"content"`
	Metadata  map[string]any `json:"metadata,omitempty"`
	Vector    *CatalogVector `json:"vector,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
}

// CatalogVector is the dense object vector stored in Qdrant for a catalog item.
// Preview carries only the leading dims (Dim reports the true dimensionality) —
// operators eyeball it to spot degenerate vectors, so shipping all 768 floats
// to the browser would be pure waste.
type CatalogVector struct {
	Collection string    `json:"collection"`
	NumericID  uint64    `json:"numeric_id"`
	Dim        int       `json:"dim"`
	Preview    []float32 `json:"preview"`
}

// CatalogItemsListResponse paginates a list of catalog items.
type CatalogItemsListResponse struct {
	Items  []CatalogItemSummary `json:"items"`
	Total  int                  `json:"total"`
	Limit  int                  `json:"limit"`
	Offset int                  `json:"offset"`
}

// CatalogRedriveResponse is the body of single-item redrive (202 Accepted).
type CatalogRedriveResponse struct {
	ID       int64  `json:"id"`
	ObjectID string `json:"object_id"`
	State    string `json:"state"`
}

// CatalogBulkRedriveResponse is the body of bulk dead-letter redrive.
type CatalogBulkRedriveResponse struct {
	Namespace string `json:"namespace"`
	Redriven  int    `json:"redriven"`
}

// LogEntry is a single captured log line from a batch run.
type LogEntry struct {
	Ts    string `json:"ts"`
	Level string `json:"level"`
	Msg   string `json:"msg"`
}

// BatchRunLog is one cron batch cycle record for a namespace.
type BatchRunLog struct {
	ID                int64      `json:"id"`
	Namespace         string     `json:"namespace"`
	StartedAt         time.Time  `json:"started_at"`
	CompletedAt       *time.Time `json:"completed_at"`
	DurationMs        *int       `json:"duration_ms"`
	EntitiesProcessed int        `json:"entities_processed"`
	Success           bool       `json:"success"`
	ErrorMessage      *string    `json:"error_message"`
	TriggerSource     string     `json:"trigger_source"`
	CancelRequested   bool       `json:"cancel_requested"`
	LogLines          []LogEntry `json:"log_lines"`

	// Per-phase breakdown (nil when the phase was skipped or the row predates migration 007).
	Phase1OK       *bool   `json:"phase1_ok"`
	Phase1DurMs    *int    `json:"phase1_duration_ms"`
	Phase1Subjects *int    `json:"phase1_subjects"`
	Phase1Objects  *int    `json:"phase1_objects"`
	Phase1Error    *string `json:"phase1_error"`

	Phase2OK       *bool   `json:"phase2_ok"`
	Phase2DurMs    *int    `json:"phase2_duration_ms"`
	Phase2Items    *int    `json:"phase2_items"`
	Phase2Subjects *int    `json:"phase2_subjects"`
	Phase2Error    *string `json:"phase2_error"`

	Phase3OK    *bool   `json:"phase3_ok"`
	Phase3DurMs *int    `json:"phase3_duration_ms"`
	Phase3Items *int    `json:"phase3_items"`
	Phase3Error *string `json:"phase3_error"`

	// Re-embed runs only — the target (strategy_id, strategy_version) the
	// run was kicked off against. NULL for cron/manual rows.
	TargetStrategyID      *string `json:"target_strategy_id,omitempty"`
	TargetStrategyVersion *string `json:"target_strategy_version,omitempty"`
}

// TrendingAdminEntry extends a trending item with Redis cache TTL info.
type TrendingAdminEntry struct {
	ObjectID    string  `json:"object_id"`
	Score       float64 `json:"score"`
	CacheTTLSec int     `json:"cache_ttl_sec"` // -1 = no expiry, -2 = key missing
}

// CreateSessionRequest is the payload for POST /api/v1/auth/sessions.
type CreateSessionRequest struct {
	APIKey string `json:"api_key"`
}

// CreateSessionResponse is the body of a successful session creation.
type CreateSessionResponse struct {
	ExpiresAt time.Time `json:"expires_at"`
}

// NamespaceUpsertRequest is the payload for PUT /api/admin/v1/namespaces/{ns}.
type NamespaceUpsertRequest struct {
	ActionWeights   map[string]float64 `json:"action_weights"`
	Lambda          *float64           `json:"lambda"`
	Gamma           *float64           `json:"gamma"`
	Alpha           *float64           `json:"alpha"`
	MaxResults      *int               `json:"max_results"`
	SeenItemsDays   *int               `json:"seen_items_days"`
	ExcludeAuthored *bool              `json:"exclude_authored"`
	DenseSource     *string            `json:"dense_source"`
	EmbeddingDim    *int               `json:"embedding_dim"`
	DenseDistance   *string            `json:"dense_distance"`
	TrendingWindow  *int               `json:"trending_window"`
	TrendingTTL     *int               `json:"trending_ttl"`
	LambdaTrending  *float64           `json:"lambda_trending"`
}

// NamespaceUpsertResponse is the response for PUT /api/admin/v1/namespaces/{ns}.
type NamespaceUpsertResponse struct {
	Namespace string    `json:"namespace"`
	UpdatedAt time.Time `json:"updated_at"`
	APIKey    *string   `json:"api_key,omitempty"` // non-nil only on first create
}

// RecommendDebug contains additional operator-only recommendation diagnostics.
type RecommendDebug struct {
	SparseNNZ        int     `json:"sparse_nnz"`
	DenseScore       float64 `json:"dense_score"`
	Alpha            float64 `json:"alpha"`
	SeenItemsCount   int     `json:"seen_items_count"`
	InteractionCount int     `json:"interaction_count"`
}

// RecommendDebugItem is a single item in the recommendation debug response.
type RecommendDebugItem struct {
	ObjectID string  `json:"object_id"`
	Score    float64 `json:"score"`
	Rank     int     `json:"rank"`
}

// RecommendResponse is the body returned by the admin recommendations
// sub-resource endpoint. The Debug block is populated only when debug=true.
type RecommendResponse struct {
	SubjectID   string               `json:"subject_id"`
	Namespace   string               `json:"namespace"`
	Items       []RecommendDebugItem `json:"items"`
	Source      string               `json:"source"`
	Limit       int                  `json:"limit"`
	Offset      int                  `json:"offset"`
	Total       int                  `json:"total"`
	GeneratedAt time.Time            `json:"generated_at"`
	Debug       *RecommendDebug      `json:"debug,omitempty"`
}

// BatchRunStats holds aggregate counts across all statuses for a namespace.
type BatchRunStats struct {
	Total   int `json:"total"`
	Running int `json:"running"`
	OK      int `json:"ok"`
	Failed  int `json:"failed"`
}

// BatchRunsResponse is the response for GET /api/admin/v1/batch-runs.
// Items are summaries (no log_lines / no full per-phase blob); operators
// click into a row to fetch the BatchRunDetail via /batch-runs/{id}.
type BatchRunsResponse struct {
	Items  []BatchRunSummary `json:"items"`
	Total  int               `json:"total"`
	Offset int               `json:"offset"`
	Stats  BatchRunStats     `json:"stats"`
}

// TrendingAdminResponse is the response for GET /api/admin/v1/namespaces/{ns}/trending.
type TrendingAdminResponse struct {
	Namespace   string               `json:"namespace"`
	Items       []TrendingAdminEntry `json:"items"`
	WindowHours int                  `json:"window_hours"`
	Limit       int                  `json:"limit"`
	Offset      int                  `json:"offset"`
	Total       int                  `json:"total"`
	CacheTTLSec int                  `json:"cache_ttl_sec"`
	GeneratedAt time.Time            `json:"generated_at"`
}

// NamespacesListResponse is the response for GET /api/admin/v1/namespaces.
type NamespacesListResponse struct {
	Items []NamespaceConfig `json:"items"`
	Total int               `json:"total"`
}

// NamespaceStatus is the computed health state for a namespace.
// Values: "active", "idle", "degraded", "cold".
//   - active   — last batch run succeeded and there are events in the last 24 h
//   - idle     — last batch run succeeded but no events in the last 24 h
//   - degraded — last completed batch run failed
//   - cold     — no completed batch run exists yet
type NamespaceStatus = string

// NSStatus* constants enumerate the possible namespace health states.
const (
	NSStatusActive   NamespaceStatus = "active"
	NSStatusIdle     NamespaceStatus = "idle"
	NSStatusDegraded NamespaceStatus = "degraded"
	NSStatusCold     NamespaceStatus = "cold"
)

// HealthResponse is the response for GET /api/admin/v1/health.
type HealthResponse struct {
	Postgres string `json:"postgres"`
	Redis    string `json:"redis"`
	Qdrant   string `json:"qdrant"`
	Status   string `json:"status"`
}

// QdrantCollection holds point counts for one Qdrant collection.
type QdrantCollection struct {
	Exists      bool   `json:"exists"`
	PointsCount uint64 `json:"points_count"`
}

// QdrantInspectResponse is the response for GET /api/admin/v1/namespaces/{ns}/qdrant.
type QdrantInspectResponse struct {
	Subjects      QdrantCollection `json:"subjects"`
	Objects       QdrantCollection `json:"objects"`
	SubjectsDense QdrantCollection `json:"subjects_dense"`
	ObjectsDense  QdrantCollection `json:"objects_dense"`
}

// BatchRunCreateResponse is returned when an on-demand batch run is accepted.
type BatchRunCreateResponse struct {
	ID        int64     `json:"id"`
	Namespace string    `json:"namespace"`
	Status    string    `json:"status"`
	StartedAt time.Time `json:"started_at"`
}

// EventSummary is a single event row for the admin events list.
type EventSummary struct {
	ID         int64   `json:"id"`
	Namespace  string  `json:"namespace"`
	SubjectID  string  `json:"subject_id"`
	ObjectID   string  `json:"object_id"`
	Action     string  `json:"action"`
	Weight     float64 `json:"weight"`
	OccurredAt string  `json:"occurred_at"`
}

// EventsListResponse wraps a page of events with pagination metadata.
type EventsListResponse struct {
	Items  []EventSummary `json:"items"`
	Total  int            `json:"total"`
	Limit  int            `json:"limit"`
	Offset int            `json:"offset"`
}

// InjectEventRequest is the payload for the admin event injection endpoint.
type InjectEventRequest struct {
	SubjectID  string  `json:"subject_id"`
	ObjectID   string  `json:"object_id"`
	Action     string  `json:"action"`
	OccurredAt *string `json:"occurred_at,omitempty"`
}

// InjectEventResponse is returned by POST /api/admin/v1/namespaces/{ns}/events.
// event_id is the id of the freshly-persisted row so the SPA can highlight it
// in the live tail.
type InjectEventResponse struct {
	OK      bool  `json:"ok"`
	EventID int64 `json:"event_id"`
}

// EventsSummaryAction is one action's share of ingest within the summary window.
type EventsSummaryAction struct {
	Action string  `json:"action"`
	Count  int     `json:"count"`
	Rate   float64 `json:"rate"`
}

// EventsSummaryBucket is one time-bucket of the events-over-time series.
type EventsSummaryBucket struct {
	Ts    string `json:"ts"`
	Count int    `json:"count"`
}

// EventsSummaryResponse is the server-side aggregation backing the events tail
// sidebar (GET /api/admin/v1/namespaces/{ns}/events/summary).
type EventsSummaryResponse struct {
	WindowSeconds int                   `json:"window_seconds"`
	BucketSeconds int                   `json:"bucket_seconds"`
	Total         int                   `json:"total"`
	RatePerSecond float64               `json:"rate_per_second"`
	ByAction      []EventsSummaryAction `json:"by_action"`
	Series        []EventsSummaryBucket `json:"series"`
}

// MetricsSummaryIngest is the ingest slice of /metrics/summary — per-namespace
// events-per-second derived from the admin-plane rate tracker (not a cross-
// process Prometheus scrape).
type MetricsSummaryIngest struct {
	EventsPerSec1m map[string]float64 `json:"events_per_sec_1m"`
	EventsPerSec5m map[string]float64 `json:"events_per_sec_5m"`
}

// MetricsSummaryCron is the cron slice — current batch-job lag.
type MetricsSummaryCron struct {
	BatchLagSeconds float64 `json:"batch_lag_seconds"`
}

// MetricsSummaryResponse is the curated rolling-window metrics view
// (GET /api/admin/v1/metrics/summary). Recommend/embedder slices are omitted:
// those counters live in separate processes (cmd/api, cmd/embedder) and are
// scraped by Prometheus/Grafana directly rather than proxied here.
type MetricsSummaryResponse struct {
	GeneratedAt string               `json:"generated_at"`
	Ingest      MetricsSummaryIngest `json:"ingest"`
	Cron        MetricsSummaryCron   `json:"cron"`
}

// DemoDatasetResponse is returned after seeding or clearing the bundled demo dataset.
type DemoDatasetResponse struct {
	Namespace           string `json:"namespace"`
	EventsCreated       int    `json:"events_created,omitempty"`
	EventsDeleted       int    `json:"events_deleted,omitempty"`
	CatalogItemsCreated int    `json:"catalog_items_created,omitempty"`
	APIKey              string `json:"api_key,omitempty"`
}

// NamespaceDeleteResponse is returned by DELETE /api/admin/v1/namespaces/{ns}
// after the namespace and all of its data have been wiped from PostgreSQL,
// Redis, and Qdrant.
type NamespaceDeleteResponse struct {
	Namespace     string `json:"namespace"`
	EventsDeleted int    `json:"events_deleted"`
}

// ResetAppRequest is the body of POST /api/admin/v1/reset. The operator must
// type the literal string "RESET" to confirm the destructive action.
type ResetAppRequest struct {
	Confirm string `json:"confirm"`
}

// ResetAppResponse summarises a successful app-wide reset.
type ResetAppResponse struct {
	NamespacesDeleted int      `json:"namespaces_deleted"`
	EventsDeleted     int      `json:"events_deleted"`
	Namespaces        []string `json:"namespaces"`
}

// ----------------------------------------------------------------------------
// Phase 0 schema for the redesigned admin API.
//
// These types back the new aggregate + lifecycle endpoints introduced in
// BUILD_PLAN_WEB_ADMIN_V2.md §3. Existing types above remain in use by the
// v1 handlers; the new types are wired up as their handlers land per phase.
// ----------------------------------------------------------------------------

// PhaseEntry is one phase result inside a BatchRunDetail. The mapping of OK
// + Skipped follows BUILD_PLAN §3.2:
//
//	OK = true,  Skipped = nil          → phase ran and succeeded.
//	OK = false, Error    != nil        → phase ran and failed.
//	OK = nil,   Skipped  != nil        → phase was skipped (reason in Skipped).
//
// Per-phase metric fields (Subjects/Objects/Items) use pointer + omitempty so
// JSON only carries the metrics that apply to the named phase.
type PhaseEntry struct {
	N          int     `json:"n"`
	Name       string  `json:"name"`
	OK         *bool   `json:"ok"`
	Skipped    *string `json:"skipped"`
	DurationMs int     `json:"duration_ms"`
	Subjects   *int    `json:"subjects,omitempty"`
	Objects    *int    `json:"objects,omitempty"`
	Items      *int    `json:"items,omitempty"`
	Error      *string `json:"error"`
}

// TargetStrategy identifies the embed strategy a re-embed batch run was kicked
// off against. Nil on cron / manual CF runs.
type TargetStrategy struct {
	ID      string `json:"id"`
	Version string `json:"version"`
}

// BatchRunSummary is the lightweight row returned in batch-run list endpoints.
// PhaseStatus[i] ∈ "ok" | "fail" | "skipped" | nil (nil = phase not yet run /
// run still in flight). UI renders it as three Badge tones (Davinci).
type BatchRunSummary struct {
	ID                int64      `json:"id"`
	Namespace         string     `json:"namespace"`
	Kind              string     `json:"kind"` // "cf" | "reembed"
	TriggerSource     string     `json:"trigger_source"`
	StartedAt         time.Time  `json:"started_at"`
	CompletedAt       *time.Time `json:"completed_at"`
	DurationMs        *int       `json:"duration_ms"`
	Success           bool       `json:"success"`
	CancelRequested   bool       `json:"cancel_requested"`
	EntitiesProcessed int        `json:"entities_processed"`
	PhaseStatus       [3]*string `json:"phase_status"`
	ErrorMessage      *string    `json:"error_message"`
}

// BatchRunDetail is the full payload for the single run detail page. Embeds
// BatchRunSummary and adds the full per-phase breakdown, captured log lines,
// and target strategy (re-embed only).
type BatchRunDetail struct {
	BatchRunSummary
	Phases         []PhaseEntry    `json:"phases"`
	LogLines       []LogEntry      `json:"log_lines"`
	TargetStrategy *TargetStrategy `json:"target_strategy"`
}

// Alert is one entry on OverviewResponse.Alerts. Levels: "warn" | "error".
// Kinds match the generation rules in BUILD_PLAN §3.1.
type Alert struct {
	Level     string `json:"level"`
	Namespace string `json:"namespace,omitempty"`
	Kind      string `json:"kind"`
	Message   string `json:"message"`
}

// CronHeartbeat reports cron liveness on /overview.
type CronHeartbeat struct {
	LastRunAt  *time.Time `json:"last_run_at"`
	LagSeconds int        `json:"lag_seconds"`
	OK         bool       `json:"ok"`
}

// EmbedderHeartbeat reports embedder liveness on /overview.
type EmbedderHeartbeat struct {
	LastSeenAt *time.Time `json:"last_seen_at"`
	OK         bool       `json:"ok"`
}

// NamespaceOverviewRun is the compact last-run snapshot inside NamespaceOverview.
type NamespaceOverviewRun struct {
	ID          int64      `json:"id"`
	StartedAt   time.Time  `json:"started_at"`
	Success     bool       `json:"success"`
	PhaseStatus [3]*string `json:"phase_status"`
}

// NamespaceOverviewCatalog is the catalog summary inside NamespaceOverview.
type NamespaceOverviewCatalog struct {
	Enabled    bool `json:"enabled"`
	Pending    int  `json:"pending"`
	DeadLetter int  `json:"dead_letter"`
}

// NamespaceOverviewQdrant is the Qdrant point-count summary inside NamespaceOverview.
type NamespaceOverviewQdrant struct {
	Subjects uint64 `json:"subjects"`
	Objects  uint64 `json:"objects"`
}

// NamespaceOverview is one row in OverviewResponse.Namespaces.
type NamespaceOverview struct {
	Namespace       string                   `json:"namespace"`
	Status          NamespaceStatus          `json:"status"`
	LastRun         *NamespaceOverviewRun    `json:"last_run"`
	Events24h       int                      `json:"events_24h"`
	EventsPerMinNow float64                  `json:"events_per_min_now"`
	Catalog         NamespaceOverviewCatalog `json:"catalog"`
	Qdrant          NamespaceOverviewQdrant  `json:"qdrant"`
}

// OverviewResponse is the body of GET /api/admin/v1/overview — a single
// payload that drives the Fleet overview page.
type OverviewResponse struct {
	GeneratedAt       time.Time           `json:"generated_at"`
	Health            HealthResponse      `json:"health"`
	CronHeartbeat     CronHeartbeat       `json:"cron_heartbeat"`
	EmbedderHeartbeat EmbedderHeartbeat   `json:"embedder_heartbeat"`
	Alerts            []Alert             `json:"alerts"`
	Namespaces        []NamespaceOverview `json:"namespaces"`
}

// NamespaceDashboardResponse is the body of
// GET /api/admin/v1/namespaces/{ns}/dashboard — everything the /ns/:ns page
// needs in one round-trip.
type NamespaceDashboardResponse struct {
	Namespace       string                `json:"namespace"`
	GeneratedAt     time.Time             `json:"generated_at"`
	Config          NamespaceConfig       `json:"config"`
	LastRuns        []BatchRunSummary     `json:"last_runs"` // up to 12 most recent
	Catalog         CatalogBacklog        `json:"catalog"`
	Events24h       int                   `json:"events_24h"`
	EventsPerMinNow float64               `json:"events_per_min_now"`
	Qdrant          QdrantInspectResponse `json:"qdrant"`
	TrendingTTLSec  int                   `json:"trending_ttl_sec"`
	AuthorCoverage  AuthorCoverage        `json:"author_coverage"`
}

// AuthorCoverage reports how many of the namespace's catalog items carry an
// author. The exclude_authored filter reads author_subject_id and silently
// does nothing when no item has one, so the config page needs this to tell
// the operator whether the toggle can act on anything.
type AuthorCoverage struct {
	Attributed int `json:"attributed"`
	Total      int `json:"total"`
}

// SubjectStats holds raw DB data for a subject used internally by Service.
type SubjectStats struct {
	InteractionCount int
	SeenItems        []string
	NumericID        *uint64 // nil if the subject has no Qdrant point yet
}

// SubjectProfileResponse is the response for GET /api/admin/v1/namespaces/{ns}/subjects/{id}/profile.
type SubjectProfileResponse struct {
	SubjectID        string   `json:"subject_id"`
	Namespace        string   `json:"namespace"`
	InteractionCount int      `json:"interaction_count"`
	SeenItems        []string `json:"seen_items"`
	SeenItemsDays    int      `json:"seen_items_days"`
	SparseVectorNNZ  int      `json:"sparse_vector_nnz"` // -1 means not yet indexed in Qdrant
}

// Sort orders accepted by GET /api/admin/v1/namespaces/{ns}/subjects.
const (
	SubjectSortLastSeen     = "last_seen"
	SubjectSortInteractions = "interactions"
	SubjectSortID           = "subject_id"
)

// SubjectListItem is one row of GET /api/admin/v1/namespaces/{ns}/subjects.
// Subjects are not a stored resource — each row is an aggregate over the
// events table, so there is no created_at and no id beyond subject_id itself.
type SubjectListItem struct {
	SubjectID        string `json:"subject_id"`
	InteractionCount int    `json:"interaction_count"`
	LastSeen         string `json:"last_seen"`
}

// SubjectsListResponse is the response for GET /api/admin/v1/namespaces/{ns}/subjects.
type SubjectsListResponse struct {
	Items  []SubjectListItem `json:"items"`
	Total  int               `json:"total"` // distinct subjects matching the filter
	Limit  int               `json:"limit"`
	Offset int               `json:"offset"`
	Sort   string            `json:"sort"`
}
