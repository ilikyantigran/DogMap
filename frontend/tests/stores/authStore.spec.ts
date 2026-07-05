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

    expect(post).toHaveBeenCalledWith('/v1/auth/login', {
      email: 'a@b.com',
      password: 'pw',
    })
    expect(auth.token).toBe('tok')
    expect(auth.userId).toBe('u1')
    expect(auth.isAuthenticated).toBe(true)
  })

  it('register does NOT auto-login; records the pending-verification email', async () => {
    post.mockResolvedValueOnce({ code: 0, message: 'ok', user_id: 'u2' })
    const auth = useAuthStore()

    await auth.register({ login: 'Test1', email: 'a@b.com', password: 'pw' })

    // Only the register call happens — no login (email confirmation required).
    expect(post).toHaveBeenCalledTimes(1)
    expect(post).toHaveBeenCalledWith('/v1/auth/register', {
      login: 'Test1',
      email: 'a@b.com',
      password: 'pw',
    })
    expect(auth.token).toBeNull()
    expect(auth.isAuthenticated).toBe(false)
    expect(auth.pendingVerificationEmail).toBe('a@b.com')
  })

  it('verifyEmail posts the token to the verify endpoint', async () => {
    post.mockResolvedValue({ code: 0, message: 'ok' })
    const auth = useAuthStore()

    await auth.verifyEmail('vtok-123')

    expect(post).toHaveBeenCalledWith('/v1/auth/verify', { token: 'vtok-123' })
  })

  it('resendVerification posts the email to the resend endpoint', async () => {
    post.mockResolvedValue({ code: 0, message: 'ok' })
    const auth = useAuthStore()

    await auth.resendVerification('a@b.com')

    expect(post).toHaveBeenCalledWith('/v1/auth/resend-verification', {
      email: 'a@b.com',
    })
  })

  it('logout calls the backend and clears the token', async () => {
    post.mockResolvedValue({ code: 0, message: 'ok', token: 't', user_id: 'u1' })
    const auth = useAuthStore()
    await auth.login({ email: 'a@b.com', password: 'pw' })

    post.mockResolvedValue({ code: 0, message: 'ok' })
    await auth.logout()

    expect(post).toHaveBeenLastCalledWith('/v1/auth/logout')
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
