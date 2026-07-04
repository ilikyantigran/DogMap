<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { storeToRefs } from 'pinia'
import { useMapStore } from '@/stores/mapStore'
import { useFriendsStore } from '@/stores/friendsStore'
import { OBJECT_TYPE_META } from '@/lib/mapObjects'
import { toastError } from '@/lib/handleError'
import type { MapObject } from '@/types/api'

// MapObjectPopup: type, visitor_count, friends-here (from friend_ids_here),
// and the "I'm going here" / "Not going" toggle -> mapStore.setVisiting.
// Privacy: strangers only ever contribute to the count; only friend ids are
// exposed for the caller, and we map those ids to logins we already know.
const props = defineProps<{ object: MapObject }>()

const map = useMapStore()
const friends = useFriendsStore()
const { friends: friendList } = storeToRefs(friends)

const meta = computed(() => OBJECT_TYPE_META[props.object.object_type])

// Authoritative per-object flag from the backend — correct even right after a
// page refresh, so the user can't re-mark an object they're already at.
const amHere = computed(() => props.object.viewer_visiting)

// Refresh this object's live counter whenever its popup opens or switches to a
// different object (Map-2: reflect the count on click, not only on the poll tick).
onMounted(() => {
  void map.refreshObject(props.object.id).catch(() => {})
})
watch(
  () => props.object.id,
  (id) => void map.refreshObject(id).catch(() => {}),
)

// Resolve friend ids present here to friendly logins where we know them.
const friendsHere = computed(() =>
  (props.object.friend_ids_here ?? []).map((id) => {
    const f = friendList.value.find((fr) => fr.user_id === id)
    return f?.login ?? id
  }),
)

const busy = ref(false)
async function toggle() {
  if (busy.value) return
  busy.value = true
  try {
    await map.setVisiting(props.object.id, !amHere.value)
  } catch (err) {
    toastError(err)
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <div class="map-popup">
    <strong>{{ meta.label }}</strong>

    <!-- visitor_count for everyone; empty state per spec ("0 people here"). -->
    <div style="margin: 0.35rem 0">
      {{ object.visitor_count }}
      {{ object.visitor_count === 1 ? 'person' : 'people' }} here
    </div>

    <!-- friends here (identity is friends-only). -->
    <div v-if="friendsHere.length" style="font-size: 0.85rem; color: var(--dm-primary)">
      Friends here: {{ friendsHere.join(', ') }}
    </div>

    <button
      type="button"
      class="primary"
      :disabled="busy"
      style="margin-top: 0.5rem"
      @click="toggle"
    >
      {{ amHere ? 'Not going' : "I'm going here" }}
    </button>
  </div>
</template>

<style scoped>
.map-popup {
  min-width: 160px;
}
</style>
