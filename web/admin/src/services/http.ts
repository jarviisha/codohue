// HTTP transport for every admin-API call. Single entry point: `request<T>()`,
// plus convenience verb wrappers. Cookies travel automatically
// (`credentials: 'include'`). 401 responses redirect the page to /login
// unless explicitly disabled (e.g. on the login endpoint itself).

const BASE_URL: string = import.meta.env.VITE_ADMIN_API_BASE_URL ?? ''

export interface RequestOptions {
  redirectOnUnauthorized?: boolean
  signal?: AbortSignal
  headers?: Record<string, string>
}

export class ApiError extends Error {
  readonly status: number
  readonly code: string
  // The full parsed response body. Some endpoints (e.g. catalog dim-mismatch
  // 400s) return a flat shape rather than the standard error envelope.
  readonly body: unknown

  constructor(status: number, code: string, message: string, body?: unknown) {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.code = code
    this.body = body
  }
}

interface ErrorEnvelope {
  error?: { code?: string; message?: string }
}

function redirectToLogin() {
  if (typeof window === 'undefined') return
  if (window.location.pathname.startsWith('/login')) return
  const current = window.location.pathname + window.location.search
  window.location.replace(`/login?next=${encodeURIComponent(current)}`)
}

export async function request<T>(
  method: string,
  path: string,
  body?: unknown,
  options: RequestOptions = {},
): Promise<T> {
  const redirect = options.redirectOnUnauthorized ?? true
  const hasBody = body !== undefined

  const res = await fetch(BASE_URL + path, {
    method,
    credentials: 'include',
    headers: {
      ...(hasBody ? { 'Content-Type': 'application/json' } : {}),
      ...options.headers,
    },
    body: hasBody ? JSON.stringify(body) : undefined,
    signal: options.signal,
  })

  if (res.status === 204) {
    return undefined as T
  }

  let parsed: unknown = null
  if (res.headers.get('content-type')?.includes('application/json')) {
    try {
      parsed = await res.json()
    } catch {
      parsed = null
    }
  }

  if (!res.ok) {
    if (res.status === 401 && redirect) redirectToLogin()
    const envelope = parsed as ErrorEnvelope | null
    const code = envelope?.error?.code ?? `http_${res.status}`
    const message = envelope?.error?.message ?? `Request failed with status ${res.status}`
    throw new ApiError(res.status, code, message, parsed)
  }

  return parsed as T
}

export const http = {
  get:  <T>(path: string, opts?: RequestOptions) => request<T>('GET',    path, undefined, opts),
  post: <T>(path: string, body?: unknown, opts?: RequestOptions) => request<T>('POST',   path, body, opts),
  put:  <T>(path: string, body?: unknown, opts?: RequestOptions) => request<T>('PUT',    path, body, opts),
  del:  <T>(path: string, opts?: RequestOptions) => request<T>('DELETE', path, undefined, opts),
} as const
