import { useQuery } from '@tanstack/react-query'
import { http } from './http'

// Backend type (internal/admin/types.go HealthResponse): flat probe map with
// a top-level summary status. Each field is a free-form string the backend
// emits — by convention "ok" means healthy; anything else is treated as
// degraded/down.
export interface HealthResponse {
  postgres: string
  redis: string
  qdrant: string
  status: string
}

export const healthKeys = {
  all: ['health'] as const,
}

export async function fetchHealth(signal?: AbortSignal): Promise<HealthResponse> {
  return http.get<HealthResponse>('/api/admin/v1/health', { signal })
}

export function useHealth() {
  return useQuery({
    queryKey: healthKeys.all,
    queryFn: ({ signal }) => fetchHealth(signal),
    refetchInterval: 30_000,
    refetchOnWindowFocus: true,
  })
}

// Map a probe string to the bracketed StatusToken state used everywhere in
// the UI (DESIGN.md §2.5). Conservative: only the literal "ok" passes; any
// other string is surfaced as warn so the operator notices the deviation.
export type ProbeState = 'ok' | 'warn'

export function probeState(value: string | undefined): ProbeState {
  return value === 'ok' ? 'ok' : 'warn'
}
