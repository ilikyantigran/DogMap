import { defineStore } from 'pinia'
import { api } from '@/api'
import { createPoller, type Poller } from '@/lib/poller'
import {
  MAP_REFRESH_MS,
  PRESENCE_HEARTBEAT_MS,
  DEFAULT_CENTER,
} from '@/config'
import type {
  ChangeMapObjectStatusRequest,
  LoadMapRequest,
  LoadMapResponse,
  MapObject,
  MapObjectResponse,
} from '@/types/api'

// Map objects + presence. Owns: nearby objects, selected object, my current
// presence, and the two polling loops (map refresh + presence heartbeat), all
// behind store actions so components never poll (Docs/03-Frontend.md).

interface Center {
  latitude: number
  longitude: number
}

interface MapState {
  objects: MapObject[]
  center: Center
  selectedId: string | null
  // The object this user is currently "visiting", or null. Presence is derived,
  // point-of-interest (not GPS) — we track only which object id, never coords.
  myPresenceObjectId: string | null
  loading: boolean
}

// Timers held outside reactive state so Pinia doesn't proxy them.
let refreshPoller: Poller | null = null
let heartbeatPoller: Poller | null = null

// Unwrap a single-object response that may be either the bare object or wrapped
// in `{ object: ... }` (the backend doc doesn't pin the key — be tolerant).
function extractObject(res: MapObjectResponse | MapObject): MapObject {
  if ('object' in res && res.object) return res.object
  return res as MapObject
}

export const useMapStore = defineStore('map', {
  state: (): MapState => ({
    objects: [],
    center: { ...DEFAULT_CENTER },
    selectedId: null,
    myPresenceObjectId: null,
    loading: false,
  }),
  getters: {
    selectedObject: (state): MapObject | null =>
      state.objects.find((o) => o.id === state.selectedId) ?? null,
    // "On a walk" is derived from holding presence (matches backend semantics).
    isVisiting: (state): boolean => state.myPresenceObjectId !== null,
  },
  actions: {
    setCenter(center: Center): void {
      this.center = center
    },

    select(id: string | null): void {
      this.selectedId = id
    },

    /** Load nearby objects around the current center. Also the refresh tick. */
    async refresh(): Promise<void> {
      this.loading = true
      try {
        const req: LoadMapRequest = {
          latitude: this.center.latitude,
          longitude: this.center.longitude,
        }
        const res = await api.post<LoadMapResponse>('/map/LoadMap', req)
        this.objects = res.objects ?? []
      } finally {
        this.loading = false
      }
    },

    /** Merge an updated single object back into the list (after a status change). */
    upsertObject(updated: MapObject): void {
      const idx = this.objects.findIndex((o) => o.id === updated.id)
      if (idx >= 0) this.objects[idx] = updated
      else this.objects.push(updated)
    },

    /**
     * Mark visiting / not-visiting a map object. Drives the presence heartbeat:
     * starting to visit begins the heartbeat; stopping ends it. The acting user
     * is the token owner — no user_id is sent (Docs/02-Backend.md).
     */
    async setVisiting(objectId: string, visiting: boolean): Promise<void> {
      const req: ChangeMapObjectStatusRequest = {
        id: objectId,
        action: visiting ? 'VISITING' : 'NOT_VISITING',
      }
      const res = await api.post<MapObjectResponse>(
        '/map/ChangeMapObjectStatus',
        req,
      )
      this.upsertObject(extractObject(res))

      if (visiting) {
        this.myPresenceObjectId = objectId
        this.startHeartbeat()
      } else {
        if (this.myPresenceObjectId === objectId) {
          this.myPresenceObjectId = null
        }
        this.stopHeartbeat()
      }
    },

    /**
     * Presence heartbeat: while visiting, re-send VISITING every ~2 min so the
     * backend's 15-min TTL never expires mid-walk (Docs/02-Backend.md +
     * Docs/03-Frontend.md). Swappable for WS/SSE later without touching views.
     */
    startHeartbeat(): void {
      if (!heartbeatPoller) {
        heartbeatPoller = createPoller(async () => {
          const id = this.myPresenceObjectId
          if (!id) {
            this.stopHeartbeat()
            return
          }
          const res = await api.post<MapObjectResponse>(
            '/map/ChangeMapObjectStatus',
            { id, action: 'VISITING' } satisfies ChangeMapObjectStatusRequest,
          )
          this.upsertObject(extractObject(res))
        }, PRESENCE_HEARTBEAT_MS)
      }
      heartbeatPoller.start(false)
    },

    stopHeartbeat(): void {
      heartbeatPoller?.stop()
    },

    /** Start map refresh polling while the Map page is active. */
    startPolling(): void {
      if (!refreshPoller) {
        refreshPoller = createPoller(() => this.refresh(), MAP_REFRESH_MS)
      }
      refreshPoller.start(true)
      // If we were already visiting (e.g. navigated back to Map), resume beats.
      if (this.myPresenceObjectId) this.startHeartbeat()
    },

    /**
     * Stop ALL polling (tab hidden / page unmount). Note: we intentionally do
     * NOT clear presence on the backend here — the 15-min TTL will lapse on its
     * own, and the user may just be switching tabs briefly.
     */
    stopPolling(): void {
      refreshPoller?.stop()
      heartbeatPoller?.stop()
    },
  },
})
