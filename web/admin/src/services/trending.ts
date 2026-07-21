import { useQuery } from '@tanstack/react-query'
import { apiFetch } from './http'

// ---------------------------------------------------------------------------
// Wire types — mirror internal/admin/types.go::TrendingAdminResponse.
// ---------------------------------------------------------------------------

type TrendingAdminEntry = {
  object_id: string
  score: number
  /** -1 means no TTL (key persists), -2 means key missing from Redis. */
  cache_ttl_sec: number
}

export type TrendingAdminResponse = {
  namespace: string
  items: TrendingAdminEntry[]
  window_hours: number
  limit: number
  offset: number
  total: number
  cache_ttl_sec: number
  generated_at: string
}

// ---------------------------------------------------------------------------
// Query keys
// ---------------------------------------------------------------------------

const trendingKeys = {
  list: (ns: string, filter: Record<string, unknown>) =>
    ['ns', ns, 'trending', filter] as const,
}

// ---------------------------------------------------------------------------
// Hooks
// ---------------------------------------------------------------------------

export type TrendingFilter = {
  limit?: number
  offset?: number
  windowHours?: number
}

function trendingQueryString(f: TrendingFilter): string {
  const p = new URLSearchParams()
  if (f.limit != null) p.set('limit', String(f.limit))
  if (f.offset != null) p.set('offset', String(f.offset))
  if (f.windowHours != null) p.set('window_hours', String(f.windowHours))
  const q = p.toString()
  return q ? `?${q}` : ''
}

export function useTrending(ns: string | null, filter: TrendingFilter = {}) {
  return useQuery({
    queryKey: ns
      ? trendingKeys.list(ns, filter as Record<string, unknown>)
      : ['ns', 'unknown', 'trending'],
    queryFn: () =>
      apiFetch<TrendingAdminResponse>(
        `/api/admin/v1/namespaces/${ns}/trending${trendingQueryString(filter)}`,
      ),
    enabled: !!ns,
    refetchInterval: 30_000,
  })
}
