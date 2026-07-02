# DogMap — Product / Idea

> Source of truth for the product concept and scope. Companion files: `02-Backend.md`, `03-Frontend.md`.
> Status: MVP design locked (2026-07-02). Supersedes `The idea.docx`.

## Concept

A platform for dog-owners ("dog-persons") to coordinate walks: see who is walking
their dog **near you right now**, at dog-relevant places (parks, dog-parks,
dog-friendly beaches), and connect with friends who also have dogs.

Key design stance: **point-of-interest presence, not live GPS tracking.** A user
marks "I'm going to *this park*" — we never stream their raw coordinates. Cheaper,
and far less invasive.

## MVP scope

Web application (Vue + Go microservices). Mobile apps are **post-MVP**, but build
the web UI responsive from day one.

Three pages:
1. **Auth Page** — login / register. Public.
2. **Profile Page** — user + pets + friends. Logged-in only.
3. **Map Page** — map with dog-relevant objects + presence. Logged-in only.

## Locked decisions (do not relitigate without a reason)

| Decision | Choice | Rationale |
|---|---|---|
| **Privacy model** | Friends see *names*; strangers see *counts only* | Safety/GDPR. A stranger sees "3 people here now"; only friends see identity/status. |
| **Presence** | Ephemeral, TTL-based (15 min), refreshed by client heartbeat | Prevents "stuck at the park forever" when app closes. |
| **Status source of truth** | Derived from map presence — NOT a separately stored profile flag | "On a walk" == "currently visiting a map object". |
| **Map data source** | Seeded from OpenStreetMap, read-only for MVP | No moderation needed; fast to populate. |
| **DB** | Postgres + PostGIS | See `02-Backend.md`. |
| **User ids** | String UUIDs everywhere | No sequential-count leak; cross-service friendly. |
| **Auth tokens** | Opaque session tokens in Valkey | Trivial revocation on logout. |

## Privacy rules (enforced backend-side)

- **Presence identity is friends-only.** Map responses return `visitor_count` for
  everyone and `friend_ids_here` computed *for the calling user*. Raw visitor id
  lists are never sent to the client.
- **PII (email, phone) is friends-only.** `GetUserInfo` returns a full shape to
  friends/self and a reduced shape (name + pets + presence) to non-friends.
- **The acting user is always the token owner**, never a `user_id` from the request
  body. Body ids are only ever *target* ids.

## Safety features (required for MVP, not v2)

- Remove friend (unfriend).
- Block user (blocked users cannot send requests or see presence).
- (Nice-to-have) Report user.

## Data model (product view)

**User**
- `login` — string, **immutable**, unique
- `name`, `surname` — string
- `email` — string, valid format, unique
- `phone` — E.164 format (country code + number)
- `pets` — 0..N
- presence/status — **derived**, not stored on the profile

**Pet**
- `breed` — string
- `name` — string
- `sex` — M / F
- `is_castrated` — bool
- `age` — int

**Map object**
- `id` — UUID
- `object_type` — enum: `PARK`, `DOG_PARK`, `DOG_BEACH`
- `longitude`, `latitude` — float
- presence — ephemeral (count for all, friend ids for the caller)

## User paths (acceptance tests — keep these current)

**Path 1 — register → profile → mark presence**
1. Register (email, login, password, confirm).
2. Log in (email + password).
3. Open Profile, fill name/surname/phone, add pet (Poodle "Bruno", M, castrated, age 3), save.
4. Open Map, pick a park → sees `0 people here`.
5. Mark "I'm going here" → sees `1 person here`.
6. Mark "not going" → back to `0`.

**Path 2 — friend request (assumes Test1 exists)**
1. Register + log in as Test2.
2. Fill profile.
3. Find friend by login `Test1` → see Test1's *reduced* profile.
4. Send friend request → Test1 can approve/decline.
5. After approval: each sees the other's presence + full profile.

## Out of scope for MVP

- Live GPS tracking.
- Native mobile apps.
- User-generated / editable map objects.
- Chat / messaging.
- Push notifications (polling instead — see `03-Frontend.md`).
