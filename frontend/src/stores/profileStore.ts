import { defineStore } from 'pinia'
import { api } from '@/api'
import type {
  EditUserRequest,
  GetUserInfoRequest,
  Pet,
  UserInfo,
} from '@/types/api'
import { useAuthStore } from './authStore'

// Profile + pets: load self, edit (explicit save). Editing sends name/surname/
// phone/pets only — login/email are immutable at this layer (Docs/02-Backend.md
// EditUser). Acting user is the token owner; no user_id is sent.

interface ProfileState {
  profile: UserInfo | null
  loading: boolean
  saving: boolean
}

function emptyPet(): Pet {
  return { breed: '', name: '', sex: 'M', is_castrated: false, age: 0 }
}

export const useProfileStore = defineStore('profile', {
  state: (): ProfileState => ({
    profile: null,
    loading: false,
    saving: false,
  }),
  actions: {
    /** Load the current user's own full profile. */
    async loadSelf(): Promise<void> {
      const auth = useAuthStore()
      if (!auth.userId) return
      this.loading = true
      try {
        const req: GetUserInfoRequest = { user_id_target: auth.userId }
        this.profile = await api.post<UserInfo>('/profiles/GetUserInfo', req)
      } finally {
        this.loading = false
      }
    },

    /** Fetch another user's info (full if friend/self, reduced otherwise). */
    async getUserInfo(userIdTarget: string): Promise<UserInfo> {
      const req: GetUserInfoRequest = { user_id_target: userIdTarget }
      return api.post<UserInfo>('/profiles/GetUserInfo', req)
    },

    /** Explicit save (per product spec). Returns the updated full profile. */
    async save(edit: EditUserRequest): Promise<void> {
      this.saving = true
      try {
        this.profile = await api.post<UserInfo>('/profiles/EditUser', edit)
      } finally {
        this.saving = false
      }
    },

    addPet(): void {
      if (!this.profile) return
      this.profile.pets.push(emptyPet())
    },

    removePet(index: number): void {
      if (!this.profile) return
      this.profile.pets.splice(index, 1)
    },
  },
})
