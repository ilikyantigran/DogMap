<script setup lang="ts">
import { onMounted, onUnmounted } from 'vue'
import { useProfileStore } from '@/stores/profileStore'
import { useFriendsStore } from '@/stores/friendsStore'
import { toastError } from '@/lib/handleError'
import ProfileForm from '@/components/profile/ProfileForm.vue'
import FriendsPanel from '@/components/profile/FriendsPanel.vue'

// Guarded Profile page. Loads profile once, and starts friends polling while the
// page is active (Docs/03-Frontend.md: "Profile page active -> friendsStore
// .refresh()"). Polling lifecycle is owned here; the store owns the interval.
const profileStore = useProfileStore()
const friendsStore = useFriendsStore()

function onVisibilityChange() {
  if (document.hidden) friendsStore.stopPolling()
  else friendsStore.startPolling()
}

onMounted(async () => {
  try {
    await profileStore.loadSelf()
  } catch (err) {
    toastError(err)
  }
  friendsStore.startPolling() // runs an immediate refresh, then polls
  document.addEventListener('visibilitychange', onVisibilityChange)
})

onUnmounted(() => {
  friendsStore.stopPolling()
  document.removeEventListener('visibilitychange', onVisibilityChange)
})
</script>

<template>
  <div>
    <h1>Profile</h1>
    <div class="profile-layout">
      <ProfileForm />
      <FriendsPanel />
    </div>
  </div>
</template>

<style scoped>
.profile-layout {
  display: grid;
  gap: 1rem;
  grid-template-columns: 1fr;
}
@media (min-width: 820px) {
  .profile-layout {
    grid-template-columns: 1fr 1fr;
    align-items: start;
  }
}
</style>
