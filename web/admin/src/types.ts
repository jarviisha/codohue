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
  namespaces: NamespaceConfig[]
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
  namespaces: NamespaceHealth[]
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

export interface BatchRunsResponse {
  runs: BatchRunLog[]
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
  events: EventSummary[]
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

export interface HealthData {
  postgres: string
  redis: string
  qdrant: string
  status: string
}

export interface QdrantCollectionStat {
  exists: boolean
  points_count: number
  indexed_vectors_count: number
}

export interface QdrantStatsResponse {
  namespace: string
  collections: Record<string, QdrantCollectionStat>
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

export interface RecommendDebugResponse {
  subject_id: string
  namespace: string
  items: RecommendDebugItem[]
  source: string
  limit: number
  offset: number
  total: number
  generated_at: string
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

export interface TriggerBatchResponse {
  batch_run_id: number
  namespace: string
  started_at: string
  duration_ms: number
  success: boolean
}
