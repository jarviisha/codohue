import { useQuery } from '@tanstack/react-query'
import { apiFetch } from './http'
import { queryKeys } from './queryKeys'
import type { HealthResponse } from './health'
import type { PhaseStatus } from './batchRuns'

export type NamespaceStatus = 'active' | 'idle' | 'degraded' | 'cold' | string

type CronHeartbeat = {
  last_run_at: string | null
  lag_seconds: number
  ok: boolean
}

type EmbedderHeartbeat = {
  last_seen_at: string | null
  ok: boolean
}

type AlertLevel = 'warn' | 'error'

type Alert = {
  level: AlertLevel
  namespace?: string
  kind: string
  message: string
}

type NamespaceOverviewRun = {
  id: number
  started_at: string
  success: boolean
  phase_status: PhaseStatus[]
}

type NamespaceOverviewCatalog = {
  enabled: boolean
  pending: number
  dead_letter: number
}

type NamespaceOverviewQdrant = {
  subjects: number
  objects: number
}

export type NamespaceOverview = {
  namespace: string
  status: NamespaceStatus
  last_run: NamespaceOverviewRun | null
  events_24h: number
  events_per_min_now: number
  catalog: NamespaceOverviewCatalog
  qdrant: NamespaceOverviewQdrant
}

export type OverviewResponse = {
  generated_at: string
  health: HealthResponse
  cron_heartbeat: CronHeartbeat
  embedder_heartbeat: EmbedderHeartbeat
  alerts: Alert[]
  namespaces: NamespaceOverview[]
}

export function useOverview() {
  return useQuery({
    queryKey: queryKeys.overview,
    queryFn: () => apiFetch<OverviewResponse>('/api/admin/v1/overview'),
    refetchInterval: 30_000,
  })
}
