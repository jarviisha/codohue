import { useQuery } from '@tanstack/react-query'
import { api } from '../services/api'

export interface TrendingAdminEntry {
  object_id: string
  score: number
  cache_ttl_sec: number
}

export interface TrendingAdminResponse {
  namespace: string
  items: TrendingAdminEntry[]
  window_hours: number
  limit: number
  offset: number
  total: number
  cache_ttl_sec: number
  generated_at: string
}

export function useTrending(namespace: string, limit = 50, offset = 0, windowHours = 0) {
  const params = new URLSearchParams({ limit: String(limit), offset: String(offset) })
  if (windowHours > 0) params.set('window_hours', String(windowHours))

  return useQuery<TrendingAdminResponse>({
    queryKey: ['trending', namespace, limit, offset, windowHours],
    queryFn: () => api.get(`/api/admin/v1/trending/${namespace}?${params}`),
    enabled: !!namespace,
  })
}
