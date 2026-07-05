<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { storeToRefs } from 'pinia'
import { useRouter } from 'vue-router'
import { useProfileStore } from '@/stores/profileStore'
import { useAuthStore } from '@/stores/authStore'
import { useToastStore } from '@/stores/toastStore'
import { isValidPhone } from '@/lib/validation'
import { toastError } from '@/lib/handleError'
import { markSetupDone } from '@/lib/profileSetup'
import PetEditor from '@/components/profile/PetEditor.vue'

// Two-step registration, step 2: a one-time ProfileSetup shown after a user's
// first confirmed login when their profile is still empty. Lets them fill
// name/surname/phone/pets, or Skip (they can do it later on Profile). Either
// action persists the "setup handled" flag so it never nags again. Reuses the
// existing EditUser store action (profileStore.save) and the PetEditor component.
const profileStore = useProfileStore()
const auth = useAuthStore()
const toast = useToastStore()
const router = useRouter()
const { profile, saving } = storeToRefs(profileStore)

const submitting = ref(false)
const phoneValid = computed(
  () => !profile.value?.phone || isValidPhone(profile.value.phone),
)

onMounted(async () => {
  // The guard usually loads the profile before routing here, but load defensively
  // so the page also works on a direct visit / refresh.
  if (!profile.value) {
    try {
      await profileStore.loadSelf()
    } catch (err) {
      toastError(err)
    }
  }
})

function markHandledAndGo(): void {
  if (auth.userId) markSetupDone(auth.userId)
  router.push('/map')
}

async function onComplete() {
  if (!profile.value || submitting.value) return
  if (!phoneValid.value) {
    toast.error('Phone must be in E.164 format, e.g. +14155552671')
    return
  }
  submitting.value = true
  try {
    await profileStore.save({
      name: profile.value.name,
      surname: profile.value.surname,
      phone: profile.value.phone ?? '',
      pets: profile.value.pets,
    })
    markHandledAndGo()
    toast.success('Profile saved')
  } catch (err) {
    toastError(err)
  } finally {
    submitting.value = false
  }
}

function onSkip() {
  // Skip does NOT save; it only records that setup was handled so later logins
  // don't nag. The user can complete their profile anytime on the Profile page.
  markHandledAndGo()
  toast.success('You can fill this in anytime on your Profile page.')
}
</script>

<template>
  <div class="setup-wrap">
    <div class="card setup-intro">
      <h1 style="margin-top: 0">Welcome to DogMap</h1>
      <p style="color: var(--dm-muted)">
        Set up your profile so friends know who you walk with. You can
        <strong>skip</strong> this and fill it in later on your Profile page.
      </p>
    </div>

    <form v-if="profile" @submit.prevent="onComplete">
      <div class="card">
        <h3 style="margin-top: 0">About you</h3>
        <div class="field">
          <label>Name</label>
          <input data-test="setup-name" v-model="profile.name" />
        </div>
        <div class="field">
          <label>Surname</label>
          <input data-test="setup-surname" v-model="profile.surname" />
        </div>
        <div class="field">
          <label>Phone (E.164)</label>
          <input
            data-test="setup-phone"
            v-model="profile.phone"
            placeholder="+14155552671"
          />
          <div v-if="!phoneValid" class="error">
            Use E.164 format, e.g. +14155552671
          </div>
        </div>
      </div>

      <PetEditor
        :pets="profile.pets"
        @add="profileStore.addPet()"
        @remove="(i: number) => profileStore.removePet(i)"
      />

      <div class="setup-actions">
        <button
          class="primary"
          type="submit"
          data-test="setup-complete"
          :disabled="saving || submitting"
        >
          {{ saving || submitting ? 'Saving…' : 'Save and continue' }}
        </button>
        <button
          type="button"
          data-test="setup-skip"
          :disabled="submitting"
          @click="onSkip"
        >
          Skip for now
        </button>
      </div>
    </form>
    <p v-else style="color: var(--dm-muted)">Loading…</p>
  </div>
</template>

<style scoped>
.setup-wrap {
  max-width: 640px;
  margin: 0 auto;
}
.setup-actions {
  display: flex;
  gap: 0.75rem;
  align-items: center;
  flex-wrap: wrap;
}
</style>
