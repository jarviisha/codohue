import { useQuery } from '@tanstack/react-query'
import { api } from '../services/api'
import type { BatchRunLog } from './useBatchRuns'
import type { NamespaceConfig } from './useNamespaces'

export type NamespaceStatus = 'active' | 'idle' | 'degraded' | 'cold'

export interface NamespaceHealth {
  config: NamespaceConfig
  status: NamespaceStatus
  active_events_24h: number
  last_run: BatchRunLog | null
}

export function useNamespacesOverview() {
  return useQuery<{ namespaces: NamespaceHealth[] }>({
    queryKey: ['namespaces-overview'],
    queryFn: () => api.get('/api/admin/v1/namespaces/overview'),
    refetchInterval: 60_000,
  })
}
