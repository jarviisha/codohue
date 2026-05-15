export interface NamespaceConfig {
  namespace: string
  action_weights: Record<string, number>
  lambda: number
  gamma: number
  alpha: number
  max_results: number
  seen_items_days: number
  dense_strategy: string
  embedding_dim: number
  dense_distance: string
  trending_window: number
  trending_ttl: number
  lambda_trending: number
  has_api_key: boolean
  updated_at: string
}

export interface NamespaceListResponse {
  items: NamespaceConfig[]
  total: number
}

export interface UpsertNamespacePayload {
  action_weights?: Record<string, number>
  lambda?: number
  gamma?: number
  alpha?: number
  max_results?: number
  seen_items_days?: number
  dense_strategy?: string
  embedding_dim?: number
  dense_distance?: string
  trending_window?: number
  trending_ttl?: number
  lambda_trending?: number
}

export interface UpsertNamespaceResponse {
  namespace: string
  updated_at: string
  api_key?: string
}

export type NamespaceStatus = 'active' | 'idle' | 'degraded' | 'cold'

export interface NamespaceHealth {
  config: NamespaceConfig
  status: NamespaceStatus
  active_events_24h: number
  last_run: BatchRunLog | null
}

export interface NamespacesOverviewResponse {
  items: NamespaceHealth[]
  total: number
}

export interface LogEntry {
  ts: string
  level: 'info' | 'warn' | 'error'
  msg: string
}

export interface BatchRunLog {
  id: number
  namespace: string
  started_at: string
  completed_at: string | null
  duration_ms: number | null
  subjects_processed: number
  success: boolean
  error_message: string | null
  trigger_source: 'cron' | 'manual' | 'admin' | 'admin_reembed' | string
  log_lines: LogEntry[]

  phase1_ok: boolean | null
  phase1_duration_ms: number | null
  phase1_subjects: number | null
  phase1_objects: number | null
  phase1_error: string | null

  phase2_ok: boolean | null
  phase2_duration_ms: number | null
  phase2_items: number | null
  phase2_subjects: number | null
  phase2_error: string | null

  phase3_ok: boolean | null
  phase3_duration_ms: number | null
  phase3_items: number | null
  phase3_error: string | null
}

export interface BatchRunStats {
  total: number
  running: number
  ok: number
  failed: number
}

export interface BatchRunsResponse {
  items: BatchRunLog[]
  total: number
  offset: number
  stats: BatchRunStats
}

export interface EventSummary {
  id: number
  namespace: string
  subject_id: string
  object_id: string
  action: string
  weight: number
  occurred_at: string
}

export interface EventsListResponse {
  items: EventSummary[]
  total: number
  limit: number
  offset: number
}

export interface InjectEventRequest {
  subject_id: string
  object_id: string
  action: string
  occurred_at?: string
}

export interface MutationOkResponse {
  ok: boolean
}

export interface DemoDatasetResponse {
  namespace: string
  events_created?: number
  events_deleted?: number
  catalog_items_created?: number
  api_key?: string
}

export interface HealthData {
  postgres: string
  redis: string
  qdrant: string
  status: string
}

export interface QdrantCollection {
  exists: boolean
  points_count: number
}

export interface QdrantInspectResponse {
  subjects: QdrantCollection
  objects: QdrantCollection
  subjects_dense: QdrantCollection
  objects_dense: QdrantCollection
}

export interface RecommendDebugRequest {
  namespace: string
  subject_id: string
  limit: number
  offset: number
}

export interface RecommendDebugItem {
  object_id: string
  score: number
  rank: number
}

export interface RecommendDebug {
  sparse_nnz: number
  dense_score: number
  alpha: number
  seen_items_count: number
  interaction_count: number
}

export interface RecommendResponse {
  subject_id: string
  namespace: string
  items: RecommendDebugItem[]
  source: string
  limit: number
  offset: number
  total: number
  generated_at: string
  debug?: RecommendDebug
}

export interface SubjectProfileRequest {
  namespace: string
  subject_id: string
}

export interface SubjectProfileResponse {
  subject_id: string
  namespace: string
  interaction_count: number
  seen_items: string[]
  seen_items_days: number
  sparse_vector_nnz: number
}

export interface TrendingAdminEntry {
  object_id: string
  score: number
  cache_ttl_sec: number
}

export interface TrendingAdminResponse {
  namespace: string
  items: TrendingAdminEntry[]
  window_hours: number
  limit: number
  offset: number
  total: number
  cache_ttl_sec: number
  generated_at: string
}

export interface BatchRunCreateResponse {
  id: number
  namespace: string
  status: string
  started_at: string
}

// ─── Catalog auto-embedding (feature 004) ─────────────────────────────────────

export interface NamespaceCatalogConfig {
  namespace: string
  enabled: boolean
  strategy_id?: string
  strategy_version?: string
  params?: Record<string, unknown>
  embedding_dim: number
  max_attempts: number
  max_content_bytes: number
  updated_at: string
}

export interface CatalogStrategyDescriptor {
  id: string
  version: string
  dim: number
  max_input_bytes?: number
  description?: string
}

export interface CatalogBacklog {
  pending: number
  in_flight: number
  embedded: number
  failed: number
  dead_letter: number
  stream_len: number
}

export interface NamespaceCatalogResponse {
  catalog: NamespaceCatalogConfig
  available_strategies: CatalogStrategyDescriptor[]
  backlog: CatalogBacklog
}

export interface NamespaceCatalogUpdateRequest {
  enabled: boolean
  strategy_id?: string
  strategy_version?: string
  params?: Record<string, unknown>
  max_attempts?: number
  max_content_bytes?: number
}

// CatalogDimMismatchBody is the flat 400 body the admin API returns when the
// chosen strategy's natural dim does not equal the namespace's embedding_dim.
// Surfaced to the form via ApiError.body so the operator sees both numbers.
export interface CatalogDimMismatchBody {
  error: string
  strategy_dim: number
  namespace_embedding_dim: number
}


export type CatalogItemState = 'pending' | 'in_flight' | 'embedded' | 'failed' | 'dead_letter'

export interface CatalogItemSummary {
  id: number
  object_id: string
  state: CatalogItemState
  strategy_id?: string
  strategy_version?: string
  attempt_count: number
  last_error?: string
  embedded_at?: string
  updated_at: string
}

export interface CatalogItemDetail extends CatalogItemSummary {
  namespace: string
  content: string
  metadata?: Record<string, unknown>
  created_at: string
}

export interface CatalogItemsListResponse {
  items: CatalogItemSummary[]
  total: number
  limit: number
  offset: number
}

export interface CatalogReEmbedResponse {
  batch_run_id: number
  namespace: string
  strategy_id: string
  strategy_version: string
  stale_items: number
  started_at: string
}

export interface CatalogRedriveResponse {
  id: number
  object_id: string
  state: CatalogItemState
}

export interface CatalogBulkRedriveResponse {
  namespace: string
  redriven: number
}

export interface CatalogItemsListParams {
  namespace: string
  state?: CatalogItemState | 'all'
  limit?: number
  offset?: number
  objectID?: string
}
