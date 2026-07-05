<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { useAuthStore } from '@/stores/authStore'
import { ApiError } from '@/api/errors'

// Public /verify page. Reads ?token= from the emailed link and confirms the
// email via POST /v1/auth/verify, then shows success or failure.
const auth = useAuthStore()
const route = useRoute()

type Status = 'pending' | 'success' | 'error'
const status = ref<Status>('pending')
const errorMessage = ref('')

onMounted(async () => {
  const token = (route.query.token as string | undefined)?.trim()
  if (!token) {
    status.value = 'error'
    errorMessage.value = 'This confirmation link is missing its token.'
    return
  }
  try {
    await auth.verifyEmail(token)
    status.value = 'success'
  } catch (err) {
    status.value = 'error'
    errorMessage.value =
      err instanceof ApiError
        ? err.message
        : 'We could not confirm your email. The link may have expired.'
  }
})
</script>

<template>
  <section style="max-width: 420px; margin: 2rem auto">
    <h1 style="text-align: center">DogMap</h1>
    <div class="card">
      <div v-if="status === 'pending'" data-testid="verify-pending">
        <h2>Confirming your email…</h2>
        <p>One moment.</p>
      </div>

      <div v-else-if="status === 'success'" data-testid="verify-success">
        <h2>Email confirmed</h2>
        <p>Your account is active. You can now log in.</p>
        <RouterLink to="/login">
          <button class="primary" type="button">Go to login</button>
        </RouterLink>
      </div>

      <div v-else data-testid="verify-error">
        <h2>Confirmation failed</h2>
        <p class="error">{{ errorMessage }}</p>
        <p>
          Need a new link? Try logging in with your email to resend the
          confirmation, or <RouterLink to="/register">register</RouterLink>.
        </p>
      </div>
    </div>
  </section>
</template>
