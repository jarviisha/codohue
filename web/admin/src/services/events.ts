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

export type InjectEventResponse = {
  ok: boolean
  event_id: number
}

// EventStreamMessage is the payload of an `event` frame on the live tail SSE.
// Shape mirrors EventSummary so tail rows and list rows render identically.
export type EventStreamMessage = EventSummary

export type EventsSummaryAction = {
  action: string
  count: number
  rate: number
}

export type EventsSummaryBucket = {
  ts: string
  count: number
}

export type EventsSummaryResponse = {
  window_seconds: number
  bucket_seconds: number
  total: number
  rate_per_second: number
  by_action: EventsSummaryAction[]
  series: EventsSummaryBucket[]
}

export type EventsSummaryWindow = '1m' | '5m' | '1h'

// ---------------------------------------------------------------------------
// Query keys
// ---------------------------------------------------------------------------

export const eventKeys = {
  list: (ns: string, filter: Record<string, unknown>) =>
    ['ns', ns, 'events', filter] as const,
  summary: (ns: string, window: string) =>
    ['ns', ns, 'events', 'summary', window] as const,
}

/**
 * eventsStreamPath builds the live-tail SSE URL with optional server-side
 * action / subject filters. Returns null when ns is absent so useServerStream
 * stays disconnected.
 */
export function eventsStreamPath(
  ns: string | null,
  filter: { action?: string; subjectId?: string } = {},
): string | null {
  if (!ns) return null
  const p = new URLSearchParams()
  if (filter.action) p.set('action', filter.action)
  if (filter.subjectId) p.set('subject_id', filter.subjectId)
  const q = p.toString()
  return `/api/admin/v1/namespaces/${ns}/events/stream${q ? `?${q}` : ''}`
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
      apiFetch<InjectEventResponse>(`/api/admin/v1/namespaces/${ns}/events`, {
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

/**
 * useEventsSummary polls the server-side aggregation backing the tail sidebar
 * (rate tiles, action mix, mini series). Refetches every 5s to match the
 * BUILD_PLAN §5.3 sidebar cadence.
 */
export function useEventsSummary(ns: string | null, window: EventsSummaryWindow = '1m') {
  return useQuery({
    queryKey: ns ? eventKeys.summary(ns, window) : ['ns', 'unknown', 'events', 'summary'],
    queryFn: () =>
      apiFetch<EventsSummaryResponse>(
        `/api/admin/v1/namespaces/${ns}/events/summary?window=${window}`,
      ),
    enabled: !!ns,
    refetchInterval: 5_000,
  })
}
