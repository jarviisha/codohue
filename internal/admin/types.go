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

// BatchRunLog is one cron batch cycle record for a namespace.
type BatchRunLog struct {
	ID                int64      `json:"id"`
	Namespace         string     `json:"namespace"`
	StartedAt         time.Time  `json:"started_at"`
	CompletedAt       *time.Time `json:"completed_at"`
	DurationMs        *int       `json:"duration_ms"`
	SubjectsProcessed int        `json:"subjects_processed"`
	Success           bool       `json:"success"`
	ErrorMessage      *string    `json:"error_message"`

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
}

// TrendingAdminEntry extends a trending item with Redis cache TTL info.
type TrendingAdminEntry struct {
	ObjectID    string  `json:"object_id"`
	Score       float64 `json:"score"`
	CacheTTLSec int     `json:"cache_ttl_sec"` // -1 = no expiry, -2 = key missing
}

// LoginRequest is the payload for POST /api/auth/login.
type LoginRequest struct {
	APIKey string `json:"api_key"`
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

// RecommendDebugRequest is the payload for POST /api/admin/v1/recommend/debug.
type RecommendDebugRequest struct {
	Namespace string `json:"namespace"`
	SubjectID string `json:"subject_id"`
	Limit     int    `json:"limit"`
	Offset    int    `json:"offset"`
}

// RecommendDebugItem is a single item in the recommendation debug response.
type RecommendDebugItem struct {
	ObjectID string  `json:"object_id"`
	Score    float64 `json:"score"`
	Rank     int     `json:"rank"`
}

// RecommendDebugResponse is the response for POST /api/admin/v1/recommend/debug.
type RecommendDebugResponse struct {
	SubjectID   string               `json:"subject_id"`
	Namespace   string               `json:"namespace"`
	Items       []RecommendDebugItem `json:"items"`
	Source      string               `json:"source"`
	Limit       int                  `json:"limit"`
	Offset      int                  `json:"offset"`
	Total       int                  `json:"total"`
	GeneratedAt time.Time            `json:"generated_at"`
}

// BatchRunsResponse is the response for GET /api/admin/v1/batch-runs.
type BatchRunsResponse struct {
	Runs []BatchRunLog `json:"runs"`
}

// TrendingAdminResponse is the response for GET /api/admin/v1/trending/{ns}.
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
	Namespaces []NamespaceConfig `json:"namespaces"`
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

// NamespacesOverviewResponse is the response for GET /api/admin/v1/namespaces/overview.
type NamespacesOverviewResponse struct {
	Namespaces []NamespaceHealth `json:"namespaces"`
}

// HealthResponse is the response for GET /api/admin/v1/health.
type HealthResponse struct {
	Postgres string `json:"postgres"`
	Redis    string `json:"redis"`
	Qdrant   string `json:"qdrant"`
	Status   string `json:"status"`
}

// QdrantCollectionStat holds point counts for one Qdrant collection.
type QdrantCollectionStat struct {
	Exists              bool   `json:"exists"`
	PointsCount         uint64 `json:"points_count"`
	IndexedVectorsCount uint64 `json:"indexed_vectors_count"`
}

// QdrantStatsResponse is the response for GET /api/admin/v1/namespaces/{ns}/qdrant-stats.
type QdrantStatsResponse struct {
	Namespace   string                          `json:"namespace"`
	Collections map[string]QdrantCollectionStat `json:"collections"`
}

// TriggerBatchResponse is returned when an on-demand batch run completes.
type TriggerBatchResponse struct {
	BatchRunID int64  `json:"batch_run_id"`
	Namespace  string `json:"namespace"`
	StartedAt  string `json:"started_at"`
	DurationMs int    `json:"duration_ms"`
	Success    bool   `json:"success"`
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
	Events []EventSummary `json:"events"`
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

// SubjectStats holds raw DB data for a subject used internally by Service.
type SubjectStats struct {
	InteractionCount int
	SeenItems        []string
	NumericID        *uint64 // nil if the subject has no Qdrant point yet
}

// SubjectProfileResponse is the response for GET /api/admin/v1/subjects/{ns}/{id}/profile.
type SubjectProfileResponse struct {
	SubjectID        string   `json:"subject_id"`
	Namespace        string   `json:"namespace"`
	InteractionCount int      `json:"interaction_count"`
	SeenItems        []string `json:"seen_items"`
	SeenItemsDays    int      `json:"seen_items_days"`
	SparseVectorNNZ  int      `json:"sparse_vector_nnz"` // -1 means not yet indexed in Qdrant
}
