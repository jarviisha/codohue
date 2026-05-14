import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { http } from './http'

// ────────────────────────────────────────────────────────────────────────────
// Wire types — mirror internal/admin/types.go catalog contracts
// ────────────────────────────────────────────────────────────────────────────

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
  strategy_id?: string | null
  strategy_version?: string | null
  params?: Record<string, unknown>
  max_attempts?: number | null
  max_content_bytes?: number | null
}

export interface CatalogReEmbedResponse {
  batch_run_id: number
  namespace: string
  strategy_id: string
  strategy_version: string
  stale_items: number
  started_at: string
}

export type CatalogItemState =
  | 'pending'
  | 'in_flight'
  | 'embedded'
  | 'failed'
  | 'dead_letter'
  | (string & {})

export type CatalogItemsStateFilter = CatalogItemState | 'all'

export interface CatalogItemSummary {
  id: number
  object_id: string
  state: CatalogItemState
  strategy_id?: string
  strategy_version?: string
  attempt_count: number
  last_error?: string
  embedded_at?: string | null
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

export interface CatalogRedriveResponse {
  id: number
  object_id: string
  state: CatalogItemState
}

export interface CatalogBulkRedriveResponse {
  namespace: string
  redriven: number
}

// ────────────────────────────────────────────────────────────────────────────
// Query keys
// ────────────────────────────────────────────────────────────────────────────

export interface CatalogItemsListParams {
  namespace: string
  state?: CatalogItemsStateFilter
  limit?: number
  offset?: number
  object_id?: string
}

export const catalogKeys = {
  all: ['catalog'] as const,
  namespace: (namespace: string) => ['catalog', namespace] as const,
  config: (namespace: string) => ['catalog', namespace, 'config'] as const,
  items: (namespace: string) => ['catalog', namespace, 'items'] as const,
  itemList: (params: CatalogItemsListParams) =>
    [
      'catalog',
      params.namespace,
      'items',
      {
        state: params.state ?? 'all',
        limit: params.limit ?? 50,
        offset: params.offset ?? 0,
        object_id: params.object_id ?? '',
      },
    ] as const,
  itemDetail: (namespace: string, id: number | string) =>
    ['catalog', namespace, 'items', String(id)] as const,
}

// ────────────────────────────────────────────────────────────────────────────
// Request functions
// ────────────────────────────────────────────────────────────────────────────

function nsPath(namespace: string) {
  return `/api/admin/v1/namespaces/${encodeURIComponent(namespace)}/catalog`
}

function itemPath(namespace: string, id: number | string) {
  return `${nsPath(namespace)}/items/${encodeURIComponent(String(id))}`
}

function queryString(params: Record<string, string | number | undefined>) {
  const q = new URLSearchParams()
  for (const [key, value] of Object.entries(params)) {
    if (value === undefined || value === '') continue
    q.set(key, String(value))
  }
  const encoded = q.toString()
  return encoded ? `?${encoded}` : ''
}

export function getCatalogConfig(namespace: string, signal?: AbortSignal) {
  return http.get<NamespaceCatalogResponse>(nsPath(namespace), { signal })
}

export function updateCatalogConfig(
  namespace: string,
  payload: NamespaceCatalogUpdateRequest,
) {
  return http.put<NamespaceCatalogConfig>(nsPath(namespace), payload)
}

export function triggerCatalogReEmbed(namespace: string) {
  return http.post<CatalogReEmbedResponse>(`${nsPath(namespace)}/re-embed`)
}

export function listCatalogItems(
  params: CatalogItemsListParams,
  signal?: AbortSignal,
) {
  const qs = queryString({
    state: params.state,
    limit: params.limit,
    offset: params.offset,
    object_id: params.object_id,
  })
  return http.get<CatalogItemsListResponse>(
    `${nsPath(params.namespace)}/items${qs}`,
    { signal },
  )
}

export function getCatalogItem(
  namespace: string,
  id: number | string,
  signal?: AbortSignal,
) {
  return http.get<CatalogItemDetail>(itemPath(namespace, id), { signal })
}

export function redriveCatalogItem(namespace: string, id: number | string) {
  return http.post<CatalogRedriveResponse>(
    `${itemPath(namespace, id)}/redrive`,
  )
}

export function bulkRedriveDeadletter(namespace: string) {
  return http.post<CatalogBulkRedriveResponse>(
    `${nsPath(namespace)}/items/redrive-deadletter`,
  )
}

export function deleteCatalogItem(namespace: string, id: number | string) {
  return http.del<void>(itemPath(namespace, id))
}

// ────────────────────────────────────────────────────────────────────────────
// Hooks
// ────────────────────────────────────────────────────────────────────────────

export function useCatalogConfig(namespace: string | undefined) {
  return useQuery({
    queryKey: catalogKeys.config(namespace ?? ''),
    queryFn: ({ signal }) => getCatalogConfig(namespace as string, signal),
    enabled: Boolean(namespace),
    staleTime: 15_000,
    refetchInterval: 60_000,
  })
}

export interface UseUpdateCatalogConfigVariables {
  namespace: string
  payload: NamespaceCatalogUpdateRequest
}

export function useUpdateCatalogConfig() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ namespace, payload }: UseUpdateCatalogConfigVariables) =>
      updateCatalogConfig(namespace, payload),
    onSuccess: (_data, { namespace }) => {
      qc.invalidateQueries({ queryKey: catalogKeys.namespace(namespace) })
    },
  })
}

