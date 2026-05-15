import { useQuery } from '@tanstack/react-query'
import { adminApi } from '../services/adminApi'
import { queryKeys } from '../services/queryKeys'

export function useEvents(ns: string, limit: number, offset: number, subjectID: string) {
  return useQuery({
    queryKey: queryKeys.events.list(ns, limit, offset, subjectID),
    queryFn: () => adminApi.listEvents({ namespace: ns, limit, offset, subjectID }),
    enabled: ns !== '',
    staleTime: 5_000,
  })
}
