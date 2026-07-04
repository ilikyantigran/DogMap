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

## Next

- (all core scope complete — see below)
- Verify: `npm install && npm test && npm run build` on a machine with Node
  (Node is not installed in this build env, so nothing was run here).
- Optional: swap in a component lib (PrimeVue/Vuetify) — deliberately omitted.
- Optional: i18n scaffolding (nice-to-have per docs).

## Completed since last update — two-step registration (branch release/auth-registration-stage)

- **ProfileSetup step** (post-login, one-time). After a confirmed login, if the
  user's profile is empty (name+surname both blank) and they haven't skipped/
  completed before, they're routed to `/profile/setup` to fill name/surname/
  phone/pets; a clear **Skip** action lets them do it later on Profile. Either
  action persists a per-user localStorage flag so later logins don't nag.
  - `src/lib/profileSetup.ts` — `isSetupDone/markSetupDone` (localStorage key
    `dogmap.setupDone.<userId>`) + `isProfileEmpty` (name+surname blank).
  - `src/pages/ProfileSetupPage.vue` — reuses `profileStore.save` (EditUser) and
    the `PetEditor` component; Save-and-continue vs Skip; loads self on mount if
    not already loaded. Routes to `/map` after either action.
  - `src/router/index.ts` — new guarded route `profile-setup` (`/profile/setup`)
    + guard extension: on a guarded nav, when authed & !isSetupDone & profile is
    POSITIVELY empty, divert to setup. Cheap isSetupDone short-circuit avoids any
    fetch in the common case; excludes the setup route (no loop); never blocks
    navigation on a profile-fetch error. Guard is now async.
  - Register/verify flow left untouched (composes with the email-confirmation
    branch).
  - Tests: `tests/lib/profileSetup.spec.ts`, `tests/pages/ProfileSetupPage.spec.ts`
    (Complete/Skip/load-on-mount), `tests/router/profileSetup.spec.ts` (redirect,
    load-in-guard, done short-circuit, non-empty, no-loop, fetch-error, auth).
  - `npm test` 47 passed (9 files); `npm run build` (vue-tsc + vite) green.

## Completed earlier

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
