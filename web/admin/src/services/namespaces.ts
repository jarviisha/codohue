import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { http } from './http'

// ────────────────────────────────────────────────────────────────────────────
// Wire types — mirror internal/admin/types.go
// ────────────────────────────────────────────────────────────────────────────

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
  updated_at: string // ISO timestamp
}

/**
 * NamespaceStatus is a free-form string from the backend; the well-known
 * values are enumerated below. Treat unknown values as "idle" to be safe.
 */
export type NamespaceStatus = 'active' | 'idle' | 'degraded' | 'cold' | (string & {})

/**
 * Slim subset of the Go `BatchRunLog` used by namespace pages (overview
 * panel + last-run summary). The full type lives in batchRuns.ts once Phase
 * 2.D lands; pages that need richer fields will switch to it then.
 */
export interface BatchRunSummary {
  id: number
  namespace: string
  started_at: string
  completed_at: string | null
  duration_ms: number | null
  subjects_processed: number
  success: boolean
  error_message: string | null
  trigger_source: string
  phase1_ok: boolean | null
  phase1_duration_ms: number | null
  phase2_ok: boolean | null
  phase2_duration_ms: number | null
  phase3_ok: boolean | null
  phase3_duration_ms: number | null
}

export interface NamespaceHealth {
  config: NamespaceConfig
  status: NamespaceStatus
  active_events_24h: number
  last_run: BatchRunSummary | null
}

export interface NamespacesListResponse {
  items: NamespaceConfig[]
  total: number
}

export interface NamespacesOverviewResponse {
  items: NamespaceHealth[]
  total: number
}

/**
 * Partial-update payload for `PUT /api/admin/v1/namespaces/{ns}`. All
 * numeric/string knobs are nullable so callers can submit a partial form;
 * `action_weights` is required because the Go handler does not accept null
 * there.
 */
export interface NamespaceUpsertRequest {
  action_weights: Record<string, number>
  lambda?: number | null
  gamma?: number | null
  alpha?: number | null
  max_results?: number | null
  seen_items_days?: number | null
  dense_strategy?: string | null
  embedding_dim?: number | null
  dense_distance?: string | null
  trending_window?: number | null
  trending_ttl?: number | null
  lambda_trending?: number | null
}

export interface NamespaceUpsertResponse {
  namespace: string
  updated_at: string
  /** Returned by the backend only on first create; never on update. */
  api_key?: string
}

// ────────────────────────────────────────────────────────────────────────────
// Query keys
// ────────────────────────────────────────────────────────────────────────────

export const namespaceKeys = {
  all: ['ns'] as const,
  list: () => ['ns', 'list'] as const,
  overview: () => ['ns', 'overview'] as const,
  byName: (name: string) => ['ns', 'detail', name] as const,
}

// ────────────────────────────────────────────────────────────────────────────
// Request functions
// ────────────────────────────────────────────────────────────────────────────

export function fetchNamespaces(signal?: AbortSignal) {
  return http.get<NamespacesListResponse>('/api/admin/v1/namespaces', { signal })
}

export function fetchNamespacesOverview(signal?: AbortSignal) {
  return http.get<NamespacesOverviewResponse>(
    '/api/admin/v1/namespaces?include=overview',
    { signal },
  )
}

export function fetchNamespace(name: string, signal?: AbortSignal) {
  return http.get<NamespaceConfig>(
    `/api/admin/v1/namespaces/${encodeURIComponent(name)}`,
    { signal },
  )
}

export function upsertNamespace(
  name: string,
  payload: NamespaceUpsertRequest,
) {
  return http.put<NamespaceUpsertResponse>(
    `/api/admin/v1/namespaces/${encodeURIComponent(name)}`,
    payload,
  )
}

// ────────────────────────────────────────────────────────────────────────────
// Hooks
// ────────────────────────────────────────────────────────────────────────────

export function useNamespaces() {
  return useQuery({
    queryKey: namespaceKeys.list(),
    queryFn: ({ signal }) => fetchNamespaces(signal),
    staleTime: 30_000,
  })
}

export function useNamespacesOverview() {
  return useQuery({
    queryKey: namespaceKeys.overview(),
    queryFn: ({ signal }) => fetchNamespacesOverview(signal),
    staleTime: 15_000,
    refetchInterval: 60_000,
  })
}

export function useNamespace(name: string | undefined) {
  return useQuery({
    queryKey: namespaceKeys.byName(name ?? ''),
    queryFn: ({ signal }) => fetchNamespace(name as string, signal),
    enabled: Boolean(name),
    staleTime: 30_000,
  })
}

export interface UseUpsertNamespaceVariables {
  name: string
  payload: NamespaceUpsertRequest
}

export function useUpsertNamespace() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ name, payload }: UseUpsertNamespaceVariables) =>
      upsertNamespace(name, payload),
    onSuccess: (_data, { name }) => {
      qc.invalidateQueries({ queryKey: namespaceKeys.list() })
      qc.invalidateQueries({ queryKey: namespaceKeys.overview() })
      qc.invalidateQueries({ queryKey: namespaceKeys.byName(name) })
    },
  })
}

// ────────────────────────────────────────────────────────────────────────────
// Helpers used by namespace pages
// ────────────────────────────────────────────────────────────────────────────

import type { StatusState } from '../components/ui/StatusToken'

/**
 * Map a backend NamespaceStatus to the shared 6-state StatusToken vocabulary.
 * Unknown values fall through to "idle" so the operator still sees something
 * meaningful instead of a render error.
 */
export function namespaceStatusToken(status: NamespaceStatus): StatusState {
  switch (status) {
    case 'active':
      return 'ok'
    case 'degraded':
      return 'warn'
    case 'cold':
      return 'pend'
    case 'idle':
    default:
      return 'idle'
  }
}

/**
 * Map a `last_run` summary to the token shown next to a namespace row. When
 * there is no run yet we surface IDLE; a failed run beats success
 * (operators want failures to stand out).
 */
export function lastRunToken(run: BatchRunSummary | null): StatusState {
  if (!run) return 'idle'
  if (run.success) return 'ok'
  return 'fail'
}
