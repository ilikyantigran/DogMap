import { defineStore } from 'pinia'
import { api } from '@/api'
import type {
  LoginRequest,
  LoginResponse,
  RegisterRequest,
  RegisterResponse,
} from '@/types/api'

// Auth state: opaque token + current user_id, plus login/register/logout and
// the route-guard helper (Docs/03-Frontend.md "State").
//
// TOKEN STORAGE: in-memory only (Pinia state). This is the safer MVP choice per
// Docs/03-Frontend.md — it avoids the XSS exposure of localStorage, at the cost
// of requiring a re-login on a full page refresh (the token is lost on reload).
// TODO(token-storage): if the re-login-on-refresh UX proves too painful, we can
// persist the token to localStorage instead. Trade-off: convenience (survives
// refresh) vs. XSS risk (any injected script can read localStorage). If we do,
// centralize it here (load on init, write on login, clear on logout/401) so no
// other code touches storage directly. Decision deferred; in-memory for now.

interface AuthState {
  token: string | null
  userId: string | null
}

export const useAuthStore = defineStore('auth', {
  state: (): AuthState => ({
    token: null,
    userId: null,
  }),
  getters: {
    isAuthenticated: (state): boolean => state.token !== null,
  },
  actions: {
    async login(payload: LoginRequest): Promise<void> {
      const res = await api.post<LoginResponse>('/v1/auth/login', payload)
      this.token = res.token
      this.userId = res.user_id
    },

    async register(payload: RegisterRequest): Promise<void> {
      // Register, then auto-login with the same credentials
      // (Docs/03-Frontend.md: RegisterForm "-> POST /v1/auth/register, then auto-login").
      await api.post<RegisterResponse>('/v1/auth/register', payload)
      await this.login({
        login: payload.login,
        email: payload.email,
        password: payload.password,
      })
    },

    async logout(): Promise<void> {
      try {
        // Best-effort server-side revocation (deletes session:{token} in Valkey).
        await api.post('/v1/auth/logout')
      } finally {
        this.clearSession()
      }
    },

    /**
     * Wipe the session locally WITHOUT calling the backend. Used by the 401
     * handler (the token is already invalid, so there is nothing to revoke).
     */
    clearSession(): void {
      this.token = null
      this.userId = null
    },
  },
})
