import { useMutation, useQueryClient } from '@tanstack/react-query'
import { adminApi } from '../services/adminApi'
import { queryKeys } from '../services/queryKeys'

export function useSeedDemoDataset() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: () => adminApi.seedDemoDataset(),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.namespaces.overview() })
      queryClient.invalidateQueries({ queryKey: queryKeys.namespaces.list() })
      queryClient.invalidateQueries({ queryKey: queryKeys.events.namespace('demo') })
    },
  })
}

export function useClearDemoDataset() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: () => adminApi.clearDemoDataset(),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.namespaces.overview() })
      queryClient.invalidateQueries({ queryKey: queryKeys.namespaces.list() })
      queryClient.invalidateQueries({ queryKey: queryKeys.events.namespace('demo') })
      queryClient.invalidateQueries({ queryKey: queryKeys.namespaces.detail('demo') })
      queryClient.invalidateQueries({ queryKey: queryKeys.namespaces.qdrantStats('demo') })
    },
  })
}
