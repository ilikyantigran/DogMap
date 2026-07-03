import { beforeEach, afterEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'

vi.mock('@/api', () => ({
  api: { post: vi.fn(), get: vi.fn() },
  configureApi: vi.fn(),
  apiBaseUrl: '/api',
}))

import { api } from '@/api'
import { useMapStore } from '@/stores/mapStore'
import { PRESENCE_HEARTBEAT_MS } from '@/config'
import type { MapObject } from '@/types/api'

const post = api.post as unknown as ReturnType<typeof vi.fn>

function obj(over: Partial<MapObject> = {}): MapObject {
  return {
    id: 'park-1',
    object_type: 'PARK',
    longitude: -0.1,
    latitude: 51.5,
    visitor_count: 0,
    friend_ids_here: [],
    ...over,
  }
}

describe('mapStore', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    post.mockReset()
    vi.useFakeTimers()
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('refresh loads nearby objects from LoadMap around a center', async () => {
    post.mockResolvedValue({ code: 0, message: 'ok', objects: [obj()] })
    const map = useMapStore()
    map.center = { latitude: 51.5, longitude: -0.1 }

    await map.refresh()

    expect(post).toHaveBeenCalledWith('/map/LoadMap', {
      latitude: 51.5,
      longitude: -0.1,
    })
    expect(map.objects).toHaveLength(1)
  })

  it('Path 1: mark visiting -> object shows 1 person, presence recorded', async () => {
    const map = useMapStore()
    map.objects = [obj({ visitor_count: 0 })]
    map.selectedId = 'park-1'

    post.mockResolvedValue({
      code: 0,
      message: 'ok',
      object: obj({ visitor_count: 1 }),
    })

    await map.setVisiting('park-1', true)

    expect(post).toHaveBeenCalledWith('/map/ChangeMapObjectStatus', {
      id: 'park-1',
      action: 'VISITING',
    })
    expect(map.myPresenceObjectId).toBe('park-1')
    expect(map.objects[0].visitor_count).toBe(1)
  })

  it('Path 1: mark not going -> back to 0, presence cleared', async () => {
    const map = useMapStore()
    map.objects = [obj({ visitor_count: 1 })]
    map.myPresenceObjectId = 'park-1'

    post.mockResolvedValue({
      code: 0,
      message: 'ok',
      object: obj({ visitor_count: 0 }),
    })

    await map.setVisiting('park-1', false)

    expect(post).toHaveBeenCalledWith('/map/ChangeMapObjectStatus', {
      id: 'park-1',
      action: 'NOT_VISITING',
    })
    expect(map.myPresenceObjectId).toBeNull()
    expect(map.objects[0].visitor_count).toBe(0)
  })

  it('starts the presence heartbeat when visiting and stops it when not', async () => {
    const map = useMapStore()
    map.objects = [obj()]

    post.mockResolvedValue({ code: 0, message: 'ok', object: obj({ visitor_count: 1 }) })
    await map.setVisiting('park-1', true)
    post.mockClear()

    // Heartbeat re-sends VISITING before the 15-min TTL expires.
    post.mockResolvedValue({ code: 0, message: 'ok', object: obj({ visitor_count: 1 }) })
    await vi.advanceTimersByTimeAsync(PRESENCE_HEARTBEAT_MS + 5)

    expect(post).toHaveBeenCalledWith('/map/ChangeMapObjectStatus', {
      id: 'park-1',
      action: 'VISITING',
    })

    // Stop visiting -> heartbeat stops.
    post.mockResolvedValue({ code: 0, message: 'ok', object: obj({ visitor_count: 0 }) })
    await map.setVisiting('park-1', false)
    post.mockClear()

    await vi.advanceTimersByTimeAsync(PRESENCE_HEARTBEAT_MS * 2)
    // Only the heartbeat would have called post; there should be none now.
    const heartbeatCalls = post.mock.calls.filter(
      ([, body]) =>
        (body as { action?: string })?.action === 'VISITING',
    )
    expect(heartbeatCalls).toHaveLength(0)
  })

  it('stopPolling stops both refresh and heartbeat timers', async () => {
    const map = useMapStore()
    map.objects = [obj()]
    post.mockResolvedValue({ code: 0, message: 'ok', objects: [obj()] })

    map.startPolling()
    post.mockClear()
    map.stopPolling()

    await vi.advanceTimersByTimeAsync(60_000)
    expect(post).not.toHaveBeenCalled()
  })
})
