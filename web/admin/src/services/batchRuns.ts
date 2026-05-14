import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { http } from './http'
import { namespaceKeys } from './namespaces'

// ────────────────────────────────────────────────────────────────────────────
// Wire types — mirror internal/admin/types.go
// ────────────────────────────────────────────────────────────────────────────

export type BatchRunKind = 'cf' | 'reembed'
export type BatchRunStatusFilter = '' | 'running' | 'ok' | 'failed'

export interface BatchRunCreateResponse {
  id: number
  namespace: string
}

export interface BatchRunLogEntry {
  ts: string
  level: string
  msg: string
}

export interface BatchRunLog {
  id: number
  namespace: string
  started_at: string
  completed_at?: string | null
  duration_ms?: number | null
  subjects_processed: number
  success: boolean
  error_message?: string | null
  trigger_source: string
  log_lines?: BatchRunLogEntry[] | null

  phase1_ok?: boolean | null
  phase1_duration_ms?: number | null
  phase1_subjects?: number | null
  phase1_objects?: number | null
  phase1_error?: string | null

  phase2_ok?: boolean | null
  phase2_duration_ms?: number | null
  phase2_items?: number | null
  phase2_subjects?: number | null
  phase2_error?: string | null

  phase3_ok?: boolean | null
  phase3_duration_ms?: number | null
  phase3_items?: number | null
  phase3_error?: string | null
}

export interface BatchRunStats {
  total: number
  running: number
  ok: number
  failed: number
}

export interface BatchRunsListResponse {
  items: BatchRunLog[]
  total: number
  offset: number
  stats: BatchRunStats
}

export interface BatchRunsListParams {
  namespace: string
  kind?: BatchRunKind
  status?: BatchRunStatusFilter
  limit?: number
  offset?: number
}

// ────────────────────────────────────────────────────────────────────────────
// Query keys
// ────────────────────────────────────────────────────────────────────────────

export const batchRunKeys = {
  all: ['batchRuns'] as const,
  list: (namespace: string, params?: Omit<BatchRunsListParams, 'namespace'>) =>
    [
      'batchRuns',
      'list',
      namespace,
      {
        kind: params?.kind ?? '',
        status: params?.status ?? '',
        limit: params?.limit ?? 20,
        offset: params?.offset ?? 0,
      },
    ] as const,
}

// ────────────────────────────────────────────────────────────────────────────
// Request functions
// ────────────────────────────────────────────────────────────────────────────

function queryString(params: Record<string, string | number | undefined>) {
  const q = new URLSearchParams()
  for (const [key, value] of Object.entries(params)) {
    if (value === undefined || value === '') continue
    q.set(key, String(value))
  }
  const encoded = q.toString()
  return encoded ? `?${encoded}` : ''
}

export function triggerBatchRun(namespace: string) {
  return http.post<BatchRunCreateResponse>(
    `/api/admin/v1/namespaces/${encodeURIComponent(namespace)}/batch-runs`,
    {},
  )
}

export function listBatchRuns(params: BatchRunsListParams, signal?: AbortSignal) {
  const qs = queryString({
    kind: params.kind,
    status: params.status,
    limit: params.limit,
    offset: params.offset,
  })
  return http.get<BatchRunsListResponse>(
    `/api/admin/v1/namespaces/${encodeURIComponent(params.namespace)}/batch-runs${qs}`,
    { signal },
  )
}

// ────────────────────────────────────────────────────────────────────────────
// Hooks
// ────────────────────────────────────────────────────────────────────────────

export function useTriggerBatchRun() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: triggerBatchRun,
    onSuccess: (_data, namespace) => {
      qc.invalidateQueries({ queryKey: namespaceKeys.overview() })
      qc.invalidateQueries({ queryKey: namespaceKeys.byName(namespace) })
      qc.invalidateQueries({ queryKey: ['batchRuns', 'list', namespace] })
    },
  })
}

export function useBatchRunsList(params: BatchRunsListParams) {
  return useQuery({
    queryKey: batchRunKeys.list(params.namespace, params),
    queryFn: ({ signal }) => listBatchRuns(params, signal),
    enabled: Boolean(params.namespace),
    staleTime: 10_000,
  })
}
