import {
  createRouter,
  createWebHistory,
  type RouteRecordRaw,
  type Router,
} from 'vue-router'
import { useAuthStore } from '@/stores/authStore'

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
  router.beforeEach((to) => {
    const auth = useAuthStore()

    // Guarded route while logged out -> send to login, remembering the target.
    if (to.meta.requiresAuth && !auth.isAuthenticated) {
      return { name: 'login', query: { redirect: to.fullPath } }
    }

    // Already authenticated but heading to an auth page -> go to the app.
    if (to.meta.public && auth.isAuthenticated) {
      return { name: 'map' }
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
