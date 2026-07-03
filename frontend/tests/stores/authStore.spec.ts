import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'

// Mock the wired api singleton used by the store.
vi.mock('@/api', () => ({
  api: { post: vi.fn(), get: vi.fn() },
  configureApi: vi.fn(),
  apiBaseUrl: '/api',
}))

import { api } from '@/api'
import { useAuthStore } from '@/stores/authStore'

const post = api.post as unknown as ReturnType<typeof vi.fn>

describe('authStore', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    post.mockReset()
  })

  it('starts logged out', () => {
    const auth = useAuthStore()
    expect(auth.token).toBeNull()
    expect(auth.isAuthenticated).toBe(false)
  })

  it('login stores the token and user_id in memory', async () => {
    post.mockResolvedValue({ code: 0, message: 'ok', token: 'tok', user_id: 'u1' })
    const auth = useAuthStore()

    await auth.login({ email: 'a@b.com', password: 'pw' })

    expect(post).toHaveBeenCalledWith('/auth/login', {
      email: 'a@b.com',
      password: 'pw',
    })
    expect(auth.token).toBe('tok')
    expect(auth.userId).toBe('u1')
    expect(auth.isAuthenticated).toBe(true)
  })

  it('register then auto-logins (Path 1 step 1-2)', async () => {
    post
      .mockResolvedValueOnce({ code: 0, message: 'ok', user_id: 'u2' })
      .mockResolvedValueOnce({ code: 0, message: 'ok', token: 'tok2', user_id: 'u2' })
    const auth = useAuthStore()

    await auth.register({ login: 'Test1', email: 'a@b.com', password: 'pw' })

    // Registered, then logged in automatically.
    expect(post).toHaveBeenNthCalledWith(1, '/auth/register', {
      login: 'Test1',
      email: 'a@b.com',
      password: 'pw',
    })
    expect(auth.token).toBe('tok2')
    expect(auth.isAuthenticated).toBe(true)
  })

  it('logout calls the backend and clears the token', async () => {
    post.mockResolvedValue({ code: 0, message: 'ok', token: 't', user_id: 'u1' })
    const auth = useAuthStore()
    await auth.login({ email: 'a@b.com', password: 'pw' })

    post.mockResolvedValue({ code: 0, message: 'ok' })
    await auth.logout()

    expect(post).toHaveBeenLastCalledWith('/auth/logout')
    expect(auth.token).toBeNull()
    expect(auth.userId).toBeNull()
    expect(auth.isAuthenticated).toBe(false)
  })

  it('clearSession wipes token without calling the backend (used on 401)', () => {
    const auth = useAuthStore()
    auth.token = 'tok'
    auth.userId = 'u1'

    auth.clearSession()

    expect(auth.token).toBeNull()
    expect(auth.userId).toBeNull()
    expect(post).not.toHaveBeenCalled()
  })
})