export function useTriggerCatalogReEmbed() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: triggerCatalogReEmbed,
    onSuccess: (_data, namespace) => {
      qc.invalidateQueries({ queryKey: catalogKeys.namespace(namespace) })
    },
  })
}

export function useCatalogItems(params: CatalogItemsListParams | undefined) {
  return useQuery({
    queryKey: catalogKeys.itemList(
      params ?? { namespace: '', state: 'all', limit: 50, offset: 0 },
    ),
    queryFn: ({ signal }) =>
      listCatalogItems(params as CatalogItemsListParams, signal),
    enabled: Boolean(params?.namespace),
    staleTime: 10_000,
  })
}

export function useCatalogItem(
  namespace: string | undefined,
  id: number | string | undefined,
) {
  return useQuery({
    queryKey: catalogKeys.itemDetail(namespace ?? '', id ?? ''),
    queryFn: ({ signal }) =>
      getCatalogItem(namespace as string, id as number | string, signal),
    enabled: Boolean(namespace && id),
    staleTime: 10_000,
  })
}

export interface UseCatalogItemMutationVariables {
  namespace: string
  id: number | string
}

export function useRedriveCatalogItem() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ namespace, id }: UseCatalogItemMutationVariables) =>
      redriveCatalogItem(namespace, id),
    onSuccess: (_data, { namespace, id }) => {
      qc.invalidateQueries({ queryKey: catalogKeys.items(namespace) })
      qc.invalidateQueries({ queryKey: catalogKeys.itemDetail(namespace, id) })
      qc.invalidateQueries({ queryKey: catalogKeys.config(namespace) })
    },
  })
}

export function useBulkRedriveDeadletter() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: bulkRedriveDeadletter,
    onSuccess: (_data, namespace) => {
      qc.invalidateQueries({ queryKey: catalogKeys.namespace(namespace) })
    },
  })
}

export function useDeleteCatalogItem() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ namespace, id }: UseCatalogItemMutationVariables) =>
      deleteCatalogItem(namespace, id),
    onSuccess: (_data, { namespace, id }) => {
      qc.invalidateQueries({ queryKey: catalogKeys.items(namespace) })
      qc.removeQueries({ queryKey: catalogKeys.itemDetail(namespace, id) })
      qc.invalidateQueries({ queryKey: catalogKeys.config(namespace) })
    },
  })
}
