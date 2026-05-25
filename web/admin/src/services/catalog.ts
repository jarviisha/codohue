import {
  useMutation,
  useQuery,
  useQueryClient,
  type UseQueryOptions,
} from '@tanstack/react-query'
import { apiFetch } from './http'

// ---------------------------------------------------------------------------
// Wire types — mirror Go shapes from internal/admin/types.go. Hand-maintained
// for now; TypeScript codegen lands when shape churn slows.
// ---------------------------------------------------------------------------

export type CatalogItemState =
  | 'pending'
  | 'in_flight'
  | 'embedded'
  | 'failed'
  | 'dead_letter'
  | string

export type CatalogBacklog = {
  pending: number
  in_flight: number
  embedded: number
  failed: number
  dead_letter: number
  stream_len: number
}

export type CatalogStrategyDescriptor = {
  id: string
  version: string
  dim: number
  description?: string
  default?: boolean
}

export type NamespaceCatalogConfig = {
  namespace: string
  enabled: boolean
  strategy_id: string
  strategy_version: string
  strategy_params?: Record<string, unknown>
  max_content_bytes?: number
  max_attempts?: number
}

export type CatalogReEmbedSummary = {
  batch_run_id: number
  strategy_id: string
  strategy_version: string
  status: 'running' | 'success' | 'failed' | string
  started_at: string
  completed_at?: string
  duration_ms?: number
  processed?: number
  error_message?: string
}

export type NamespaceCatalogResponse = {
  catalog: NamespaceCatalogConfig
  available_strategies: CatalogStrategyDescriptor[]
  backlog: CatalogBacklog
  last_embedded_at?: string
  last_re_embed?: CatalogReEmbedSummary
}

export type CatalogBacklogSample = {
  sampled_at: string
  pending: number
  in_flight: number
  failed: number
  dead_letter: number
  stream_len: number
}

export type CatalogBacklogHistoryResponse = {
  namespace: string
  window_seconds: number
  samples: CatalogBacklogSample[]
}

export type CatalogFailureReason = {
  reason: string
  count: number
  sample_object_id?: string
}

export type CatalogFailuresSummaryResponse = {
  namespace: string
  window_seconds: number
  reasons: CatalogFailureReason[]
}

export type CatalogItemSummary = {
  id: number
  namespace: string
  object_id: string
  state: CatalogItemState
  strategy_id?: string
  strategy_version?: string
  attempt_count: number
  last_error?: string
  embedded_at?: string
  created_at: string
  updated_at: string
}

export type CatalogItemsListResponse = {
  items: CatalogItemSummary[]
  total: number
  limit: number
  offset: number
}

export type CatalogItemDetail = CatalogItemSummary & {
  content: string
  metadata?: Record<string, unknown>
  vector?: {
    collection: string
    numeric_id: number
    dim: number
    preview: number[]
  }
}

export type CatalogRedriveResponse = {
  item: CatalogItemDetail
}

export type CatalogBulkRedriveResponse = {
  redriven: number
}

export type CatalogReEmbedResponse = {
  batch_run_id: number
  namespace: string
  status: string
  started_at: string
}

// ---------------------------------------------------------------------------
// Query keys
// ---------------------------------------------------------------------------

export const catalogKeys = {
  config: (ns: string) => ['catalog', ns, 'config'] as const,
  backlogHistory: (ns: string, window: string) =>
    ['catalog', ns, 'backlog-history', window] as const,
  failures: (ns: string, window: string) => ['catalog', ns, 'failures', window] as const,
  items: (ns: string, filter: Record<string, unknown>) =>
    ['catalog', ns, 'items', filter] as const,
  item: (ns: string, id: number | string) => ['catalog', ns, 'item', id] as const,
}

// ---------------------------------------------------------------------------
// Catalog status / config
// ---------------------------------------------------------------------------

export function useCatalogConfig(ns: string | null) {
  return useQuery({
    queryKey: ns ? catalogKeys.config(ns) : ['catalog', 'unknown', 'config'],
    queryFn: () => apiFetch<NamespaceCatalogResponse>(`/api/admin/v1/namespaces/${ns}/catalog`),
    enabled: ns != null && ns !== '',
    refetchInterval: 15_000,
  })
}

