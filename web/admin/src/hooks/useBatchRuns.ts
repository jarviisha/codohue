import { useQuery } from '@tanstack/react-query'
import { adminApi } from '../services/adminApi'
import { queryKeys } from '../services/queryKeys'

export const BATCH_PAGE_SIZE = 20

export function useBatchRuns(namespace?: string, page = 0, status = '') {
  const offset = page * BATCH_PAGE_SIZE
  return useQuery({
    queryKey: queryKeys.batchRuns.list(namespace, offset, status),
    queryFn: () => adminApi.listBatchRuns(namespace, BATCH_PAGE_SIZE, offset, status),
    refetchInterval: (query) => {
      const runs = query.state.data?.runs ?? []
      return runs.some(r => !r.completed_at) ? 5_000 : 30_000
    },
  })
}
