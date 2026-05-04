import { useMutation, useQueryClient } from '@tanstack/react-query'
import { api } from '../services/api'

export interface InjectEventRequest {
  subject_id: string
  object_id: string
  action: string
  occurred_at?: string
}

export function useInjectEvent(ns: string) {
  const queryClient = useQueryClient()
  return useMutation<{ ok: boolean }, Error, InjectEventRequest>({
    mutationFn: (req) => api.post(`/api/admin/v1/namespaces/${encodeURIComponent(ns)}/events`, req),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['events', ns] })
    },
  })
}
