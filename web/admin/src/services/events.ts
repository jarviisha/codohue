import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { apiFetch } from './http'

// ---------------------------------------------------------------------------
// Wire types — mirror internal/admin/types.go::EventSummary / EventsListResponse
// / InjectEventRequest. Hand-maintained.
// ---------------------------------------------------------------------------

export type EventSummary = {
  id: number
  namespace: string
  subject_id: string
  object_id: string
  action: string
  weight: number
  occurred_at: string
}

export type EventsListResponse = {
  items: EventSummary[]
  total: number
  limit: number
  offset: number
}

export type InjectEventRequest = {
  subject_id: string
  object_id: string
  action: string
  occurred_at?: string
}

// ---------------------------------------------------------------------------
// Query keys
// ---------------------------------------------------------------------------

export const eventKeys = {
  list: (ns: string, filter: Record<string, unknown>) =>
    ['ns', ns, 'events', filter] as const,
}

// ---------------------------------------------------------------------------
// Hooks
// ---------------------------------------------------------------------------

export type EventsFilter = {
  subjectId?: string
  limit?: number
  offset?: number
}

function eventsQueryString(f: EventsFilter): string {
  const p = new URLSearchParams()
  if (f.subjectId) p.set('subject_id', f.subjectId)
  if (f.limit != null) p.set('limit', String(f.limit))
  if (f.offset != null) p.set('offset', String(f.offset))
  const q = p.toString()
  return q ? `?${q}` : ''
}

export function useRecentEvents(ns: string | null, filter: EventsFilter = {}) {
  return useQuery({
    queryKey: ns
      ? eventKeys.list(ns, filter as Record<string, unknown>)
      : ['ns', 'unknown', 'events'],
    queryFn: () =>
      apiFetch<EventsListResponse>(
        `/api/admin/v1/namespaces/${ns}/events${eventsQueryString(filter)}`,
      ),
    enabled: !!ns,
    refetchInterval: 10_000,
  })
}

export function useInjectEvent(ns: string | null) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (body: InjectEventRequest) =>
      apiFetch<{ ok: boolean }>(`/api/admin/v1/namespaces/${ns}/events`, {
        method: 'POST',
        body: JSON.stringify(body),
      }),
    onSuccess: () => {
      if (ns) {
        qc.invalidateQueries({ queryKey: ['ns', ns, 'events'] })
      }
    },
  })
}
