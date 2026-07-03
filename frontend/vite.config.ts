/// <reference types="vitest/config" />
import { fileURLToPath, URL } from 'node:url'
import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

// https://vite.dev/config/
export default defineConfig({
  plugins: [vue()],
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url)),
    },
  },
  server: {
    port: 5173,
    // No single aggregating gateway yet: each service exposes its own HTTP edge
    // on a distinct port (Docs/02-Backend.md). Route each /v1 prefix to the right
    // service's http_port. Paths are served as-is (no rewrite).
    //   auth      -> :8081   profiles+friends -> :8083   map -> :8085
    proxy: {
      '/v1/auth': { target: 'http://localhost:8081', changeOrigin: true },
      '/v1/profiles': { target: 'http://localhost:8083', changeOrigin: true },
      '/v1/friends': { target: 'http://localhost:8083', changeOrigin: true },
      '/v1/map': { target: 'http://localhost:8085', changeOrigin: true },
    },
  },
  test: {
    globals: true,
    environment: 'happy-dom',
    setupFiles: ['./tests/setup.ts'],
    include: ['tests/**/*.spec.ts'],
  },
})
