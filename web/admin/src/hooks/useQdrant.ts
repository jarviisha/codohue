import { useQuery } from '@tanstack/react-query'
import { adminApi } from '../services/adminApi'
import { queryKeys } from '../services/queryKeys'

export function useQdrant(ns: string) {
  return useQuery({
    queryKey: queryKeys.namespaces.qdrantStats(ns),
    queryFn: () => adminApi.getQdrant(ns),
    enabled: !!ns && ns !== 'new',
    refetchInterval: 30_000,
  })
}
