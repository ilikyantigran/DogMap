import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { createRouter, createMemoryHistory } from 'vue-router'

vi.mock('@/api', () => ({
  api: { post: vi.fn(), get: vi.fn() },
  configureApi: vi.fn(),
  apiBaseUrl: '/api',
}))

import { routes, installGuards } from '@/router'
import { useAuthStore } from '@/stores/authStore'

function makeRouter() {
  const router = createRouter({ history: createMemoryHistory(), routes })
  installGuards(router)
  return router
}

describe('router guards', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  it('redirects guarded routes to /login when logged out', async () => {
    const router = makeRouter()
    await router.push('/map')
    expect(router.currentRoute.value.name).toBe('login')
    expect(router.currentRoute.value.query.redirect).toBe('/map')
  })

  it('allows guarded routes when authenticated', async () => {
    const auth = useAuthStore()
    auth.token = 'tok'
    auth.userId = 'u1'
    const router = makeRouter()

    await router.push('/profile')
    expect(router.currentRoute.value.name).toBe('profile')
  })

  it('keeps /login public when logged out', async () => {
    const router = makeRouter()
    await router.push('/login')
    expect(router.currentRoute.value.name).toBe('login')
  })

  it('keeps /verify public when logged out (reached from emailed link)', async () => {
    const router = makeRouter()
    await router.push('/verify?token=abc')
    expect(router.currentRoute.value.name).toBe('verify')
    expect(router.currentRoute.value.query.token).toBe('abc')
  })

  it('bounces an authenticated user away from /login to the map', async () => {
    const auth = useAuthStore()
    auth.token = 'tok'
    const router = makeRouter()

    await router.push('/login')
    expect(router.currentRoute.value.name).toBe('map')
  })
})
