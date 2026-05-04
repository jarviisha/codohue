import { useQuery } from '@tanstack/react-query'
import { api } from '../services/api'

export interface EventSummary {
  id: number
  namespace: string
  subject_id: string
  object_id: string
  action: string
  weight: number
  occurred_at: string
}

export interface EventsListResponse {
  events: EventSummary[]
  total: number
  limit: number
  offset: number
}

export function useEvents(ns: string, limit: number, offset: number, subjectID: string) {
  const params = new URLSearchParams({
    limit: String(limit),
    offset: String(offset),
  })
  if (subjectID) params.set('subject_id', subjectID)

  return useQuery<EventsListResponse>({
    queryKey: ['events', ns, limit, offset, subjectID],
    queryFn: () => api.get(`/api/admin/v1/namespaces/${encodeURIComponent(ns)}/events?${params}`),
    enabled: ns !== '',
    staleTime: 5_000,
  })
}
