import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { mount } from '@vue/test-utils'

// Stub the api singleton so the store actions don't hit the network.
const post = vi.fn()
vi.mock('@/api', () => ({
  api: { post: (...args: unknown[]) => post(...args), get: vi.fn() },
  configureApi: vi.fn(),
  apiBaseUrl: '/api',
}))

// Router pushes are captured via a stub.
const push = vi.fn()
vi.mock('vue-router', () => ({
  useRouter: () => ({ push }),
}))

import ProfileSetupPage from '@/pages/ProfileSetupPage.vue'
import { useAuthStore } from '@/stores/authStore'
import { useProfileStore } from '@/stores/profileStore'
import { isSetupDone } from '@/lib/profileSetup'
import type { UserInfo } from '@/types/api'

function emptyProfile(): UserInfo {
  return {
    user_id: 'u1',
    login: 'ada',
    name: '',
    surname: '',
    email: 'ada@example.com',
    phone: '',
    pets: [],
    on_walk: false,
    current_object_id: null,
    friend_status: 'NONE',
  }
}

beforeEach(() => {
  setActivePinia(createPinia())
  post.mockReset()
  push.mockReset()
  localStorage.clear()
  const auth = useAuthStore()
  auth.token = 'tok'
  auth.userId = 'u1'
})

afterEach(() => {
  localStorage.clear()
})

describe('ProfileSetupPage', () => {
  it('Complete saves via EditUser, marks setup done, and routes to /map', async () => {
    const profileStore = useProfileStore()
    profileStore.profile = emptyProfile()
    // First post = get self (not needed, already loaded); save returns updated profile.
    post.mockResolvedValue({ ...emptyProfile(), name: 'Ada', surname: 'Lovelace' })

    const wrapper = mount(ProfileSetupPage)
    await wrapper.vm.$nextTick()

    // Fill name + surname
    const nameInput = wrapper.find('[data-test="setup-name"]')
    const surnameInput = wrapper.find('[data-test="setup-surname"]')
    expect(nameInput.exists()).toBe(true)
    expect(surnameInput.exists()).toBe(true)
    await nameInput.setValue('Ada')
    await surnameInput.setValue('Lovelace')

    await wrapper.find('form').trigger('submit')
    await wrapper.vm.$nextTick()
    await wrapper.vm.$nextTick()

    // Saved through the profiles/edit endpoint (EditUser).
    expect(post).toHaveBeenCalledWith(
      '/v1/profiles/edit',
      expect.objectContaining({ name: 'Ada', surname: 'Lovelace' }),
    )
    expect(isSetupDone('u1')).toBe(true)
    expect(push).toHaveBeenCalledWith('/map')
  })

  it('Skip marks setup done WITHOUT saving and routes to /map', async () => {
    const profileStore = useProfileStore()
    profileStore.profile = emptyProfile()

    const wrapper = mount(ProfileSetupPage)
    await wrapper.vm.$nextTick()

    await wrapper.find('[data-test="setup-skip"]').trigger('click')
    await wrapper.vm.$nextTick()

    // No EditUser call on skip.
    expect(post).not.toHaveBeenCalledWith('/v1/profiles/edit', expect.anything())
    expect(isSetupDone('u1')).toBe(true)
    expect(push).toHaveBeenCalledWith('/map')
  })

  it('loads self on mount when the profile is not already loaded', async () => {
    // No profile preloaded -> the page should ask the store to load it.
    post.mockResolvedValue(emptyProfile())

    mount(ProfileSetupPage)
    await Promise.resolve()
    await Promise.resolve()

    expect(post).toHaveBeenCalledWith('/v1/profiles/get', { user_id_target: 'u1' })
  })
})
