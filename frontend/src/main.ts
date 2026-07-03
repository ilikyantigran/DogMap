import { createApp } from 'vue'
import { createPinia } from 'pinia'
import 'leaflet/dist/leaflet.css'
import './styles/main.css'
import App from './App.vue'
import { createAppRouter } from './router'
import { configureApi } from './api'
import { useAuthStore } from './stores/authStore'

const app = createApp(App)
const pinia = createPinia()
app.use(pinia)

// Wire the apiClient to the running app's auth store: read the current token for
// the auth_token header, and on any 401 clear the session + bounce to /login.
const router = createAppRouter()
const auth = useAuthStore()
configureApi({
  getToken: () => auth.token,
  onUnauthorized: () => {
    auth.clearSession()
    // Avoid redundant redirects if we're already on an auth page.
    if (router.currentRoute.value.meta.public !== true) {
      void router.push({ name: 'login' })
    }
  },
})

app.use(router)
app.mount('#app')
