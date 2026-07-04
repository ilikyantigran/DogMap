import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { createRouter, createMemoryHistory } from 'vue-router'

// Stub the api singleton; the guard's setup-check may load the profile.
const post = vi.fn()
vi.mock('@/api', () => ({
  api: { post: (...args: unknown[]) => post(...args), get: vi.fn() },
  configureApi: vi.fn(),
  apiBaseUrl: '/api',
}))

import { routes, installGuards } from '@/router'
import { useAuthStore } from '@/stores/authStore'
import { useProfileStore } from '@/stores/profileStore'
import { markSetupDone } from '@/lib/profileSetup'
import type { UserInfo } from '@/types/api'

function profileWith(name: string, surname: string): UserInfo {
  return {
    user_id: 'u1',
    login: 'ada',
    name,
    surname,
    email: 'ada@example.com',
    phone: '',
    pets: [],
    on_walk: false,
    current_object_id: null,
    friend_status: 'NONE',
  }
}

function makeRouter() {
  const router = createRouter({ history: createMemoryHistory(), routes })
  installGuards(router)
  return router
}

function authed() {
  const auth = useAuthStore()
  auth.token = 'tok'
  auth.userId = 'u1'
  return auth
}

beforeEach(() => {
  setActivePinia(createPinia())
  post.mockReset()
  localStorage.clear()
})

afterEach(() => {
  localStorage.clear()
})

describe('router — profile-setup redirect', () => {
  it('redirects to /profile/setup when authed, setup not done, and profile is empty', async () => {
    authed()
    useProfileStore().profile = profileWith('', '') // empty, already loaded
    const router = makeRouter()

    await router.push('/map')
    expect(router.currentRoute.value.name).toBe('profile-setup')
  })

  it('loads the profile in the guard when not preloaded, then redirects if empty', async () => {
    authed()
    post.mockResolvedValue(profileWith('', '')) // GetUserInfo -> empty profile
    const router = makeRouter()

    await router.push('/map')
    expect(post).toHaveBeenCalledWith('/v1/profiles/get', { user_id_target: 'u1' })
    expect(router.currentRoute.value.name).toBe('profile-setup')
  })

  it('does NOT redirect once setup is marked done (no fetch, cheap short-circuit)', async () => {
    authed()
    markSetupDone('u1')
    const router = makeRouter()

    await router.push('/map')
    expect(post).not.toHaveBeenCalled()
    expect(router.currentRoute.value.name).toBe('map')
  })

  it('does NOT redirect when the profile is non-empty', async () => {
    authed()
    useProfileStore().profile = profileWith('Ada', 'Lovelace')
    const router = makeRouter()

    await router.push('/map')
    expect(router.currentRoute.value.name).toBe('map')
  })

  it('does not loop: navigating to /profile/setup itself is allowed', async () => {
    authed()
    useProfileStore().profile = profileWith('', '')
    const router = makeRouter()

    await router.push('/profile/setup')
    expect(router.currentRoute.value.name).toBe('profile-setup')
  })

  it('does not block navigation if the profile fetch errors', async () => {
    authed()
    post.mockRejectedValue(new Error('boom'))
    const router = makeRouter()

    await router.push('/map')
    // Fetch failed -> we could not positively determine emptiness -> proceed.
    expect(router.currentRoute.value.name).toBe('map')
  })

  it('requires auth for /profile/setup (redirects to login when logged out)', async () => {
    const router = makeRouter()
    await router.push('/profile/setup')
    expect(router.currentRoute.value.name).toBe('login')
  })
})
