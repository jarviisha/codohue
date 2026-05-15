import { useQuery } from '@tanstack/react-query'
import { adminApi } from '../services/adminApi'
import { queryKeys } from '../services/queryKeys'

export function useNamespacesOverview() {
  return useQuery({
    queryKey: queryKeys.namespaces.overview(),
    queryFn: adminApi.getNamespacesOverview,
    refetchInterval: 60_000,
  })
}
