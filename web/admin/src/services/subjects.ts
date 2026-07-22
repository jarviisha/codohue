import { useQuery } from '@tanstack/react-query'
import { apiFetch } from './http'

// ---------------------------------------------------------------------------
// Wire types — mirror Go shapes from internal/admin/types.go. Hand-maintained
// for now; TypeScript codegen lands when shape churn slows.
// ---------------------------------------------------------------------------

export type SubjectProfileResponse = {
  subject_id: string
  namespace: string
  interaction_count: number
  seen_items: string[]
  seen_items_days: number
  sparse_vector_nnz: number // -1 means not yet indexed in Qdrant
}

export type SubjectSort = 'last_seen' | 'interactions' | 'subject_id'

export type SubjectListItem = {
  subject_id: string
  interaction_count: number
  last_seen: string
}

export type SubjectsListResponse = {
  items: SubjectListItem[]
  total: number
  limit: number
  offset: number
  sort: SubjectSort
}

export type RecommendDebugItem = {
  object_id: string
  score: number
  rank: number
}

export type RecommendDebug = {
  sparse_nnz: number
  dense_score: number
  alpha: number
  seen_items_count: number
  interaction_count: number
}

export type RecommendResponse = {
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

// ---------------------------------------------------------------------------
// Query keys
// ---------------------------------------------------------------------------

const subjectKeys = {
  list: (ns: string, opts: Record<string, unknown>) => ['ns', ns, 'subjects', opts] as const,
  profile: (ns: string, id: string) => ['ns', ns, 'subject', id, 'profile'] as const,
  recommendations: (ns: string, id: string, opts: Record<string, unknown>) =>
    ['ns', ns, 'subject', id, 'recommendations', opts] as const,
}

// ---------------------------------------------------------------------------
// Hooks
// ---------------------------------------------------------------------------

export type SubjectsFilter = {
  q?: string
  sort?: SubjectSort
  limit?: number
  offset?: number
}

/**
 * useSubjectsList browses the subjects that have events in a namespace.
 * Subjects are an aggregate over the events table, not a stored resource, so
 * the list is derived server-side and `q` is a subject_id *prefix* match
 * (that's what rides the index) rather than a substring search.
 */
export function useSubjectsList(ns: string | null, filter: SubjectsFilter = {}) {
  return useQuery({
    queryKey: ns
      ? subjectKeys.list(ns, filter as Record<string, unknown>)
      : ['ns', 'unknown', 'subjects'],
    queryFn: () => {
      const p = new URLSearchParams()
      if (filter.q) p.set('q', filter.q)
      if (filter.sort) p.set('sort', filter.sort)
      if (filter.limit != null) p.set('limit', String(filter.limit))
      if (filter.offset != null) p.set('offset', String(filter.offset))
      const q = p.toString()
      return apiFetch<SubjectsListResponse>(
        `/api/admin/v1/namespaces/${ns}/subjects${q ? `?${q}` : ''}`,
      )
    },
    enabled: !!ns,
  })
}

export function useSubjectProfile(ns: string | null, id: string | null) {
  return useQuery({
    queryKey: ns && id ? subjectKeys.profile(ns, id) : ['ns', 'unknown', 'subject', 'profile'],
    queryFn: () =>
      apiFetch<SubjectProfileResponse>(
        `/api/admin/v1/namespaces/${ns}/subjects/${encodeURIComponent(id ?? '')}/profile`,
      ),
    enabled: !!ns && !!id,
  })
}

export type RecommendationsFilter = {
  limit?: number
  offset?: number
  debug?: boolean
}

function recsQueryString(f: RecommendationsFilter): string {
  const p = new URLSearchParams()
  if (f.limit != null) p.set('limit', String(f.limit))
  if (f.offset != null) p.set('offset', String(f.offset))
  if (f.debug) p.set('debug', 'true')
  const q = p.toString()
  return q ? `?${q}` : ''
}

export function useSubjectRecommendations(
  ns: string | null,
  id: string | null,
  filter: RecommendationsFilter = {},
) {
  return useQuery({
    queryKey:
      ns && id
        ? subjectKeys.recommendations(ns, id, filter as Record<string, unknown>)
        : ['ns', 'unknown', 'subject', 'recommendations'],
    queryFn: () =>
      apiFetch<RecommendResponse>(
        `/api/admin/v1/namespaces/${ns}/subjects/${encodeURIComponent(id ?? '')}/recommendations${recsQueryString(filter)}`,
      ),
    enabled: !!ns && !!id,
  })
}
