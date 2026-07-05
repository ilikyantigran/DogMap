<script setup lang="ts">
import { computed, reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useAuthStore } from '@/stores/authStore'
import { isNonEmpty } from '@/lib/validation'
import { toastError } from '@/lib/handleError'
import { ApiError } from '@/api/errors'
import { AUTH_EMAIL_NOT_VERIFIED } from '@/types/api'

// LoginForm: (email OR login) + password -> authStore.login.
const auth = useAuthStore()
const router = useRouter()
const route = useRoute()

const form = reactive({ identifier: '', password: '' })
const submitting = ref(false)
// When login fails because the email is unconfirmed, surface a distinct banner
// with a Resend affordance instead of a generic error toast.
const notVerified = ref(false)
const resending = ref(false)

// The backend accepts login OR email. We let the user type either; if it looks
// like an email we send it as `email`, otherwise as `login`.
const isEmailLike = computed(() => form.identifier.includes('@'))
const canSubmit = computed(
  () => isNonEmpty(form.identifier) && isNonEmpty(form.password),
)

async function onSubmit() {
  if (!canSubmit.value || submitting.value) return
  submitting.value = true
  notVerified.value = false
  try {
    await auth.login({
      email: isEmailLike.value ? form.identifier.trim() : undefined,
      login: isEmailLike.value ? undefined : form.identifier.trim(),
      password: form.password,
    })
    const redirect = (route.query.redirect as string) || '/map'
    router.push(redirect)
  } catch (err) {
    if (err instanceof ApiError && err.code === AUTH_EMAIL_NOT_VERIFIED) {
      notVerified.value = true
    } else {
      toastError(err)
    }
  } finally {
    submitting.value = false
  }
}

async function onResend() {
  // We can only resend when the identifier is an email address.
  if (!isEmailLike.value || resending.value) return
  resending.value = true
  try {
    await auth.resendVerification(form.identifier.trim())
  } catch (err) {
    toastError(err)
  } finally {
    resending.value = false
  }
}
</script>

<template>
  <form class="card" @submit.prevent="onSubmit">
    <h2>Log in</h2>

    <div v-if="notVerified" class="error" data-testid="not-verified">
      <p>Please confirm your email before logging in.</p>
      <button
        v-if="isEmailLike"
        class="primary"
        type="button"
        :disabled="resending"
        @click="onResend"
      >
        {{ resending ? 'Sending…' : 'Resend confirmation email' }}
      </button>
      <p v-else>
        Log in with the email you registered with to resend the confirmation
        link.
      </p>
    </div>

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
