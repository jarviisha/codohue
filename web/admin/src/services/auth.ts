import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { apiFetch } from './http'
import { queryKeys } from './queryKeys'

type CreateSessionResponse = { expires_at: string }

/**
 * Probe the admin plane to check whether the current cookie still carries a
 * valid session. The current-session endpoint returns the individual operator identity.
 * On 401 the http helper has already
 * dispatched the auth-expired event by the time this throws.
 *
 * `retry: false` keeps the probe from masking a real auth failure with three
 * silent retries before the redirect fires.
 */
export function useSession() {
  return useQuery({
    queryKey: queryKeys.session,
    queryFn: async () => {
      const actor = await apiFetch<{ name: string; role: string }>('/api/v1/auth/sessions/current')
      return { ok: true, actor } as const
    },
    retry: false,
    staleTime: 30_000,
  })
}

export function useLogin() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (credentials: { username: string; password: string }) => {
      return apiFetch<CreateSessionResponse>('/api/v1/auth/sessions', {
        method: 'POST',
        body: JSON.stringify(credentials),
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
