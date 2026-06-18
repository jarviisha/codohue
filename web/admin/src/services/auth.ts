import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { apiFetch } from './http'
import { queryKeys } from './queryKeys'

type CreateSessionResponse = { expires_at: string }

/**
 * Probe the admin plane to check whether the current cookie still carries a
 * valid session. We use `/api/admin/v1/health` because it lives behind
 * RequireSession and returns 200 quickly; on 401 the http helper has already
 * dispatched the auth-expired event by the time this throws.
 *
 * `retry: false` keeps the probe from masking a real auth failure with three
 * silent retries before the redirect fires.
 */
export function useSession() {
  return useQuery({
    queryKey: queryKeys.session,
    queryFn: async () => {
      await apiFetch('/api/admin/v1/health')
      return { ok: true } as const
    },
    retry: false,
    staleTime: 30_000,
  })
}

export function useLogin() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (apiKey: string) => {
      return apiFetch<CreateSessionResponse>('/api/v1/auth/sessions', {
        method: 'POST',
        body: JSON.stringify({ api_key: apiKey }),
      })
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.session })
    },
  })
}

export function useLogout() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async () => {
      await apiFetch('/api/v1/auth/sessions/current', { method: 'DELETE' })
    },
    onSuccess: () => {
      qc.clear()
    },
  })
}
