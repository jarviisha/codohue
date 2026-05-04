import { useQuery } from '@tanstack/react-query'
import { adminApi } from '../services/adminApi'
import { queryKeys } from '../services/queryKeys'

export function useBatchRuns(namespace?: string) {
  return useQuery({
    queryKey: queryKeys.batchRuns.list(namespace),
    queryFn: () => adminApi.listBatchRuns(namespace),
    refetchInterval: 30_000,
  })
}
