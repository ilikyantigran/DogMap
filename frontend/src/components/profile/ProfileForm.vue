<script setup lang="ts">
import { computed, ref } from 'vue'
import { storeToRefs } from 'pinia'
import { useProfileStore } from '@/stores/profileStore'
import { isValidPhone } from '@/lib/validation'
import { toastError } from '@/lib/handleError'
import { useToastStore } from '@/stores/toastStore'
import PetEditor from './PetEditor.vue'

// ProfileForm: name, surname, phone (E.164), read-only login/email. Pets editor
// nested. Explicit Save (product spec) -> profileStore.save (EditUser). Acting
// user is the token owner; login/email are not editable here.
const profileStore = useProfileStore()
const toast = useToastStore()
const { profile, saving } = storeToRefs(profileStore)

const phoneValid = computed(
  () => !profile.value?.phone || isValidPhone(profile.value.phone),
)
const submitting = ref(false)

async function onSave() {
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
    toast.success('Profile saved')
  } catch (err) {
    toastError(err)
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <form v-if="profile" @submit.prevent="onSave">
    <div class="card">
      <h3 style="margin-top: 0">Your profile</h3>
      <div class="field">
        <label>Login</label>
        <input :value="profile.login" readonly disabled />
      </div>
      <div class="field">
        <label>Email</label>
        <input :value="profile.email" readonly disabled />
      </div>
      <div class="field">
        <label>Name</label>
        <input v-model="profile.name" />
      </div>
      <div class="field">
        <label>Surname</label>
        <input v-model="profile.surname" />
      </div>
      <div class="field">
        <label>Phone (E.164)</label>
        <input v-model="profile.phone" placeholder="+14155552671" />
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

    <button class="primary" type="submit" :disabled="saving || submitting">
      {{ saving || submitting ? 'Saving…' : 'Save' }}
    </button>
  </form>
  <p v-else style="color: var(--dm-muted)">Loading profile…</p>
</template>
