const BASE = import.meta.env.VITE_ADMIN_API_BASE_URL ?? ''

export class ApiError extends Error {
  status: number
  code: string
  // body carries the full parsed response body for endpoints whose error
  // shape diverges from the standard {error: {code, message}} envelope.
  // The catalog PUT endpoint, for example, returns a flat
  // {error, strategy_dim, namespace_embedding_dim} on dim mismatch — the
  // form needs both numbers to render a precise message.
  body: unknown

  constructor(status: number, code: string, message: string, body?: unknown) {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.code = code
    this.body = body
  }
}

interface RequestOptions {
  redirectOnUnauthorized?: boolean
}

async function request<T>(
  method: string,
  path: string,
  body?: unknown,
  options: RequestOptions = {},
): Promise<T> {
  const redirectOnUnauthorized = options.redirectOnUnauthorized ?? true

  const res = await fetch(BASE + path, {
    method,
    credentials: 'include',
    headers: body ? { 'Content-Type': 'application/json' } : undefined,
    body: body ? JSON.stringify(body) : undefined,
  })

  if (res.status === 401 && redirectOnUnauthorized) {
    window.location.href = '/login'
    throw new ApiError(401, 'unauthorized', 'Session expired')
  }

  const data = await res.json().catch(() => null)

  if (!res.ok) {
    // Two error envelopes are produced by the admin API:
    //   1. Standard:  {error: {code, message}}
    //   2. Flat:      {error: "...", ...domain fields}  (e.g. dim mismatch)
    // Detect which we got so the message and code stay informative either way.
    const errField = (data as { error?: unknown } | null)?.error
    let code = 'unknown'
    let message = 'Request failed'
    if (errField && typeof errField === 'object') {
      const e = errField as { code?: string; message?: string }
      code = e.code ?? code
      message = e.message ?? message
    } else if (typeof errField === 'string') {
      message = errField
    }
    throw new ApiError(res.status, code, message, data)
  }

  return data as T
}

export const api = {
  get: <T>(path: string) => request<T>('GET', path),
  post: <T>(path: string, body: unknown) => request<T>('POST', path, body),
  put: <T>(path: string, body: unknown) => request<T>('PUT', path, body),
  delete: <T>(path: string) => request<T>('DELETE', path),
}

export async function login(apiKey: string): Promise<void> {
  await request('POST', '/api/v1/auth/sessions', { api_key: apiKey }, { redirectOnUnauthorized: false })
}

export async function logout(): Promise<void> {
  await request('DELETE', '/api/v1/auth/sessions/current')
}
