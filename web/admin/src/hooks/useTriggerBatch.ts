import { useMutation, useQueryClient } from '@tanstack/react-query'
import { adminApi } from '../services/adminApi'
import { queryKeys } from '../services/queryKeys'

export function useTriggerBatch(ns: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: () => adminApi.triggerBatchRun(ns),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['batch-runs'] })
      queryClient.invalidateQueries({ queryKey: queryKeys.namespaces.overview() })
    },
  })
}
