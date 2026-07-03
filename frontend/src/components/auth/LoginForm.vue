<script setup lang="ts">
import { computed, reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useAuthStore } from '@/stores/authStore'
import { isNonEmpty } from '@/lib/validation'
import { toastError } from '@/lib/handleError'

// LoginForm: (email OR login) + password -> authStore.login.
const auth = useAuthStore()
const router = useRouter()
const route = useRoute()

const form = reactive({ identifier: '', password: '' })
const submitting = ref(false)

// The backend accepts login OR email. We let the user type either; if it looks
// like an email we send it as `email`, otherwise as `login`.
const isEmailLike = computed(() => form.identifier.includes('@'))
const canSubmit = computed(
  () => isNonEmpty(form.identifier) && isNonEmpty(form.password),
)

async function onSubmit() {
  if (!canSubmit.value || submitting.value) return
  submitting.value = true
  try {
    await auth.login({
      email: isEmailLike.value ? form.identifier.trim() : undefined,
      login: isEmailLike.value ? undefined : form.identifier.trim(),
      password: form.password,
    })
    const redirect = (route.query.redirect as string) || '/map'
    router.push(redirect)
  } catch (err) {
    toastError(err)
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <form class="card" @submit.prevent="onSubmit">
    <h2>Log in</h2>
    <div class="field">
      <label for="login-id">Email or login</label>
      <input id="login-id" v-model="form.identifier" autocomplete="username" />
    </div>
    <div class="field">
      <label for="login-pw">Password</label>
      <input
        id="login-pw"
        v-model="form.password"
        type="password"
        autocomplete="current-password"
      />
    </div>
    <button class="primary" type="submit" :disabled="!canSubmit || submitting">
      {{ submitting ? 'Logging in…' : 'Log in' }}
    </button>
    <p>
      No account?
      <RouterLink to="/register">Register</RouterLink>
    </p>
  </form>
</template>
