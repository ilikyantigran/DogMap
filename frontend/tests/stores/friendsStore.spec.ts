import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'

vi.mock('@/api', () => ({
  api: { post: vi.fn(), get: vi.fn() },
  configureApi: vi.fn(),
  apiBaseUrl: '/api',
}))

import { api } from '@/api'
import { useFriendsStore } from '@/stores/friendsStore'

const post = api.post as unknown as ReturnType<typeof vi.fn>

function emptyList() {
  return {
    code: 0,
    message: 'ok',
    friends: [],
    incoming_requests: [],
    outgoing_requests: [],
  }
}

describe('friendsStore', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    post.mockReset()
  })

  it('refresh loads friends + incoming + outgoing', async () => {
    post.mockResolvedValue({
      code: 0,
      message: 'ok',
      friends: [{ user_id: 'u1', login: 'Test1', on_walk: true, current_object_id: 'p1' }],
      incoming_requests: [
        { from_user_id: 'u2', from_login: 'Test2', friend_request_id: 'r1' },
      ],
      outgoing_requests: [],
    })
    const friends = useFriendsStore()

    await friends.refresh()

    expect(post).toHaveBeenCalledWith('/profiles/ListFriends')
    expect(friends.friends[0].login).toBe('Test1')
    expect(friends.friends[0].on_walk).toBe(true)
    expect(friends.incoming[0].from_login).toBe('Test2')
  })

  it('Path 2: find friend by login returns the reduced profile', async () => {
    // Reduced profile: no email / phone for a stranger.
    post.mockResolvedValue({
      user_id: 'u1',
      login: 'Test1',
      name: 'Test',
      surname: '',
      pets: [{ breed: 'Poodle', name: 'Bruno', sex: 'M', is_castrated: true, age: 3 }],
      on_walk: false,
      friend_status: 'NONE',
    })
    const friends = useFriendsStore()

    const result = await friends.findByLogin('Test1')

    expect(post).toHaveBeenCalledWith('/profiles/FindUserByLogin', {
      login: 'Test1',
    })
    expect(result?.login).toBe('Test1')
    expect(result?.email).toBeUndefined()
    expect(result?.friend_status).toBe('NONE')
    expect(friends.lookupResult?.login).toBe('Test1')
  })

  it('Path 2: send request then refreshes the graph', async () => {
    const friends = useFriendsStore()
    post
      .mockResolvedValueOnce({ code: 0, message: 'ok', friend_request_id: 'r9' })
      .mockResolvedValueOnce(emptyList())

    await friends.sendRequest('u1')

    expect(post).toHaveBeenNthCalledWith(1, '/profiles/SendFriendRequest', {
      user_id_target: 'u1',
    })
    expect(post).toHaveBeenNthCalledWith(2, '/profiles/ListFriends')
  })

  it('respondToRequest accepts (resolution true) then refreshes', async () => {
    const friends = useFriendsStore()
    post
      .mockResolvedValueOnce({ code: 0, message: 'ok' })
      .mockResolvedValueOnce(emptyList())

    await friends.respondToRequest('r1', true)

    expect(post).toHaveBeenNthCalledWith(1, '/profiles/SendFriendResponse', {
      friend_request_id: 'r1',
      resolution: true,
    })
  })

  it('block sends the target id then refreshes', async () => {
    const friends = useFriendsStore()
    post
      .mockResolvedValueOnce({ code: 0, message: 'ok' })
      .mockResolvedValueOnce(emptyList())

    await friends.block('u2')

    expect(post).toHaveBeenNthCalledWith(1, '/profiles/BlockUser', {
      user_id_target: 'u2',
    })
  })
})
