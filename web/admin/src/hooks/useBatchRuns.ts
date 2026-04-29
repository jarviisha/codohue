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

  phase1_ok: boolean | null
  phase1_duration_ms: number | null
  phase1_subjects: number | null
  phase1_objects: number | null
  phase1_error: string | null

  phase2_ok: boolean | null
  phase2_duration_ms: number | null
  phase2_items: number | null
  phase2_subjects: number | null
  phase2_error: string | null

  phase3_ok: boolean | null
  phase3_duration_ms: number | null
  phase3_items: number | null
  phase3_error: string | null
}

export function useBatchRuns(namespace?: string) {
  const params = namespace ? `?namespace=${encodeURIComponent(namespace)}` : ''
  return useQuery<{ runs: BatchRunLog[] }>({
    queryKey: ['batch-runs', namespace ?? ''],
    queryFn: () => api.get(`/api/admin/v1/batch-runs${params}`),
    refetchInterval: 30_000,
  })
}
