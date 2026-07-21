const API_BASE = import.meta.env.VITE_ADMIN_API_BASE_URL ?? ''

class ApiError extends Error {
  status: number
  code: string

  constructor(status: number, code: string, message: string) {
    super(message)
    this.status = status
    this.code = code
    this.name = 'ApiError'
  }
}

/**
 * apiFetch is the single entry point for every admin REST call. The
 * `tests/urls.test.mjs` smoke gate fails the build if any other file calls
 * `fetch(` directly, so route every request through this helper.
 *
 * On 401 it dispatches a global `codohue:auth-expired` event — the router
 * subscribes once at mount and navigates the SPA to `/login`. SSE streams
 * use the same convention via `services/stream.ts`.
 */
export async function apiFetch<T = unknown>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(`${API_BASE}${path}`, {
    credentials: 'include',
    ...init,
    headers: {
      'Content-Type': 'application/json',
      ...init?.headers,
    },
  })

  if (res.status === 401) {
    window.dispatchEvent(new CustomEvent('codohue:auth-expired'))
    throw new ApiError(401, 'unauthorized', 'session expired')
  }

  if (!res.ok) {
    let code = 'http_error'
    let message = res.statusText || `HTTP ${res.status}`
    try {
      const body = (await res.json()) as { error?: { code?: string; message?: string } }
      if (body?.error?.code) code = body.error.code
      if (body?.error?.message) message = body.error.message
    } catch {
      // body was not JSON; keep defaults
    }
    throw new ApiError(res.status, code, message)
  }

  if (res.status === 204) {
    return undefined as T
  }
  return (await res.json()) as T
}

export const apiBaseUrl = API_BASE
