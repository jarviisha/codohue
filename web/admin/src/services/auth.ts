import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ApiError, http } from './http'

// Backend contract (see CLAUDE.md):
//   POST   /api/v1/auth/sessions          -> 201 + { expires_at }
//   DELETE /api/v1/auth/sessions/current  -> 204
//   Session is tracked by an httpOnly `codohue_admin_session` cookie.

export interface LoginPayload {
  api_key: string
}

export interface LoginResponse {
  expires_at: string
}

export const authKeys = {
  all:     ['auth'] as const,
  session: () => ['auth', 'session'] as const,
}

export async function login(payload: LoginPayload): Promise<LoginResponse> {
  return http.post<LoginResponse>('/api/v1/auth/sessions', payload, {
    redirectOnUnauthorized: false,
  })
}

export async function logout(): Promise<void> {
  await http.del<void>('/api/v1/auth/sessions/current', {
    redirectOnUnauthorized: false,
  })
}

// Probe whether the current session cookie is valid. Uses the admin health
// endpoint (cheapest authenticated GET available) and treats 401 as
// "no session" without triggering a redirect.
//
// Returns `true` if the request succeeds, `false` if the server says 401.
// Other errors propagate.
export async function probeSession(signal?: AbortSignal): Promise<boolean> {
  try {
    await http.get<unknown>('/api/admin/v1/health', {
      redirectOnUnauthorized: false,
      signal,
    })
    return true
  } catch (err) {
    if (err instanceof ApiError && err.status === 401) return false
    throw err
  }
}

export function useSession() {
  return useQuery({
    queryKey: authKeys.session(),
    queryFn: ({ signal }) => probeSession(signal),
    staleTime: 60_000,
    retry: false,
  })
}

export function useLogin() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: login,
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: authKeys.all })
    },
  })
}

export function useLogout() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: logout,
    onSuccess: () => {
      qc.clear()
    },
  })
}
