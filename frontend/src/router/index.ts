import {
  createRouter,
  createWebHistory,
  type RouteRecordRaw,
  type Router,
} from 'vue-router'
import { useAuthStore } from '@/stores/authStore'
import { useProfileStore } from '@/stores/profileStore'
import { isProfileEmpty, isSetupDone } from '@/lib/profileSetup'

// Routes: Auth is public; Profile and Map are guarded (require a token).
// `requiresAuth` marks the guarded routes (Docs/03-Frontend.md "Route guards").
export const routes: RouteRecordRaw[] = [
  {
    path: '/',
    redirect: '/map',
  },
  {
    path: '/login',
    name: 'login',
    component: () => import('@/pages/AuthPage.vue'),
    props: { mode: 'login' },
    meta: { public: true },
  },
  {
    path: '/register',
    name: 'register',
    component: () => import('@/pages/AuthPage.vue'),
    props: { mode: 'register' },
    meta: { public: true },
  },
  {
    path: '/profile',
    name: 'profile',
    component: () => import('@/pages/ProfilePage.vue'),
    meta: { requiresAuth: true },
  },
  {
    // Two-step registration, step 2: one-time post-login profile setup.
    path: '/profile/setup',
    name: 'profile-setup',
    component: () => import('@/pages/ProfileSetupPage.vue'),
    meta: { requiresAuth: true },
  },
  {
    path: '/map',
    name: 'map',
    component: () => import('@/pages/MapPage.vue'),
    meta: { requiresAuth: true },
  },
  {
    path: '/:pathMatch(.*)*',
    redirect: '/map',
  },
]

/**
 * Install the global navigation guard on a router. Extracted so tests can
 * exercise the guard logic directly. Requires an active Pinia.
 */
export function installGuards(router: Router): void {
  router.beforeEach(async (to) => {
    const auth = useAuthStore()

    // Guarded route while logged out -> send to login, remembering the target.
    if (to.meta.requiresAuth && !auth.isAuthenticated) {
      return { name: 'login', query: { redirect: to.fullPath } }
    }

    // Already authenticated but heading to an auth page -> go to the app.
    if (to.meta.public && auth.isAuthenticated) {
      return { name: 'map' }
    }

    // Two-step registration: on the way to a guarded page, if this authenticated
    // user has never completed/skipped setup AND their profile is still empty,
    // divert them to the one-time ProfileSetup step. Excludes the setup route
    // itself (no redirect loop). The isSetupDone check is a cheap localStorage
    // short-circuit that avoids any fetch in the common (already-handled) case.
    if (
      to.meta.requiresAuth &&
      auth.isAuthenticated &&
      to.name !== 'profile-setup' &&
      auth.userId &&
      !isSetupDone(auth.userId)
    ) {
      const profileStore = useProfileStore()
      // Load the profile only if we don't already have it. Never block
      // navigation on a fetch error — we only redirect when we can POSITIVELY
      // determine the profile is empty.
      if (!profileStore.profile) {
        try {
          await profileStore.loadSelf()
        } catch {
          return true
        }
      }
      if (profileStore.profile && isProfileEmpty(profileStore.profile)) {
        return { name: 'profile-setup' }
      }
    }

    return true
  })
}

export function createAppRouter(): Router {
  const router = createRouter({
    history: createWebHistory(),
    routes,
  })
  installGuards(router)
  return router
}
