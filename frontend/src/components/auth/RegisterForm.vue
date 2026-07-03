<script setup lang="ts">
import { computed, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { useAuthStore } from '@/stores/authStore'
import { isNonEmpty, isValidEmail, passwordsMatch } from '@/lib/validation'
import { toastError } from '@/lib/handleError'

// RegisterForm: login, email, password, confirm -> authStore.register (which
// registers then auto-logins). Client-side validation: email format + confirm.
const auth = useAuthStore()
const router = useRouter()

const form = reactive({
  login: '',
  email: '',
  password: '',
  confirm: '',
})
const submitting = ref(false)

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
    await auth.register({
      login: form.login.trim(),
      email: form.email.trim(),
      password: form.password,
    })
    // Auto-login happened inside register(); go set up the profile next.
    router.push('/profile')
  } catch (err) {
    toastError(err)
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <form class="card" @submit.prevent="onSubmit">
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
