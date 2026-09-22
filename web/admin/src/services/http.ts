const API_BASE = import.meta.env.VITE_ADMIN_API_BASE_URL ?? ''

export class ApiError extends Error {
  status: number
  code: string
  details?: { fields?: Record<string, string>; current?: unknown }

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
      'X-Codohue-CSRF': '1',
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
    let details: ApiError['details']
    try {
      const body = (await res.json()) as {
        error?: {
          code?: string
          message?: string
          fields?: Record<string, string>
          current?: unknown
        }
      }
      details = body.error
      if (body?.error?.code) code = body.error.code
      if (body?.error?.message) message = body.error.message
    } catch {
      // body was not JSON; keep defaults
    }
    const error = new ApiError(res.status, code, message)
    error.details = details
    throw error
  }

  if (res.status === 204) {
    return undefined as T
  }
  return (await res.json()) as T
}

export const apiBaseUrl = API_BASE

/**
 * isAuthError distinguishes "the session is gone" from "the request failed".
 * AuthGuard needs the split: bouncing to /login on a 500 or a dropped
 * connection logs out an operator whose session is perfectly valid.
 */
export function isAuthError(error: unknown): boolean {
  return error instanceof ApiError && error.status === 401
}
