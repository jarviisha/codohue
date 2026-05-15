package admin

import "time"

// NamespaceConfig is the admin view of a namespace configuration.
type NamespaceConfig struct {
	Namespace      string             `json:"namespace"`
	ActionWeights  map[string]float64 `json:"action_weights"`
	Lambda         float64            `json:"lambda"`
	Gamma          float64            `json:"gamma"`
	Alpha          float64            `json:"alpha"`
	MaxResults     int                `json:"max_results"`
	SeenItemsDays  int                `json:"seen_items_days"`
	DenseStrategy  string             `json:"dense_strategy"`
	EmbeddingDim   int                `json:"embedding_dim"`
	DenseDistance  string             `json:"dense_distance"`
	TrendingWindow int                `json:"trending_window"`
	TrendingTTL    int                `json:"trending_ttl"`
	LambdaTrending float64            `json:"lambda_trending"`
	HasAPIKey      bool               `json:"has_api_key"`
	UpdatedAt      time.Time          `json:"updated_at"`
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

// CatalogStrategyConflict is the typed error the service returns when the
// requested combination of dense_strategy and catalog_enabled would have two
// pipelines write to {ns}_objects_dense (cron Phase 2 dense training AND the
// catalog embedder). The handler maps it to 400 with both fields in the body.
//
// dense_strategy ∈ {byoe, disabled} is the only safe pair with catalog_enabled.
type CatalogStrategyConflict struct {
	DenseStrategy  string
	CatalogEnabled bool
}

func (e *CatalogStrategyConflict) Error() string {
	return "dense_strategy conflicts with catalog_enabled"
}

// CatalogReEmbedResponse is the body returned by POST .../catalog/re-embed.
// 202 Accepted; the operator can poll batch_run_logs by ID for progress.
type CatalogReEmbedResponse struct {
	BatchRunID      int64     `json:"batch_run_id"`
	Namespace       string    `json:"namespace"`
	StrategyID      string    `json:"strategy_id"`
	StrategyVersion string    `json:"strategy_version"`
	StaleItems      int       `json:"stale_items"`
	StartedAt       time.Time `json:"started_at"`
}

// CatalogItemSummary is the projection returned in the items list endpoint.
// Includes a bounded content preview for table scanning. Operators fetch the
// full record via GET .../catalog/items/{id} for full content and metadata.
type CatalogItemSummary struct {
	ID              int64      `json:"id"`
	ObjectID        string     `json:"object_id"`
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
type CatalogVector struct {
	Collection string    `json:"collection"`
	NumericID  uint64    `json:"numeric_id"`
	Dim        int       `json:"dim"`
	Values     []float32 `json:"values"`
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
	ActionWeights  map[string]float64 `json:"action_weights"`
	Lambda         *float64           `json:"lambda"`
	Gamma          *float64           `json:"gamma"`
	Alpha          *float64           `json:"alpha"`
	MaxResults     *int               `json:"max_results"`
	SeenItemsDays  *int               `json:"seen_items_days"`
	DenseStrategy  *string            `json:"dense_strategy"`
	EmbeddingDim   *int               `json:"embedding_dim"`
	DenseDistance  *string            `json:"dense_distance"`
	TrendingWindow *int               `json:"trending_window"`
	TrendingTTL    *int               `json:"trending_ttl"`
	LambdaTrending *float64           `json:"lambda_trending"`
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
type BatchRunsResponse struct {
	Items  []BatchRunLog `json:"items"`
	Total  int           `json:"total"`
	Offset int           `json:"offset"`
	Stats  BatchRunStats `json:"stats"`
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

// NamespaceHealth combines config, last batch run, and recent activity into a
// single health record for the overview dashboard.
type NamespaceHealth struct {
	Config          NamespaceConfig `json:"config"`
	Status          NamespaceStatus `json:"status"`
	ActiveEvents24h int             `json:"active_events_24h"`
	LastRun         *BatchRunLog    `json:"last_run"`
}

// NamespacesOverviewResponse is the response for GET /api/admin/v1/namespaces?include=overview.
type NamespacesOverviewResponse struct {
	Items []NamespaceHealth `json:"items"`
	Total int               `json:"total"`
}

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

// DemoDatasetResponse is returned after seeding or clearing the bundled demo dataset.
type DemoDatasetResponse struct {
	Namespace           string `json:"namespace"`
	EventsCreated       int    `json:"events_created,omitempty"`
	EventsDeleted       int    `json:"events_deleted,omitempty"`
	CatalogItemsCreated int    `json:"catalog_items_created,omitempty"`
	APIKey              string `json:"api_key,omitempty"`
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
