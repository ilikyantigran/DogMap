<script setup lang="ts">
import { useRouter } from 'vue-router'
import { storeToRefs } from 'pinia'
import { useAuthStore } from '@/stores/authStore'
import { useToastStore } from '@/stores/toastStore'
import ToastHost from '@/components/ToastHost.vue'

const auth = useAuthStore()
const { isAuthenticated } = storeToRefs(auth)
const toast = useToastStore()
const router = useRouter()

async function onLogout() {
  try {
    await auth.logout()
    toast.success('Logged out')
  } catch {
    // logout() clears the session even on error; nothing else to do.
  } finally {
    router.push({ name: 'login' })
  }
}
</script>

<template>
  <div>
    <nav v-if="isAuthenticated" class="app-nav">
      <RouterLink to="/map">Map</RouterLink>
      <RouterLink to="/profile">Profile</RouterLink>
      <span class="spacer" />
      <button @click="onLogout">Log out</button>
    </nav>

    <main class="app-main">
      <RouterView />
    </main>

    <ToastHost />
  </div>
</template>
