import { useQuery } from '@tanstack/react-query'
import { http } from './http'

// ────────────────────────────────────────────────────────────────────────────
// Wire types — mirror internal/admin/types.go
// ────────────────────────────────────────────────────────────────────────────

export interface TrendingItem {
  object_id: string
  score: number
  // Redis TTL on the per-key trending entry. -1 = no expiry, -2 = key missing.
  cache_ttl_sec: number
}

export interface TrendingAdminResponse {
  namespace: string
  items: TrendingItem[]
  window_hours: number
  limit: number
  offset: number
  total: number
  // Redis TTL on the namespace-level trending ZSET. Same -1/-2 convention.
  cache_ttl_sec: number
  generated_at: string
}

export interface TrendingListParams {
  namespace: string
  limit?: number
  offset?: number
  window_hours?: number
}

// Available windows for the page selector. 7d collapses to 168h to match the
// Redis ZSET retention strategy used by the data plane.
export const TRENDING_WINDOWS = [
  { label: '1h', value: 1 },
  { label: '6h', value: 6 },
  { label: '24h', value: 24 },
  { label: '7d', value: 168 },
] as const

export type TrendingWindowHours = (typeof TRENDING_WINDOWS)[number]['value']

// ────────────────────────────────────────────────────────────────────────────
// Query keys
// ────────────────────────────────────────────────────────────────────────────

export const trendingKeys = {
  all: ['trending'] as const,
  list: (namespace: string, params?: Omit<TrendingListParams, 'namespace'>) =>
    [
      'trending',
      'list',
      namespace,
      {
        limit: params?.limit ?? 50,
        offset: params?.offset ?? 0,
        window_hours: params?.window_hours ?? 0,
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

export function listTrending(params: TrendingListParams, signal?: AbortSignal) {
  const qs = queryString({
    limit: params.limit,
    offset: params.offset,
    window_hours: params.window_hours,
  })
  return http.get<TrendingAdminResponse>(
    `/api/admin/v1/namespaces/${encodeURIComponent(params.namespace)}/trending${qs}`,
    { signal },
  )
}

// ────────────────────────────────────────────────────────────────────────────
// Hooks
// ────────────────────────────────────────────────────────────────────────────

export function useTrending(params: TrendingListParams) {
  return useQuery({
    queryKey: trendingKeys.list(params.namespace, params),
    queryFn: ({ signal }) => listTrending(params, signal),
    enabled: Boolean(params.namespace),
    staleTime: 10_000,
  })
}
