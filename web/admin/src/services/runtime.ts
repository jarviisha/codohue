import { useQuery } from '@tanstack/react-query'
import { apiFetch } from './http'

export type RuntimeSnapshot = {
  process: string
  instance: string
  started_at: string
  reported_at: string
  settings: Array<{ name: string; value: string | number | boolean }>
}
export function useRuntime() {
  return useQuery({
    queryKey: ['system', 'runtime'],
    queryFn: () =>
      apiFetch<{ processes: RuntimeSnapshot[]; observed_at: string; expiry_seconds: number }>(
        '/api/admin/v1/runtime',
      ),
    refetchInterval: 30_000,
  })
}
