import { useMutation, useQueryClient } from '@tanstack/react-query'
import { adminApi } from '../services/adminApi'
import { queryKeys } from '../services/queryKeys'
import type { InjectEventRequest } from '../types'

export function useInjectEvent(ns: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (req: InjectEventRequest) => adminApi.injectEvent(ns, req),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.events.namespace(ns) })
    },
  })
}