export function useCatalogBacklogHistory(ns: string | null, window: string = '1h') {
  return useQuery({
    queryKey: ns ? catalogKeys.backlogHistory(ns, window) : ['catalog', 'unknown', 'history'],
    queryFn: () =>
      apiFetch<CatalogBacklogHistoryResponse>(
        `/api/admin/v1/namespaces/${ns}/catalog/backlog-history?window=${encodeURIComponent(window)}`,
      ),
    enabled: ns != null && ns !== '',
    refetchInterval: 60_000,
  })
}

export function useCatalogFailuresSummary(ns: string | null, window: string = '24h', limit = 10) {
  return useQuery({
    queryKey: ns ? catalogKeys.failures(ns, window) : ['catalog', 'unknown', 'failures'],
    queryFn: () =>
      apiFetch<CatalogFailuresSummaryResponse>(
        `/api/admin/v1/namespaces/${ns}/catalog/failures-summary?window=${encodeURIComponent(window)}&limit=${limit}`,
      ),
    enabled: ns != null && ns !== '',
    refetchInterval: 60_000,
  })
}

// ---------------------------------------------------------------------------
// Catalog items list / detail
// ---------------------------------------------------------------------------

export type CatalogItemsFilter = {
  state?: CatalogItemState | ''
  objectId?: string
  limit?: number
  offset?: number
}

function itemsQueryString(f: CatalogItemsFilter): string {
  const params = new URLSearchParams()
  if (f.state) params.set('state', f.state)
  if (f.objectId) params.set('object_id', f.objectId)
  if (f.limit != null) params.set('limit', String(f.limit))
  if (f.offset != null) params.set('offset', String(f.offset))
  const q = params.toString()
  return q ? `?${q}` : ''
}

export function useCatalogItems(ns: string | null, filter: CatalogItemsFilter = {}) {
  return useQuery({
    queryKey: ns ? catalogKeys.items(ns, filter as Record<string, unknown>) : ['catalog', 'unknown', 'items'],
    queryFn: () =>
      apiFetch<CatalogItemsListResponse>(
        `/api/admin/v1/namespaces/${ns}/catalog/items${itemsQueryString(filter)}`,
      ),
    enabled: ns != null && ns !== '',
    refetchInterval: 20_000,
  })
}

export function useCatalogItem(
  ns: string | null,
  id: number | string | null,
  options?: Omit<UseQueryOptions<CatalogItemDetail>, 'queryKey' | 'queryFn'>,
) {
  return useQuery({
    queryKey: ns && id ? catalogKeys.item(ns, id) : ['catalog', 'unknown', 'item'],
    queryFn: () => apiFetch<CatalogItemDetail>(`/api/admin/v1/namespaces/${ns}/catalog/items/${id}`),
    enabled: ns != null && id != null && ns !== '',
    ...options,
  })
}

// ---------------------------------------------------------------------------
// Mutations
// ---------------------------------------------------------------------------

export function useRedriveCatalogItem(ns: string | null) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: number) =>
      apiFetch<CatalogRedriveResponse>(
        `/api/admin/v1/namespaces/${ns}/catalog/items/${id}/redrive`,
        { method: 'POST' },
      ),
    onSuccess: () => {
      if (ns) {
        qc.invalidateQueries({ queryKey: ['catalog', ns] })
      }
    },
  })
}

export function useBulkRedriveDeadletter(ns: string | null) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: () =>
      apiFetch<CatalogBulkRedriveResponse>(
        `/api/admin/v1/namespaces/${ns}/catalog/items/redrive-deadletter`,
        { method: 'POST' },
      ),
    onSuccess: () => {
      if (ns) {
        qc.invalidateQueries({ queryKey: ['catalog', ns] })
      }
    },
  })
}

export function useDeleteCatalogItem(ns: string | null) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: number) =>
      apiFetch<void>(`/api/admin/v1/namespaces/${ns}/catalog/items/${id}`, { method: 'DELETE' }),
    onSuccess: () => {
      if (ns) {
        qc.invalidateQueries({ queryKey: ['catalog', ns] })
      }
    },
  })
}

export function useTriggerReEmbed(ns: string | null) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: () =>
      apiFetch<CatalogReEmbedResponse>(
        `/api/admin/v1/namespaces/${ns}/catalog/re-embed`,
        { method: 'POST' },
      ),
    onSuccess: () => {
      if (ns) {
        qc.invalidateQueries({ queryKey: catalogKeys.config(ns) })
      }
    },
  })
}
