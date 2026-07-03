import { ApiError } from './errors'

// Thin, centralized HTTP client (Docs/03-Frontend.md "HTTP"):
//  - injects the opaque `auth_token` header on every call,
//  - centralizes error handling (backend `code`/`message` envelope),
//  - centralizes 401/expired-session handling via an injected callback.
//
// It is created with dependency-injected `getToken` / `onUnauthorized` so it has
// no import cycle with the stores and is trivial to unit-test. See
// `src/api/index.ts` for the wired singleton used by the app.

export interface ApiClientOptions {
  baseUrl: string
  /** Returns the current opaque token, or null when logged out. */
  getToken: () => string | null
  /** Invoked exactly once per request that fails with 401 (expired session). */
  onUnauthorized: () => void
}

export interface ApiClient {
  post<T = unknown>(path: string, body?: unknown): Promise<T>
  get<T = unknown>(path: string): Promise<T>
}

function joinUrl(baseUrl: string, path: string): string {
  const base = baseUrl.endsWith('/') ? baseUrl.slice(0, -1) : baseUrl
  const suffix = path.startsWith('/') ? path : `/${path}`
  return `${base}${suffix}`
}

export function createApiClient(options: ApiClientOptions): ApiClient {
  const { baseUrl, getToken, onUnauthorized } = options

  async function request<T>(
    method: 'GET' | 'POST',
    path: string,
    body?: unknown,
  ): Promise<T> {
    const headers = new Headers({ 'Content-Type': 'application/json' })
    const token = getToken()
    if (token) {
      // The backend expects the opaque session token in this exact header.
      headers.set('auth_token', token)
    }

    let response: Response
    try {
      response = await fetch(joinUrl(baseUrl, path), {
        method,
        headers,
        body: body === undefined ? undefined : JSON.stringify(body),
      })
    } catch (networkError) {
      // Network / CORS / offline failure — no HTTP status available.
      throw new ApiError(
        networkError instanceof Error
          ? networkError.message
          : 'Network request failed',
        -1,
        0,
      )
    }

    // Any 401 = expired/invalid session. Centralize the reaction here so no
    // store or component has to remember to handle it (Docs/03-Frontend.md:
    // "On any 401/expired session -> clear token, redirect to Auth").
    if (response.status === 401) {
      onUnauthorized()
    }

    const payload = await parseBody(response)

    // Prefer the backend's structured error contract over the HTTP status text.
    const code = typeof payload?.code === 'number' ? payload.code : null
    const message =
      typeof payload?.message === 'string' && payload.message.length > 0
        ? payload.message
        : response.statusText || 'Request failed'

    if (!response.ok) {
      throw new ApiError(message, code ?? response.status, response.status)
    }

    // Envelope success is code === 0. A non-zero code on a 2xx response is still
    // an application error (the doc puts code/message on every error path).
    if (code !== null && code !== 0) {
      throw new ApiError(message, code, response.status)
    }

    return payload as T
  }

  return {
    post: <T>(path: string, body?: unknown) => request<T>('POST', path, body),
    get: <T>(path: string) => request<T>('GET', path),
  }
}

async function parseBody(
  response: Response,
): Promise<{ code?: unknown; message?: unknown } & Record<string, unknown>> {
  const text = await response.text()
  if (!text) return {}
  try {
    return JSON.parse(text)
  } catch {
    // Non-JSON body (e.g. a gateway 502 HTML page). Surface it as a message.
    return { message: text }
  }
}
