<script setup lang="ts">
import { computed } from 'vue'
import { storeToRefs } from 'pinia'
import { useMapStore } from '@/stores/mapStore'
import { useFriendsStore } from '@/stores/friendsStore'
import { toastError } from '@/lib/handleError'
import type { FriendPresence } from '@/types/api'

// FriendsOnMap: right-hand rail listing the caller's friends currently on a walk.
// Data comes from mapStore.friendsPresence (POST /v1/map/friends-presence, refreshed
// on the map poll tick). The friend's login is resolved from friendsStore
// (FriendsPresence returns ids only). Clicking a friend centers the map on their
// object and opens its popup (mapStore.focusFriendObject).
const map = useMapStore()
const friends = useFriendsStore()
const { friendsPresence } = storeToRefs(map)
const { friends: friendList } = storeToRefs(friends)

// A friend's display login, resolved from the friend graph; fall back to the id.
function loginFor(userId: string): string {
  return friendList.value.find((f) => f.user_id === userId)?.login ?? userId
}

// A named object shows its name; unnamed OSM features get a neutral placeholder.
function placeFor(fp: FriendPresence): string {
  return fp.object_name || 'a dog-friendly spot'
}

const walking = computed(() => friendsPresence.value)

async function focus(fp: FriendPresence) {
  try {
    await map.focusFriendObject(fp)
  } catch (err) {
    toastError(err)
  }
}
</script>

<template>
  <aside class="friends-on-map card" aria-label="Friends on a walk">
    <h3 style="margin-top: 0">Friends on a walk</h3>

    <p v-if="walking.length === 0" style="color: var(--dm-muted)">
      No friends are out right now.
    </p>

    <ul v-else style="list-style: none; padding: 0; margin: 0">
      <li v-for="fp in walking" :key="fp.user_id" style="padding: 0.35rem 0">
        <button
          type="button"
          class="friend-row"
          @click="focus(fp)"
          :title="`Show ${loginFor(fp.user_id)} on the map`"
        >
          <span class="friend-login">🐾 {{ loginFor(fp.user_id) }}</span>
          <span class="friend-place">at {{ placeFor(fp) }}</span>
        </button>
      </li>
    </ul>
  </aside>
</template>

<style scoped>
.friends-on-map {
  min-width: 200px;
}
.friend-row {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 0.1rem;
  width: 100%;
  text-align: left;
  background: none;
  border: none;
  padding: 0.35rem 0.5rem;
  border-radius: 6px;
  cursor: pointer;
}
.friend-row:hover {
  background: var(--dm-hover, rgba(0, 0, 0, 0.05));
}
.friend-login {
  color: var(--dm-primary);
  font-weight: 600;
}
.friend-place {
  font-size: 0.85rem;
  color: var(--dm-muted);
}
</style>
