import { useQuery } from '@tanstack/react-query'
import { http } from './http'

// ────────────────────────────────────────────────────────────────────────────
// Wire types — mirror internal/admin/types.go
// ────────────────────────────────────────────────────────────────────────────

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
  // One of: collaborative_filtering | hybrid | hybrid_rank | hybrid_cold | fallback_popular.
  source: string
  limit: number
  offset: number
  total: number
  generated_at: string
  debug?: RecommendDebug
}

export interface SubjectProfileResponse {
  subject_id: string
  namespace: string
  interaction_count: number
  // Items the subject has interacted with inside seen_items_days; used by the
  // recommender to filter already-seen objects from results.
  seen_items: string[]
  seen_items_days: number
  // -1 = subject has no Qdrant point yet (cold).
  sparse_vector_nnz: number
}

export interface RecommendDebugParams {
  namespace: string
  subject_id: string
  limit?: number
  offset?: number
  debug?: boolean
}

export interface SubjectProfileParams {
  namespace: string
  subject_id: string
}

// ────────────────────────────────────────────────────────────────────────────
// Query keys
// ────────────────────────────────────────────────────────────────────────────

export const recommendKeys = {
  all: ['recommend'] as const,
  recommendations: (namespace: string, subjectID: string, params?: Omit<RecommendDebugParams, 'namespace' | 'subject_id'>) =>
    [
      'recommend',
      'recommendations',
      namespace,
      subjectID,
      {
        limit: params?.limit ?? 10,
        offset: params?.offset ?? 0,
        debug: params?.debug ?? false,
      },
    ] as const,
  profile: (namespace: string, subjectID: string) =>
    ['recommend', 'profile', namespace, subjectID] as const,
}

// ────────────────────────────────────────────────────────────────────────────
// Request functions
// ────────────────────────────────────────────────────────────────────────────

function queryString(params: Record<string, string | number | boolean | undefined>) {
  const q = new URLSearchParams()
  for (const [key, value] of Object.entries(params)) {
    if (value === undefined || value === '' || value === false) continue
    q.set(key, String(value))
  }
  const encoded = q.toString()
  return encoded ? `?${encoded}` : ''
}

export function recommendDebug(params: RecommendDebugParams, signal?: AbortSignal) {
  const qs = queryString({
    limit: params.limit,
    offset: params.offset,
    debug: params.debug,
  })
  return http.get<RecommendResponse>(
    `/api/admin/v1/namespaces/${encodeURIComponent(params.namespace)}/subjects/${encodeURIComponent(params.subject_id)}/recommendations${qs}`,
    { signal },
  )
}

export function subjectProfile(params: SubjectProfileParams, signal?: AbortSignal) {
  return http.get<SubjectProfileResponse>(
    `/api/admin/v1/namespaces/${encodeURIComponent(params.namespace)}/subjects/${encodeURIComponent(params.subject_id)}/profile`,
    { signal },
  )
}

// ────────────────────────────────────────────────────────────────────────────
// Hooks
// ────────────────────────────────────────────────────────────────────────────

// Both hooks are gated on subject_id so the debug page can render an empty
// form without firing requests for the placeholder value.
export function useRecommendDebug(params: RecommendDebugParams, enabled = true) {
  return useQuery({
    queryKey: recommendKeys.recommendations(params.namespace, params.subject_id, params),
    queryFn: ({ signal }) => recommendDebug(params, signal),
    enabled: enabled && Boolean(params.namespace) && Boolean(params.subject_id),
    staleTime: 10_000,
  })
}

export function useSubjectProfile(params: SubjectProfileParams, enabled = true) {
  return useQuery({
    queryKey: recommendKeys.profile(params.namespace, params.subject_id),
    queryFn: ({ signal }) => subjectProfile(params, signal),
    enabled: enabled && Boolean(params.namespace) && Boolean(params.subject_id),
    staleTime: 10_000,
  })
}
