import { useMutation, useQueryClient } from '@tanstack/react-query'
import { api } from '../services/api'

export interface TriggerBatchResponse {
  batch_run_id: number
  namespace: string
  started_at: string
  duration_ms: number
  success: boolean
}

export function useTriggerBatch(ns: string) {
  const queryClient = useQueryClient()
  return useMutation<TriggerBatchResponse, Error>({
    mutationFn: () => api.post(`/api/admin/v1/namespaces/${encodeURIComponent(ns)}/batch-runs/trigger`, {}),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['batch-runs'] })
      queryClient.invalidateQueries({ queryKey: ['namespaces-overview'] })
    },
  })
}
