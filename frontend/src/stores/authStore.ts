import { defineStore } from 'pinia'
import { api } from '@/api'
import type {
  LoginRequest,
  LoginResponse,
  RegisterRequest,
  RegisterResponse,
  ResendVerificationResponse,
  VerifyEmailResponse,
} from '@/types/api'

// Auth state: opaque token + current user_id, plus login/register/logout and
// the route-guard helper (Docs/03-Frontend.md "State").
//
// TOKEN STORAGE: persisted to localStorage so a page refresh keeps the user
// logged in while the token is still valid (an expired/invalid token is cleared
// on the first 401 -> onUnauthorized -> clearSession). Trade-off accepted: this
// exposes the token to XSS (any injected script can read localStorage). All
// storage access is centralized here so no other code touches it directly.

const TOKEN_KEY = 'dogmap.token'
const USER_ID_KEY = 'dogmap.userId'

interface AuthState {
  token: string | null
  userId: string | null
  // Email of the account awaiting confirmation after register (no auto-login
  // anymore). Drives the "check your email" panel and the login Resend button.
  pendingVerificationEmail: string | null
}

export const useAuthStore = defineStore('auth', {
  // Hydrate from localStorage on init so a refresh restores the session.
  state: (): AuthState => ({
    token: localStorage.getItem(TOKEN_KEY),
    userId: localStorage.getItem(USER_ID_KEY),
    pendingVerificationEmail: null,
  }),
  getters: {
    isAuthenticated: (state): boolean => state.token !== null,
  },
  actions: {
    async login(payload: LoginRequest): Promise<void> {
      const res = await api.post<LoginResponse>('/v1/auth/login', payload)
      this.token = res.token
      this.userId = res.user_id
      localStorage.setItem(TOKEN_KEY, res.token)
      localStorage.setItem(USER_ID_KEY, res.user_id)
    },

    async register(payload: RegisterRequest): Promise<void> {
      // Email confirmation: register does NOT auto-login anymore. The account is
      // created unverified; the backend emails a /verify link. We record the
      // email so the UI can show "check your email" + a Resend affordance.
      await api.post<RegisterResponse>('/v1/auth/register', payload)
      this.pendingVerificationEmail = payload.email
    },

    /** Confirm an email via the token from the /verify?token= link. */
    async verifyEmail(token: string): Promise<void> {
      await api.post<VerifyEmailResponse>('/v1/auth/verify', { token })
    },

    /** Re-send the confirmation email. Always resolves (backend is generic). */
    async resendVerification(email: string): Promise<void> {
      await api.post<ResendVerificationResponse>(
        '/v1/auth/resend-verification',
        { email },
      )
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
      this.pendingVerificationEmail = null
      localStorage.removeItem(TOKEN_KEY)
      localStorage.removeItem(USER_ID_KEY)
    },
  },
})
