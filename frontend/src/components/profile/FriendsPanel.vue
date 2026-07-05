<script setup lang="ts">
import { ref } from 'vue'
import { storeToRefs } from 'pinia'
import { useFriendsStore } from '@/stores/friendsStore'
import { toastError } from '@/lib/handleError'
import { useToastStore } from '@/stores/toastStore'

// FriendsPanel: friends list (on-walk indicator + where), incoming/outgoing
// requests, find-friend-by-login (reduced profile) -> send request, remove/block.
// All data + mutations go through friendsStore; this component stays dumb.
const friends = useFriendsStore()
const toast = useToastStore()
const { friends: friendList, incoming, outgoing, lookupResult, presenceByUser } =
  storeToRefs(friends)

const searchLogin = ref('')
const searching = ref(false)

async function onSearch() {
  if (!searchLogin.value.trim() || searching.value) return
  searching.value = true
  try {
    const result = await friends.findByLogin(searchLogin.value.trim())
    if (!result) toast.info('No user found with that login')
  } catch (err) {
    toastError(err)
  } finally {
    searching.value = false
  }
}

async function run(fn: () => Promise<void>, ok?: string) {
  try {
    await fn()
    if (ok) toast.success(ok)
  } catch (err) {
    toastError(err)
  }
}
</script>

<template>
  <!-- Pending requests are shown ABOVE the friends list. -->
  <!-- Incoming requests: approve / decline -->
  <div class="card" v-if="incoming.length">
    <h4 style="margin-top: 0">Incoming requests</h4>
    <ul style="list-style: none; padding: 0">
      <li
        v-for="r in incoming"
        :key="r.friend_request_id"
        style="display: flex; align-items: center; gap: 0.5rem; padding: 0.4rem 0"
      >
        <span>{{ r.from_login }}</span>
        <span style="flex: 1" />
        <button
          type="button"
          class="primary"
          @click="run(() => friends.respondToRequest(r.friend_request_id, true), 'Accepted')"
        >
          Approve
        </button>
        <button
          type="button"
          @click="run(() => friends.respondToRequest(r.friend_request_id, false))"
        >
          Decline
        </button>
      </li>
    </ul>
  </div>

  <!-- Outgoing requests -->
  <div class="card" v-if="outgoing.length">
    <h4 style="margin-top: 0">Sent requests</h4>
    <ul style="list-style: none; padding: 0">
      <li v-for="r in outgoing" :key="r.friend_request_id">
        Request to {{ r.to_login }} — pending
      </li>
    </ul>
  </div>

  <!-- Friends list with on-walk indicator + where -->
  <div class="card">
    <h3 style="margin-top: 0">Friends</h3>

    <p v-if="friendList.length === 0" style="color: var(--dm-muted)">
      No friends yet.
    </p>
    <ul style="list-style: none; padding: 0">
      <li
        v-for="f in friendList"
        :key="f.user_id"
        style="display: flex; align-items: center; gap: 0.5rem; padding: 0.4rem 0"
      >
        <span>{{ f.login }}</span>
        <span
          v-if="presenceByUser[f.user_id]"
          title="On a walk"
          style="color: var(--dm-primary)"
        >
          🐾 on a walk at
          <RouterLink
            :to="{ name: 'map', query: { object: presenceByUser[f.user_id].object_id } }"
          >
            {{ presenceByUser[f.user_id].object_name }}
          </RouterLink>
        </span>
        <span v-else style="color: var(--dm-muted)">offline</span>
        <span style="flex: 1" />
        <button type="button" @click="run(() => friends.removeFriend(f.user_id), 'Removed')">
          Remove
        </button>
        <button type="button" class="danger" @click="run(() => friends.block(f.user_id), 'Blocked')">
          Block
        </button>
      </li>
    </ul>
  </div>

  <!-- Find friend by login -> reduced profile -> send request -->
  <div class="card">
    <h4 style="margin-top: 0">Find a friend</h4>
    <div style="display: flex; gap: 0.5rem">
      <input v-model="searchLogin" placeholder="Their login" @keyup.enter="onSearch" />
      <button type="button" @click="onSearch" :disabled="searching">Search</button>
    </div>

    <div v-if="lookupResult" style="margin-top: 0.75rem">
      <strong>{{ lookupResult.login }}</strong>
      <span v-if="lookupResult.name"> — {{ lookupResult.name }}</span>
      <!-- Reduced profile: no email/phone for a stranger (privacy). -->
      <div v-if="lookupResult.pets?.length" style="color: var(--dm-muted); font-size: 0.9rem">
        {{ lookupResult.pets.length }} pet(s)
      </div>
      <div style="margin-top: 0.5rem">
        <button
          v-if="lookupResult.friend_status === 'NONE'"
          type="button"
          class="primary"
          @click="run(() => friends.sendRequest(lookupResult!.user_id), 'Request sent')"
        >
          Send friend request
        </button>
        <span v-else style="color: var(--dm-muted)">
          Status: {{ lookupResult.friend_status }}
        </span>
      </div>
    </div>
  </div>
</template>
