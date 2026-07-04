<script setup lang="ts">
import { createApp, onBeforeUnmount, onMounted, ref, watch, type App } from 'vue'
import { createPinia, getActivePinia } from 'pinia'
import L from 'leaflet'
import { storeToRefs } from 'pinia'
import { useMapStore } from '@/stores/mapStore'
import { OBJECT_TYPE_META } from '@/lib/mapObjects'
import { DEFAULT_ZOOM } from '@/config'
import MapObjectPopup from './MapObjectPopup.vue'
import type { MapObject } from '@/types/api'

// MapView: renders map_objects from LoadMap on a Leaflet + OpenStreetMap map.
// Dumb-ish: it reads objects/center from mapStore and reports selection back;
// it does NOT fetch or poll (that is the store's / page's job).
//
// The object popup is a real Vue component (MapObjectPopup) mounted into a
// Leaflet popup DOM node so it keeps full reactivity to the store.

const map = useMapStore()
const { objects, center, selectedId } = storeToRefs(map)

const el = ref<HTMLElement | null>(null)
let leaflet: L.Map | null = null
const markers = new Map<string, L.CircleMarker>()
// One mounted Vue app per open popup, torn down on close to avoid leaks.
const popupApps = new Map<string, App>()

function makeCircle(obj: MapObject): L.CircleMarker {
  return L.circleMarker([obj.latitude, obj.longitude], {
    radius: 9,
    color: OBJECT_TYPE_META[obj.object_type].color,
    fillColor: OBJECT_TYPE_META[obj.object_type].color,
    fillOpacity: 0.75,
    weight: 2,
  })
}

function mountPopup(id: string): HTMLElement {
  const container = document.createElement('div')
  // Reuse the app-wide Pinia so the popup component talks to the same stores.
  // Pass only the id: the popup reads the (live) object from the store itself so
  // it stays reactive when the store replaces objects on refresh.
  const app = createApp(MapObjectPopup, { id })
  app.use(getActivePinia() ?? createPinia())
  app.mount(container)
  popupApps.set(id, app)
  return container
}

function unmountPopup(id: string) {
  const app = popupApps.get(id)
  if (app) {
    app.unmount()
    popupApps.delete(id)
  }
}

function renderMarkers() {
  if (!leaflet) return
  const seen = new Set<string>()

  for (const obj of objects.value) {
    seen.add(obj.id)
    let marker = markers.get(obj.id)
    if (!marker) {
      marker = makeCircle(obj)
      marker.on('click', () => map.select(obj.id))
      marker.on('popupclose', () => unmountPopup(obj.id))
      marker.bindPopup(() => mountPopup(obj.id))
      marker.addTo(leaflet)
      markers.set(obj.id, marker)
    } else {
      marker.setLatLng([obj.latitude, obj.longitude])
    }
  }

  // Remove markers whose objects are gone from the latest LoadMap.
  for (const [id, marker] of markers) {
    if (!seen.has(id)) {
      unmountPopup(id)
      marker.remove()
      markers.delete(id)
    }
  }
}

onMounted(() => {
  if (!el.value) return
  leaflet = L.map(el.value).setView(
    [center.value.latitude, center.value.longitude],
    DEFAULT_ZOOM,
  )
  // OpenStreetMap tiles — free, no per-call billing (Docs/03-Frontend.md).
  L.tileLayer('https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png', {
    maxZoom: 19,
    attribution: '&copy; OpenStreetMap contributors',
  }).addTo(leaflet)
  renderMarkers()
})

// Re-render markers whenever the object list changes (polling updates it).
watch(objects, renderMarkers, { deep: true })

// Recenter when the store center changes (e.g. after geolocation resolves).
watch(center, (c) => {
  leaflet?.setView([c.latitude, c.longitude])
})

// Open the popup programmatically when selection changes (e.g. deep link).
watch(selectedId, (id) => {
  if (id && markers.has(id)) markers.get(id)!.openPopup()
})

onBeforeUnmount(() => {
  for (const id of [...popupApps.keys()]) unmountPopup(id)
  leaflet?.remove()
  leaflet = null
  markers.clear()
})
</script>

<template>
  <div ref="el" class="map-canvas" />
</template>
