import { useQuery } from '@tanstack/react-query'
import { adminApi } from '../services/adminApi'
import { queryKeys } from '../services/queryKeys'

export function useQdrantStats(ns: string) {
  return useQuery({
    queryKey: queryKeys.namespaces.qdrantStats(ns),
    queryFn: () => adminApi.getQdrantStats(ns),
    enabled: !!ns && ns !== 'new',
    refetchInterval: 30_000,
  })
}
