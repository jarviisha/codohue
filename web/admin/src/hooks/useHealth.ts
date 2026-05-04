import { useQuery } from '@tanstack/react-query'
import { adminApi } from '../services/adminApi'
import { queryKeys } from '../services/queryKeys'

export function useHealth() {
  return useQuery({
    queryKey: queryKeys.health(),
    queryFn: adminApi.getHealth,
    refetchInterval: 10_000,
    retry: false,
  })
}
