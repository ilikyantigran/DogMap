# DogMap Frontend — Build Progress

Terse Done / Next / Notes log. On resume: read this first, continue from the first
unfinished item in **Next**.

## Done

- Project scaffold: `package.json`, `tsconfig*.json`, `vite.config.ts` (+ Vitest
  config, `/api` dev proxy), `index.html`, `.gitignore`, `.env.example`, `env.d.ts`.
- API contract types mirroring `Docs/02-Backend.md` — `src/types/api.ts`.
- `apiClient` (`src/api/apiClient.ts`) — injects `auth_token` header, centralizes
  `code`/`message` envelope errors + 401 handling via injected callbacks.
  Wired singleton `src/api/index.ts`; `ApiError` in `src/api/errors.ts`.
- Pinia stores: `authStore` (in-memory token + localStorage TODO), `profileStore`,
  `friendsStore` (polling behind actions), `mapStore` (map refresh + presence
  heartbeat behind actions), `toastStore`.
- Polling abstraction `src/lib/poller.ts`; intervals/TTL constants `src/config.ts`.
- Router + guards `src/router/index.ts` (Auth public; Profile/Map guarded).
- `main.ts` wiring (apiClient <-> authStore, 401 redirect), `App.vue` shell + nav.
- Components: ToastHost; auth LoginForm/RegisterForm + AuthPage; profile
  ProfileForm/PetEditor/FriendsPanel + ProfilePage; map MapLegend/MapObjectPopup.
- Helpers: validation, geolocation, mapObjects meta, handleError.
- Tests (first, where practical): apiClient, authStore, mapStore (incl. heartbeat),
  router guards, validation.

## Feature: email confirmation on registration (branch release/auth-email-confirm)
_2026-07-05._ Frontend slice; `npm test` (32) + `npm run build` green.
- `authStore.register` NO LONGER auto-logins — it records
  `pendingVerificationEmail`. New actions `verifyEmail(token)` (POST
  /v1/auth/verify) and `resendVerification(email)` (POST
  /v1/auth/resend-verification). `clearSession` also clears the pending email.
- `RegisterForm.vue`: on success shows a "Check your email" panel (no navigation)
  with a Resend button (`data-testid="check-email"`).
- `LoginForm.vue`: catches `ApiError.code === AUTH_EMAIL_NOT_VERIFIED` (6) and
  shows a "please confirm your email" banner (`data-testid="not-verified"`) with
  a Resend button (shown only when the identifier is an email).
- New public route `/verify` → `src/pages/VerifyPage.vue`: reads `?token=`, calls
  `verifyEmail`, renders pending/success/error.
- `src/types/api.ts`: `VerifyEmail*`/`ResendVerification*` types +
  `AUTH_EMAIL_NOT_VERIFIED = 6` code constant.
- Tests added: authStore (no auto-login + verify + resend), guards (/verify public).

## Next

- (all core scope complete — see below)
- Verify: `npm install && npm test && npm run build` on a machine with Node
  (Node is not installed in this build env, so nothing was run here).
- Optional: swap in a component lib (PrimeVue/Vuetify) — deliberately omitted.
- Optional: i18n scaffolding (nice-to-have per docs).

## Completed since last update

- MapView.vue — Leaflet + OSM tiles, circle markers per object, Vue-mounted
  popups wired to the store.
- MapPage.vue — geolocation center + fallback, start/stop map polling,
  tab-hidden handling, `?object=` deep link from the friends "where" link.
- favicon.svg, README.md.
- friendsStore test (refresh, find-by-login reduced profile, request/accept/block).
- Committed milestone inside the worktree.

## Notes

- Node is NOT installed on this machine, so `npm install` / `npm test` / `npm run
  build` could NOT be run here. Everything is authored to run under Vitest/vue-tsc
  once deps are installed; treat first-run typecheck as the outstanding verify step.
- Store pollers (map + friends) are keyed by store instance via a WeakMap so a
  fresh Pinia per test / HMR never reuses a stale poller closure.
- Token storage is in-memory (Pinia) per docs; localStorage trade-off documented as
  a TODO in `src/stores/authStore.ts`.
- Backend REST paths assumed as `/<service>/<Method>` (e.g. `/auth/login`,
  `/profiles/EditUser`, `/map/LoadMap`). `FindUserByLogin` is assumed (not pinned in
  the doc) — flag to backend.
- Frontend placed at repo-top-level `frontend/` per task instruction (the agent doc
  says `Frontend/WebApplication/`; explicit task instruction overrode it).
