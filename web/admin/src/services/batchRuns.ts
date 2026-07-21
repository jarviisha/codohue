import {
  useMutation,
  useQuery,
  useQueryClient,
  type UseQueryOptions,
} from '@tanstack/react-query'
import { apiFetch } from './http'
import { queryKeys } from './queryKeys'

// ---------------------------------------------------------------------------
// Wire types — mirror Go shapes from internal/admin/types.go. Hand-maintained
// for now; TypeScript codegen (BUILD_PLAN D7) lands when shape churn slows.
// ---------------------------------------------------------------------------

export type PhaseStatus = 'ok' | 'fail' | 'skipped' | null

export type PhaseEntry = {
  n: number
  name: 'sparse' | 'dense' | 'trending' | string
  ok: boolean | null
  skipped: string | null
  duration_ms: number
  subjects?: number
  objects?: number
  items?: number
  error: string | null
}

type TargetStrategy = {
  id: string
  version: string
}

export type BatchRunSummary = {
  id: number
  namespace: string
  kind: 'cf' | 'reembed' | string
  trigger_source: 'cron' | 'manual' | 'admin_reembed' | string
  started_at: string
  completed_at: string | null
  duration_ms: number | null
  success: boolean
  cancel_requested: boolean
  entities_processed: number
  phase_status: PhaseStatus[]
  error_message: string | null
}

export type BatchRunDetail = BatchRunSummary & {
  phases: PhaseEntry[]
  log_lines: LogLine[]
  target_strategy: TargetStrategy | null
}

export type LogLine = {
  ts: string
  level: 'info' | 'warn' | 'error' | string
  msg: string
}

type BatchRunStats = {
  total: number
  running: number
  ok: number
  failed: number
}

export type BatchRunsResponse = {
  items: BatchRunSummary[]
  total: number
  offset: number
  stats: BatchRunStats
}

type BatchRunStatsBucket = {
  ts: string
  ok: number
  failed: number
  cancelled: number
  avg_duration_ms: number
}

export type BatchRunStatsResponse = {
  window_seconds: number
  bucket_seconds: number
  series: BatchRunStatsBucket[]
}

export type BatchRunsFilter = {
  namespace?: string
  status?: 'running' | 'ok' | 'failed' | ''
  kind?: 'cf' | 'reembed' | ''
  limit?: number
  offset?: number
}

export type CreateBatchRunResponse = {
  id?: number
  namespace: string
  status: string
  started_at: string
}

// ---------------------------------------------------------------------------
// Queries
// ---------------------------------------------------------------------------

function batchRunsQueryParams(f: BatchRunsFilter): string {
  const params = new URLSearchParams()
  if (f.namespace) params.set('namespace', f.namespace)
  if (f.status) params.set('status', f.status)
  if (f.kind) params.set('kind', f.kind)
  if (f.limit != null) params.set('limit', String(f.limit))
  if (f.offset != null) params.set('offset', String(f.offset))
  const q = params.toString()
  return q ? `?${q}` : ''
}

export function useBatchRuns(filter: BatchRunsFilter = {}, opts?: { refetchInterval?: number | false }) {
  return useQuery({
    queryKey: queryKeys.batchRuns(filter as Record<string, unknown>),
    queryFn: () =>
      apiFetch<BatchRunsResponse>(`/api/admin/v1/batch-runs${batchRunsQueryParams(filter)}`),
    refetchInterval: opts?.refetchInterval ?? 30_000,
  })
}

export function useBatchRunDetail(
  id: number | string | null,
  options?: Omit<UseQueryOptions<BatchRunDetail>, 'queryKey' | 'queryFn'>,
) {
  return useQuery({
    queryKey: queryKeys.batchRunDetail(id ?? 'unknown'),
    queryFn: () => apiFetch<BatchRunDetail>(`/api/admin/v1/batch-runs/${id}`),
    enabled: id != null,
    ...options,
  })
}

export function useBatchRunStats(window: string = '24h', bucket: string = '1h') {
  return useQuery({
    queryKey: queryKeys.batchRunStats(window, bucket),
    queryFn: () =>
      apiFetch<BatchRunStatsResponse>(
        `/api/admin/v1/batch-runs/stats?window=${encodeURIComponent(window)}&bucket=${encodeURIComponent(bucket)}`,
      ),
    refetchInterval: 60_000,
  })
}

export function useCancelBatchRun() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: number) =>
      apiFetch<BatchRunSummary>(`/api/admin/v1/batch-runs/${id}/cancel`, {
        method: 'POST',
      }),
    onSuccess: (_data, id) => {
      qc.invalidateQueries({ queryKey: queryKeys.batchRunDetail(id) })
      qc.invalidateQueries({ queryKey: ['batch-runs'] })
    },
  })
}

export function useRetryBatchRun() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: number) =>
      apiFetch<CreateBatchRunResponse>(`/api/admin/v1/batch-runs/${id}/retry`, {
        method: 'POST',
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['batch-runs'] })
    },
  })
}

