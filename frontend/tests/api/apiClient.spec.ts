import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createApiClient } from '@/api/apiClient'
import { ApiError } from '@/api/errors'

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  })
}

describe('apiClient', () => {
  let fetchMock: ReturnType<typeof vi.fn>

  beforeEach(() => {
    fetchMock = vi.fn()
    vi.stubGlobal('fetch', fetchMock)
  })

  it('injects the auth_token header when a token is present', async () => {
    fetchMock.mockResolvedValue(jsonResponse({ code: 0, message: 'ok' }))
    const client = createApiClient({
      baseUrl: '/api',
      getToken: () => 'opaque-token-123',
      onUnauthorized: vi.fn(),
    })

    await client.post('/profiles/EditUser', { name: 'A' })

    const [, init] = fetchMock.mock.calls[0]
    const headers = new Headers(init.headers)
    expect(headers.get('auth_token')).toBe('opaque-token-123')
  })

  it('omits the auth_token header when there is no token', async () => {
    fetchMock.mockResolvedValue(jsonResponse({ code: 0, message: 'ok' }))
    const client = createApiClient({
      baseUrl: '/api',
      getToken: () => null,
      onUnauthorized: vi.fn(),
    })

    await client.post('/auth/login', { password: 'x' })

    const [, init] = fetchMock.mock.calls[0]
    const headers = new Headers(init.headers)
    expect(headers.has('auth_token')).toBe(false)
  })

  it('returns the parsed JSON body on success', async () => {
    fetchMock.mockResolvedValue(
      jsonResponse({ code: 0, message: 'ok', token: 't', user_id: 'u1' }),
    )
    const client = createApiClient({
      baseUrl: '/api',
      getToken: () => null,
      onUnauthorized: vi.fn(),
    })

    const body = await client.post<{ token: string }>('/auth/login', {})
    expect(body.token).toBe('t')
  })

  it('throws ApiError with backend code/message on a non-zero envelope code', async () => {
    fetchMock.mockResolvedValue(
      jsonResponse({ code: 42, message: 'duplicate login' }, 200),
    )
    const client = createApiClient({
      baseUrl: '/api',
      getToken: () => null,
      onUnauthorized: vi.fn(),
    })

    await expect(client.post('/auth/register', {})).rejects.toMatchObject({
      code: 42,
      message: 'duplicate login',
    })
  })

  it('calls onUnauthorized and throws on 401', async () => {
    fetchMock.mockResolvedValue(
      jsonResponse({ code: 1, message: 'expired' }, 401),
    )
    const onUnauthorized = vi.fn()
    const client = createApiClient({
      baseUrl: '/api',
      getToken: () => 'expired-token',
      onUnauthorized,
    })

    await expect(client.post('/map/LoadMap', {})).rejects.toBeInstanceOf(
      ApiError,
    )
    expect(onUnauthorized).toHaveBeenCalledTimes(1)
  })

  it('joins baseUrl and path without a double slash', async () => {
    fetchMock.mockResolvedValue(jsonResponse({ code: 0, message: 'ok' }))
    const client = createApiClient({
      baseUrl: '/api/',
      getToken: () => null,
      onUnauthorized: vi.fn(),
    })

    await client.post('/auth/login', {})
    const [url] = fetchMock.mock.calls[0]
    expect(url).toBe('/api/auth/login')
  })
})
