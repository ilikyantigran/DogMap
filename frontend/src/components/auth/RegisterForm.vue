<script setup lang="ts">
import { computed, reactive, ref } from 'vue'
import { useAuthStore } from '@/stores/authStore'
import { isNonEmpty, isValidEmail, passwordsMatch } from '@/lib/validation'
import { toastError } from '@/lib/handleError'

// RegisterForm: login, email, password, confirm -> authStore.register. With
// email confirmation there is NO auto-login: on success we show a "check your
// email" panel and let the user confirm via the emailed link.
const auth = useAuthStore()

const form = reactive({
  login: '',
  email: '',
  password: '',
  confirm: '',
})
const submitting = ref(false)
// Set once registration succeeds; flips the template to the "check email" state.
const registeredEmail = ref<string | null>(null)
const resending = ref(false)

const emailValid = computed(() => isValidEmail(form.email))
const confirmValid = computed(() => passwordsMatch(form.password, form.confirm))
const canSubmit = computed(
  () =>
    isNonEmpty(form.login) &&
    emailValid.value &&
    isNonEmpty(form.password) &&
    confirmValid.value,
)

async function onSubmit() {
  if (!canSubmit.value || submitting.value) return
  submitting.value = true
  try {
    const email = form.email.trim()
    await auth.register({
      login: form.login.trim(),
      email,
      password: form.password,
    })
    // No auto-login: switch to the confirmation-pending state.
    registeredEmail.value = email
  } catch (err) {
    toastError(err)
  } finally {
    submitting.value = false
  }
}

async function onResend() {
  if (!registeredEmail.value || resending.value) return
  resending.value = true
  try {
    await auth.resendVerification(registeredEmail.value)
  } catch (err) {
    toastError(err)
  } finally {
    resending.value = false
  }
}
</script>

<template>
  <!-- Success state: account created, waiting on email confirmation. -->
  <div v-if="registeredEmail" class="card" data-testid="check-email">
    <h2>Check your email</h2>
    <p>
      We sent a confirmation link to <strong>{{ registeredEmail }}</strong>.
      Open it to activate your account, then log in.
    </p>
    <button
      class="primary"
      type="button"
      :disabled="resending"
      @click="onResend"
    >
      {{ resending ? 'Sending…' : 'Resend confirmation email' }}
    </button>
    <p>
      Already confirmed?
      <RouterLink to="/login">Log in</RouterLink>
    </p>
  </div>

  <!-- Default state: the registration form. -->
  <form v-else class="card" @submit.prevent="onSubmit">
    <h2>Register</h2>
    <div class="field">
      <label for="reg-login">Login (permanent)</label>
      <input id="reg-login" v-model="form.login" autocomplete="username" />
    </div>
    <div class="field">
      <label for="reg-email">Email</label>
      <input id="reg-email" v-model="form.email" type="email" />
      <div v-if="form.email && !emailValid" class="error">
        Enter a valid email address.
      </div>
    </div>
    <div class="field">
      <label for="reg-pw">Password</label>
      <input
        id="reg-pw"
        v-model="form.password"
        type="password"
        autocomplete="new-password"
      />
    </div>
    <div class="field">
      <label for="reg-confirm">Confirm password</label>
      <input
        id="reg-confirm"
        v-model="form.confirm"
        type="password"
        autocomplete="new-password"
      />
      <div v-if="form.confirm && !confirmValid" class="error">
        Passwords do not match.
      </div>
    </div>
    <button class="primary" type="submit" :disabled="!canSubmit || submitting">
      {{ submitting ? 'Creating…' : 'Create account' }}
    </button>
    <p>
      Already have an account?
      <RouterLink to="/login">Log in</RouterLink>
    </p>
  </form>
</template>
