import { useQuery } from '@tanstack/react-query'
import { apiFetch } from './http'
import { queryKeys } from './queryKeys'

export type ComponentStatus = 'ok' | 'degraded' | 'unknown' | 'error' | string

export type HealthResponse = {
  postgres: ComponentStatus
  redis: ComponentStatus
  qdrant: ComponentStatus
  status: ComponentStatus
}

export function useHealth() {
  return useQuery({
    queryKey: queryKeys.health,
    queryFn: () => apiFetch<HealthResponse>('/api/admin/v1/health'),
    refetchInterval: 30_000,
  })
}
