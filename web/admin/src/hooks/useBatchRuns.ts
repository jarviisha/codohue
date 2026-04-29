import { useQuery } from '@tanstack/react-query'
import { api } from '../services/api'

export interface BatchRunLog {
  id: number
  namespace: string
  started_at: string
  completed_at: string | null
  duration_ms: number | null
  subjects_processed: number
  success: boolean
  error_message: string | null
}

export function useBatchRuns(namespace?: string) {
  const params = namespace ? `?namespace=${encodeURIComponent(namespace)}` : ''
  return useQuery<{ runs: BatchRunLog[] }>({
    queryKey: ['batch-runs', namespace ?? ''],
    queryFn: () => api.get(`/api/admin/v1/batch-runs${params}`),
    refetchInterval: 30_000,
  })
}
