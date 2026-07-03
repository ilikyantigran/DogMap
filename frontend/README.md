# DogMap Frontend

Vue 3 + TypeScript + Vite web app for DogMap. Source of truth for behavior:
`../Docs/01-Idea.md` (product) and `../Docs/03-Frontend.md` (frontend design);
API shapes in `../Docs/02-Backend.md`.

## Stack

- Vue 3 + TypeScript + Vite
- Pinia (state) — all API calls and polling live in stores, components stay dumb
- Vue Router (route guards: Auth public; Profile/Map require a token)
- Leaflet + OpenStreetMap tiles for the map
- No component library (kept dependency-light for the MVP scaffold)

## Run

```bash
npm install
npm run dev        # http://localhost:5173
```

The dev server proxies `/api/*` to `http://localhost:8080` (the Go REST edge/
gateway). Change the target in `vite.config.ts`, or set `VITE_API_BASE_URL`
(see `.env.example`) to point at a deployed gateway.

```bash
npm run build      # type-check + production build
npm run preview    # preview the production build
```

## Test

```bash
npm test           # run the Vitest suite once
npm run test:watch # watch mode
```

Tests cover the apiClient (auth_token injection, 401 + envelope errors), the
auth/friends/map stores (including the presence heartbeat and the register→
auto-login and friend-request acceptance paths), the router guards, and the
client-side validation helpers.

## Architecture notes

- **Auth token is held in memory** (Pinia `authStore`). This is the safer MVP
  choice (no XSS-readable storage), at the cost of a re-login on full page
  refresh. See the `TODO(token-storage)` in `src/stores/authStore.ts` for the
  localStorage trade-off.
- **Polling is behind store actions** (`src/lib/poller.ts`), so it can be
  swapped for WebSocket/SSE later without touching components: map refresh
  (~25s), presence heartbeat (~2m, under the backend's 15m TTL), friends refresh
  (~30s). All polling stops on tab-hidden / page unmount.
- **Privacy:** the UI only ever shows `visitor_count` for strangers and
  `friend_ids_here` (mapped to friend logins) for the caller — never raw visitor
  lists or non-friend PII.

## Layout

```
src/
  api/            apiClient (auth_token + 401 + code/message), errors, wired singleton
  stores/         authStore, profileStore, friendsStore, mapStore, toastStore
  router/         routes + guards
  pages/          AuthPage, ProfilePage, MapPage
  components/     auth/, profile/, map/, ToastHost
  lib/            poller, validation, geolocation, mapObjects, handleError
  types/          api.ts (mirrors Docs/02-Backend.md)
tests/            mirrors src/ (Vitest + @vue/test-utils + happy-dom)
```
