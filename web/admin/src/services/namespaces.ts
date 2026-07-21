import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { apiFetch } from './http'
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
  dense_source?: string
  embedding_dim?: number
  dense_distance?: string
  trending_window?: number
  trending_ttl?: number
  lambda_trending?: number
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
}

export function useNamespaces() {
  return useQuery({
    queryKey: queryKeys.namespaces,
    queryFn: () => apiFetch<NamespacesListResponse>('/api/admin/v1/namespaces'),
    staleTime: 30_000,
  })
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
