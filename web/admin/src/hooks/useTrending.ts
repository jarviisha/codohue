import { useQuery } from '@tanstack/react-query'
import { adminApi } from '../services/adminApi'
import { queryKeys } from '../services/queryKeys'

export function useTrending(namespace: string, limit = 50, offset = 0, windowHours = 0) {
  return useQuery({
    queryKey: queryKeys.trending.list(namespace, limit, offset, windowHours),
    queryFn: () => adminApi.getTrending({ namespace, limit, offset, windowHours }),
    enabled: !!namespace,
  })
}
