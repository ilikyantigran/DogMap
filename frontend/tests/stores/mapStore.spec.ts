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
    viewer_visiting: false,
    ...over,
  }
}

// setVisiting makes TWO calls: POST /v1/map/status (the mark) then POST
// /v1/map/load (the follow-up refresh). Route the mock by URL so both resolve,
// and so refresh() derives the caller's presence from viewer_visiting.
function routeMock(statusObj: MapObject, loadObjs: MapObject[]) {
  post.mockImplementation((url: string) => {
    if (url === '/v1/map/status') {
      return Promise.resolve({ code: 0, message: 'ok', object: statusObj })
    }
    if (url === '/v1/map/load') {
      return Promise.resolve({ code: 0, message: 'ok', objects: loadObjs })
    }
    return Promise.resolve({ code: 0, message: 'ok' })
  })
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

    expect(post).toHaveBeenCalledWith('/v1/map/load', {
      latitude: 51.5,
      longitude: -0.1,
    })
    expect(map.objects).toHaveLength(1)
  })

  it('refresh restores the caller presence from viewer_visiting', async () => {
    post.mockResolvedValue({
      code: 0,
      message: 'ok',
      objects: [obj({ viewer_visiting: true })],
    })
    const map = useMapStore()

    await map.refresh()

    // Survives a page refresh: presence is re-derived from the backend flag.
    expect(map.myPresenceObjectId).toBe('park-1')
  })

  it('Path 1: mark visiting -> object shows 1 person, presence recorded', async () => {
    const map = useMapStore()
    map.objects = [obj({ visitor_count: 0 })]
    map.selectedId = 'park-1'
    routeMock(obj({ visitor_count: 1, viewer_visiting: true }), [
      obj({ visitor_count: 1, viewer_visiting: true }),
    ])

    await map.setVisiting('park-1', true)

    expect(post).toHaveBeenCalledWith('/v1/map/status', {
      id: 'park-1',
      action: 'VISITING',
    })
    // The mark is followed by a map refresh.
    expect(post).toHaveBeenCalledWith('/v1/map/load', expect.anything())
    expect(map.myPresenceObjectId).toBe('park-1')
    expect(map.objects[0].visitor_count).toBe(1)
  })

  it('Path 1: mark not going -> back to 0, presence cleared', async () => {
    const map = useMapStore()
    map.objects = [obj({ visitor_count: 1, viewer_visiting: true })]
    map.myPresenceObjectId = 'park-1'
    routeMock(obj({ visitor_count: 0 }), [obj({ visitor_count: 0 })])

    await map.setVisiting('park-1', false)

    expect(post).toHaveBeenCalledWith('/v1/map/status', {
      id: 'park-1',
      action: 'NOT_VISITING',
    })
    expect(map.myPresenceObjectId).toBeNull()
    expect(map.objects[0].visitor_count).toBe(0)
  })

  it('starts the presence heartbeat when visiting and stops it when not', async () => {
    const map = useMapStore()
    map.objects = [obj()]

    routeMock(obj({ visitor_count: 1, viewer_visiting: true }), [
      obj({ visitor_count: 1, viewer_visiting: true }),
    ])
    await map.setVisiting('park-1', true)
    post.mockClear()

    // Heartbeat re-sends VISITING before the 15-min TTL expires.
    routeMock(obj({ visitor_count: 1, viewer_visiting: true }), [
      obj({ visitor_count: 1, viewer_visiting: true }),
    ])
    await vi.advanceTimersByTimeAsync(PRESENCE_HEARTBEAT_MS + 5)

    expect(post).toHaveBeenCalledWith('/v1/map/status', {
      id: 'park-1',
      action: 'VISITING',
    })

    // Stop visiting -> heartbeat stops.
    routeMock(obj({ visitor_count: 0 }), [obj({ visitor_count: 0 })])
    await map.setVisiting('park-1', false)
    post.mockClear()

    await vi.advanceTimersByTimeAsync(PRESENCE_HEARTBEAT_MS * 2)
    // Only the heartbeat would have re-sent VISITING; there should be none now.
    const heartbeatCalls = post.mock.calls.filter(
      ([url, body]) =>
        url === '/v1/map/status' &&
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
