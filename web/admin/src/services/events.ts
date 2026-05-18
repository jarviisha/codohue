import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { http } from './http'
import { namespaceKeys } from './namespaces'

// ────────────────────────────────────────────────────────────────────────────
// Wire types — mirror internal/admin/types.go
// ────────────────────────────────────────────────────────────────────────────

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
  items: EventSummary[]
  total: number
  limit: number
  offset: number
}

export interface InjectEventRequest {
  subject_id: string
  object_id: string
  action: string
  occurred_at?: string
}

export interface EventsListParams {
  namespace: string
  limit?: number
  offset?: number
  subject_id?: string
}

// Canonical action labels accepted by the data-plane ingest endpoint.
// Mirrors pkg/codohuetypes/event.go ActionView/Like/Comment/Share/Skip.
export const EVENT_ACTIONS = ['VIEW', 'LIKE', 'COMMENT', 'SHARE', 'SKIP'] as const
export type EventAction = (typeof EVENT_ACTIONS)[number]

// ────────────────────────────────────────────────────────────────────────────
// Query keys
// ────────────────────────────────────────────────────────────────────────────

export const eventKeys = {
  all: ['events'] as const,
  list: (namespace: string, params?: Omit<EventsListParams, 'namespace'>) =>
    [
      'events',
      'list',
      namespace,
      {
        limit: params?.limit ?? 100,
        offset: params?.offset ?? 0,
        subject_id: params?.subject_id ?? '',
      },
    ] as const,
}

// ────────────────────────────────────────────────────────────────────────────
// Request functions
// ────────────────────────────────────────────────────────────────────────────

function queryString(params: Record<string, string | number | undefined>) {
  const q = new URLSearchParams()
  for (const [key, value] of Object.entries(params)) {
    if (value === undefined || value === '') continue
    q.set(key, String(value))
  }
  const encoded = q.toString()
  return encoded ? `?${encoded}` : ''
}

export function listEvents(params: EventsListParams, signal?: AbortSignal) {
  const qs = queryString({
    limit: params.limit,
    offset: params.offset,
    subject_id: params.subject_id,
  })
  return http.get<EventsListResponse>(
    `/api/admin/v1/namespaces/${encodeURIComponent(params.namespace)}/events${qs}`,
    { signal },
  )
}

export function injectEvent(namespace: string, payload: InjectEventRequest) {
  return http.post<{ ok: boolean }>(
    `/api/admin/v1/namespaces/${encodeURIComponent(namespace)}/events`,
    payload,
  )
}

// ────────────────────────────────────────────────────────────────────────────
// Hooks
// ────────────────────────────────────────────────────────────────────────────

export interface UseEventsOptions extends EventsListParams {
  // Poll cadence in ms. 0 disables polling. Used by the page's live-tail
  // toggle to flip between idle (no poll) and a tight refetch cadence.
  refetchIntervalMs?: number
}

export function useEvents({ refetchIntervalMs, ...params }: UseEventsOptions) {
  return useQuery({
    queryKey: eventKeys.list(params.namespace, params),
    queryFn: ({ signal }) => listEvents(params, signal),
    enabled: Boolean(params.namespace),
    staleTime: 5_000,
    refetchInterval: refetchIntervalMs && refetchIntervalMs > 0 ? refetchIntervalMs : false,
  })
}

export function useInjectEvent() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ namespace, payload }: { namespace: string; payload: InjectEventRequest }) =>
      injectEvent(namespace, payload),
    onSuccess: (_data, vars) => {
      qc.invalidateQueries({ queryKey: ['events', 'list', vars.namespace] })
      qc.invalidateQueries({ queryKey: namespaceKeys.overview() })
    },
  })
}
