import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ApiError, apiFetch } from './http'
import { queryKeys } from './queryKeys'
import type { BatchRunSummary } from './batchRuns'

export type NamespaceConfig = {
  namespace: string
  action_weights: Record<string, number>
  lambda: number
  gamma: number
  alpha: number
  max_results: number
  seen_items_days: number
  exclude_authored: boolean
  dense_source: string
  embedding_dim: number
  dense_distance: string
  trending_window: number
  trending_ttl: number
  lambda_trending: number
  has_api_key: boolean
  catalog_strategy_id?: string
  catalog_strategy_version?: string
  updated_at: string
}

export type NamespacesListResponse = {
  items: NamespaceConfig[]
  total: number
}

export type NamespaceUpsertRequest = {
  action_weights?: Record<string, number>
  lambda?: number
  gamma?: number
  alpha?: number
  max_results?: number
  seen_items_days?: number
  exclude_authored?: boolean
  dense_source?: string
  embedding_dim?: number
  dense_distance?: string
  trending_window?: number
  trending_ttl?: number
  lambda_trending?: number
  /**
   * Only honoured with dense_source="catalog", where the backend requires
   * both fields in the same request (422 otherwise) so it can run the
   * registry + dim validation before flipping the namespace into catalog mode.
   */
  catalog_strategy_id?: string
  catalog_strategy_version?: string
}

export type NamespaceUpsertResponse = {
  namespace: string
  updated_at: string
  api_key?: string
}

type QdrantCollection = {
  exists: boolean
  points_count: number
}

type QdrantInspectResponse = {
  subjects: QdrantCollection
  objects: QdrantCollection
  subjects_dense: QdrantCollection
  objects_dense: QdrantCollection
}

type CatalogBacklog = {
  pending: number
  in_flight: number
  embedded: number
  failed: number
  dead_letter: number
  stream_len: number
}

export type NamespaceDashboardResponse = {
  namespace: string
  generated_at: string
  config: NamespaceConfig
  last_runs: BatchRunSummary[]
  catalog: CatalogBacklog
  events_24h: number
  events_per_min_now: number
  qdrant: QdrantInspectResponse
  trending_ttl_sec: number
  /** How many catalog items carry an author — drives the exclude_authored hint. */
  author_coverage: { attributed: number; total: number }
}

export function useNamespaces() {
  return useQuery({
    queryKey: queryKeys.namespaces,
    queryFn: () => apiFetch<NamespacesListResponse>('/api/admin/v1/namespaces'),
    staleTime: 30_000,
  })
}

/**
 * lookupNamespace resolves a namespace config, or null when it does not exist.
 *
 * The create-namespace form needs this because creation rides the same PUT
 * upsert as an edit: without a pre-flight existence check, typing a name that
 * is already taken silently overwrites that namespace's config and reports
 * success. Any other failure re-throws — a broken lookup must not read as
 * "name is free".
 */
export async function lookupNamespace(namespace: string): Promise<NamespaceConfig | null> {
  try {
    return await apiFetch<NamespaceConfig>(
      `/api/admin/v1/namespaces/${encodeURIComponent(namespace)}`,
    )
  } catch (error) {
    if (error instanceof ApiError && error.status === 404) return null
    throw error
  }
}

export function useNamespaceDashboard(ns: string | null) {
  return useQuery({
    queryKey: ns ? queryKeys.namespaceDashboard(ns) : ['namespaces', 'unknown', 'dashboard'],
    queryFn: () =>
      apiFetch<NamespaceDashboardResponse>(`/api/admin/v1/namespaces/${ns}/dashboard`),
    enabled: ns != null && ns !== '',
    refetchInterval: 30_000,
  })
}

export function useUpsertNamespace() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ namespace, body }: { namespace: string; body: NamespaceUpsertRequest }) =>
      apiFetch<NamespaceUpsertResponse>(`/api/admin/v1/namespaces/${namespace}`, {
        method: 'PUT',
        body: JSON.stringify(body),
      }),
    onSuccess: (_data, { namespace }) => {
      qc.invalidateQueries({ queryKey: queryKeys.namespaces })
      qc.invalidateQueries({ queryKey: queryKeys.namespaceDashboard(namespace) })
      qc.invalidateQueries({ queryKey: queryKeys.overview })
    },
  })
}
