# DogMap — Frontend

> Source of truth for frontend design. Companion: `01-Idea.md`, `02-Backend.md`.
> Status: MVP design drafted (2026-07-02). Supersedes `Frontend.docx` (which was one line).

## Stack

- **Vue 3 + TypeScript + Vite**
- **Pinia** — state management
- **Vue Router** — routing + route guards
- **Component library** — PrimeVue or Vuetify (pick one; speeds up forms/dialogs)
- **Map** — **Leaflet** (or MapLibre GL) + **OpenStreetMap** tiles.
  Free, no per-call billing, pairs with PostGIS/GeoJSON backend. Avoid Google Maps for MVP.
- **HTTP** — a thin `apiClient` (fetch/axios) that injects the `auth_token` header
  and centralizes error handling.

Build **responsive / mobile-web from day one** (native apps are post-MVP but planned).

## Auth token handling

- Token is **opaque** (from Auth service), sent as header `auth_token` on every call.
- Storage: prefer **in-memory (Pinia) + re-login on refresh** for MVP simplicity, OR
  `localStorage` if you accept the XSS exposure for convenience. (Decide before build;
  in-memory is safer, localStorage is easier for a learning MVP.)
- **Route guards:** Profile and Map routes require a token; redirect to Auth otherwise.
- On any `401`/expired session → clear token, redirect to Auth.

## Pages, routes, components

### Auth Page (`/login`, `/register`) — public
- `LoginForm` — (email OR login) + password → `POST /auth/login`.
- `RegisterForm` — login, email, password, confirm-password → `POST /auth/register`,
  then auto-login.
- Client-side validation: email format, password confirm match, phone later on profile.

### Profile Page (`/profile`) — guarded
- `ProfileForm` — name, surname, phone (E.164), read-only login/email → `EditUser`.
- `PetList` / `PetEditor` — add/edit/remove pets (breed, name, sex, castrated, age).
- `FriendsPanel`:
  - friends list with **on-walk indicator** + "where" (map object link),
  - incoming requests (approve/decline → `SendFriendResponse`),
  - outgoing requests,
  - find-friend-by-login → shows **reduced** profile → send request,
  - remove / block actions.
- Save button (explicit save, per the product spec). Logout button → `Logout` + clear token.

### Map Page (`/map`) — guarded
- `MapView` (Leaflet) — renders `map_objects` from `LoadMap` around the user's location.
- `MapLegend` — object types: Park / Dog-park / Dog-beach.
- `MapObjectPopup` — on select: type, **visitor_count**, **friends here** (from
  `friend_ids_here`), and a **"I'm going here" / "Not going" toggle** → `ChangeMapObjectStatus`.
- Geolocation: request browser location for the `LoadMap` center (fallback: manual pan).

## State (Pinia stores)

- `authStore` — token, current user_id, login/register/logout, route-guard helper.
- `profileStore` — profile + pets, load/edit.
- `friendsStore` — friends, incoming/outgoing requests, request actions, block/unfriend.
- `mapStore` — nearby objects, selected object, my current presence, mark visiting/not.

## Real-time = polling (MVP), behind an abstraction

- No WebSocket for MVP. Poll instead, **but hide it behind a store action** so it can be
  swapped for WebSocket/SSE later without touching components.
  - Map page active → `mapStore.refresh()` on an interval (e.g. every 20–30s) to update
    `visitor_count` / `friends_here`.
  - **Presence heartbeat:** while the user is marked "visiting", re-send the visiting
    action / ping every **2–3 min** so the backend 15-min TTL never expires mid-walk.
  - Profile page active → `friendsStore.refresh()` for request/status updates.
- Stop all polling when the tab is hidden / page unmounts.

## Cross-cutting

- Centralized error/toast handling (map backend `code`/`message`).
- Loading + empty states (e.g. "0 people here").
- i18n-ready structure if multi-region is a goal (nice-to-have).
- Keep components dumb; all API + polling logic in stores.

## Open items

- Decide token storage (in-memory vs localStorage) before build.
- Decide component lib (PrimeVue vs Vuetify).
- Map lib final pick (Leaflet vs MapLibre GL).
