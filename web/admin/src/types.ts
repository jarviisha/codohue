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
  trigger_source: 'cron' | 'manual'
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
