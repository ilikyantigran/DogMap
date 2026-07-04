<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { storeToRefs } from 'pinia'
import { useMapStore } from '@/stores/mapStore'
import { useFriendsStore } from '@/stores/friendsStore'
import { OBJECT_TYPE_META } from '@/lib/mapObjects'
import { toastError } from '@/lib/handleError'

// MapObjectPopup reads its object from the store BY ID so it stays reactive while
// the Leaflet popup is open. The store REPLACES object references on refresh
// (this.objects = ...), so a captured `object` prop snapshot would go stale and
// only update on a full page reload. Looking it up by id keeps it live.
const props = defineProps<{ id: string }>()

const map = useMapStore()
const friends = useFriendsStore()
const { friends: friendList } = storeToRefs(friends)

const object = computed(() => map.objects.find((o) => o.id === props.id) ?? null)
const meta = computed(() =>
  object.value ? OBJECT_TYPE_META[object.value.object_type] : null,
)

// Authoritative per-object flag from the backend — correct even right after a
// page refresh, so the user can't re-mark an object they're already at.
const amHere = computed(() => object.value?.viewer_visiting ?? false)

// Resolve friend ids present here to friendly logins where we know them.
const friendsHere = computed(() =>
  (object.value?.friend_ids_here ?? []).map((id) => {
    const f = friendList.value.find((fr) => fr.user_id === id)
    return f?.login ?? id
  }),
)

// Refresh this object's live counter whenever its popup opens or switches object
// (Map-2: reflect the count on click, not only on the poll tick).
onMounted(() => {
  void map.refreshObject(props.id).catch(() => {})
})
watch(
  () => props.id,
  (id) => void map.refreshObject(id).catch(() => {}),
)

const busy = ref(false)
async function toggle() {
  if (busy.value || !object.value) return
  busy.value = true
  try {
    await map.setVisiting(props.id, !amHere.value)
  } catch (err) {
    toastError(err)
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <div v-if="object && meta" class="map-popup">
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
