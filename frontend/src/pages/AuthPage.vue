<script setup lang="ts">
import LoginForm from '@/components/auth/LoginForm.vue'
import RegisterForm from '@/components/auth/RegisterForm.vue'

// Public Auth page. `mode` is supplied by the route (/login vs /register).
defineProps<{ mode: 'login' | 'register' }>()
</script>

<template>
  <section style="max-width: 420px; margin: 2rem auto">
    <h1 style="text-align: center">DogMap</h1>
    <LoginForm v-if="mode === 'login'" />
    <RegisterForm v-else />
  </section>
</template>
