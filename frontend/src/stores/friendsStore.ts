import { defineStore } from 'pinia'
import { api } from '@/api'
import { createPoller, type Poller } from '@/lib/poller'
import { FRIENDS_REFRESH_MS } from '@/config'
import type {
  FriendPresence,
  FriendsPresenceResponse,
  FriendSummary,
  IncomingRequest,
  ListFriendsResponse,
  OutgoingRequest,
  SendFriendRequestResponse,
  UserInfo,
} from '@/types/api'

// Friends graph: friends list (with on-walk indicator + where), incoming /
// outgoing requests, request actions, block / unfriend. Refresh polling lives
// here behind refresh()/startPolling() so components stay dumb
// (Docs/03-Frontend.md "Profile page active -> friendsStore.refresh()").

interface FriendsState {
  friends: FriendSummary[]
  incoming: IncomingRequest[]
  outgoing: OutgoingRequest[]
  loading: boolean
  // Result of the "find friend by login" lookup (reduced profile for a stranger).
  lookupResult: UserInfo | null
  // Which friends are currently on a walk + where (Map FriendsPresence).
  presence: FriendPresence[]
}

// Poller lives outside reactive state (so Pinia doesn't proxy it) and is keyed by
// store instance so a fresh Pinia in tests never reuses a stale poller closure.
const pollerByStore = new WeakMap<object, Poller>()

export const useFriendsStore = defineStore('friends', {
  state: (): FriendsState => ({
    friends: [],
    incoming: [],
    outgoing: [],
    loading: false,
    lookupResult: null,
    presence: [],
  }),
  getters: {
    // user_id -> live presence, for O(1) lookup per friend row.
    presenceByUser: (state): Record<string, FriendPresence> => {
      const m: Record<string, FriendPresence> = {}
      for (const p of state.presence) m[p.user_id] = p
      return m
    },
  },
  actions: {
    /** One refresh of the friend graph. Also the poll tick. */
    async refresh(): Promise<void> {
      this.loading = true
      try {
        const res = await api.post<ListFriendsResponse>('/v1/friends/list')
        this.friends = res.friends ?? []
        this.incoming = res.incoming_requests ?? []
        this.outgoing = res.outgoing_requests ?? []
        // Live on-walk status for friends. Friends-only by construction — the Map
        // endpoint only returns the caller's friends. Best-effort so a transient
        // presence failure never blanks the friends list.
        await this.fetchPresence().catch(() => {})
      } finally {
        this.loading = false
      }
    },

    /** Which friends are currently on a walk and where (Map FriendsPresence). */
    async fetchPresence(): Promise<void> {
      const res = await api.post<FriendsPresenceResponse>(
        '/v1/map/friends-presence',
        {},
      )
      this.presence = res.friends ?? []
    },

    /** Start polling the friend graph while the Profile page is active. */
    startPolling(): void {
      let poller = pollerByStore.get(this)
      if (!poller) {
        poller = createPoller(() => this.refresh(), FRIENDS_REFRESH_MS)
        pollerByStore.set(this, poller)
      }
      poller.start(true)
    },

    /** Stop polling (page unmount / tab hidden). */
    stopPolling(): void {
      pollerByStore.get(this)?.stop()
    },

    /** Find a user by login -> reduced profile the caller can request. */
    async findByLogin(login: string): Promise<UserInfo | null> {
      const res = await api.post<UserInfo | null>('/v1/profiles/find-by-login', {
        login,
      })
      this.lookupResult = res ?? null
      return this.lookupResult
    },

    clearLookup(): void {
      this.lookupResult = null
    },

    async sendRequest(userIdTarget: string): Promise<void> {
      await api.post<SendFriendRequestResponse>('/v1/friends/request', {
        user_id_target: userIdTarget,
      })
      await this.refresh()
    },

    /** Approve (true) or decline (false) an incoming request. */
    async respondToRequest(
      friendRequestId: string,
      resolution: boolean,
    ): Promise<void> {
      await api.post('/v1/friends/respond', {
        friend_request_id: friendRequestId,
        resolution,
      })
      await this.refresh()
    },

    async removeFriend(userIdTarget: string): Promise<void> {
      await api.post('/v1/friends/remove', { user_id_target: userIdTarget })
      await this.refresh()
    },

    async block(userIdTarget: string): Promise<void> {
      await api.post('/v1/friends/block', { user_id_target: userIdTarget })
      await this.refresh()
    },

    async unblock(userIdTarget: string): Promise<void> {
      await api.post('/v1/friends/unblock', { user_id_target: userIdTarget })
      await this.refresh()
    },
  },
})
