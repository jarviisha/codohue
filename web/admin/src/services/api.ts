const BASE = import.meta.env.VITE_ADMIN_API_BASE_URL ?? ''

export class ApiError extends Error {
  status: number
  code: string

  constructor(status: number, code: string, message: string) {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.code = code
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
    const err = data?.error
    throw new ApiError(res.status, err?.code ?? 'unknown', err?.message ?? 'Request failed')
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
