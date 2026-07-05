<script setup lang="ts">
import { computed, onMounted, onUnmounted } from 'vue'
import { useRoute } from 'vue-router'
import { storeToRefs } from 'pinia'
import { useMapStore } from '@/stores/mapStore'
import { useFriendsStore } from '@/stores/friendsStore'
import { getCurrentPosition } from '@/lib/geolocation'
import { useToastStore } from '@/stores/toastStore'
import MapView from '@/components/map/MapView.vue'
import MapLegend from '@/components/map/MapLegend.vue'
import FriendsOnMap from '@/components/map/FriendsOnMap.vue'

// Guarded Map page. Owns polling lifecycle; the store owns the intervals.
//  - request geolocation to center LoadMap (fallback: default center + manual pan)
//  - start map refresh polling while active; stop on unmount / tab hidden
//  - a friend's "where" link deep-links via ?object=<id> to open that popup
const map = useMapStore()
const friends = useFriendsStore()
const toast = useToastStore()
const route = useRoute()
const { objects, loading } = storeToRefs(map)

const isEmpty = computed(() => !loading.value && objects.value.length === 0)

function onVisibilityChange() {
  if (document.hidden) map.stopPolling()
  else map.startPolling()
}

onMounted(async () => {
  // Center on the user's location if granted; otherwise keep the default.
  try {
    const pos = await getCurrentPosition()
    map.setCenter(pos)
  } catch {
    toast.info('Using default location — pan the map to explore.')
  }

  map.startPolling() // immediate LoadMap, then polls
  // Friends list backs the "friends here" login lookup in the popup.
  friends.refresh().catch(() => {})

  document.addEventListener('visibilitychange', onVisibilityChange)

  // Deep link from FriendsPanel "where" link: select that object.
  const objectId = route.query.object
  if (typeof objectId === 'string') map.select(objectId)
})

onUnmounted(() => {
  // Stop ALL polling (map refresh + presence heartbeat) on leave.
  map.stopPolling()
  document.removeEventListener('visibilitychange', onVisibilityChange)
})
</script>

<template>
  <div>
    <h1>Map</h1>
    <MapLegend />
    <div class="map-layout">
      <div class="map-main">
        <MapView />
        <p v-if="isEmpty" style="color: var(--dm-muted)">
          No dog-friendly places nearby. Try panning the map.
        </p>
        <p v-if="loading" style="color: var(--dm-muted)">Loading nearby places…</p>
      </div>
      <!-- Right-hand rail: friends currently on a walk; click to jump to them. -->
      <FriendsOnMap class="map-rail" />
    </div>
  </div>
</template>

<style scoped>
.map-layout {
  display: flex;
  gap: 1rem;
  align-items: flex-start;
}
.map-main {
  flex: 1;
  min-width: 0;
}
.map-rail {
  flex: 0 0 220px;
}
@media (max-width: 720px) {
  .map-layout {
    flex-direction: column;
  }
  .map-rail {
    flex: 1;
    width: 100%;
  }
}
</style>
