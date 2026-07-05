# Map service — PROGRESS

Terse build log. Source of truth: `../Docs/02-Backend.md` (Service: Map, Presence
architecture, data model) + `../Docs/01-Idea.md` (locked decisions, Path 1).

## Current status (2026-07-04) — branch `release/map-friends-widget`

Building the "friends on the map" vertical slice: Map backend (MapObject.name +
new FriendsPresence RPC) AND the frontend FriendsOnMap widget. This branch is the
shared foundation Branch 6 (friend status) consumes.

STATUS: DELIVERED. PR #7 open (https://github.com/ilikyantigran/DogMap/pull/7),
awaiting owner review/merge — do NOT run pr-reviewer or merge (per task). Backend
(map): `go build/vet/test ./...` pass; `make generate` reproducible. Frontend:
`npm test` (33) + `npm run build` pass. Commit d120d88 on release/map-friends-widget.

## Plan (this branch)

Backend (map): DONE — go build/vet/test ./... green.
1. [x] map.proto: `name=8` on MapObject; RPC FriendsPresence + messages.
2. [x] `make generate` (reproducible — regenerated pb/gw/swagger).
3. [x] Server.view surfaces o.Name into MapObject.Name.
4. [x] valkey.Friends(caller) = SMEMBERS friends:{caller} (read-only).
5. [x] Server.FriendsPresence implemented (skips no-presence + missing-object;
       caches object lookups). Tests cover happy path, skip-no-presence,
       skip-missing-object, no-friends, requires-token, friend-set-error->Internal,
       plus LoadMap surfaces name.
6. [x] Interfaces + fakes extended.

Frontend: DONE — npm test (33) + npm run build green.
7. [x] types/api.ts: `name` on MapObject; FriendPresence + FriendsPresenceResponse.
8. [x] mapStore: friendsPresence state + fetchFriendsPresence() (folded into the
       refresh tick, best-effort) + focusFriendObject(fp).
9. [x] FriendsOnMap.vue widget (right-hand rail on MapPage).
10. [x] MapPage: two-column layout, mounts FriendsOnMap.
11. [x] mapStore specs: fetch, refresh fan-out, focus (loaded + not-loaded).

Also: Docs/02-Backend.md updated — `name` in the LoadMap object shape + a new
FriendsPresence section documenting the response contract.

## Done (prior scaffold — unchanged)
- Full Map scaffold: 3 RPCs (LoadMap/GetMapObject/ChangeMapObjectStatus), postgres
  store (ST_DWithin 5km, GiST), valkey presence store, janitor, gateway forwarding
  auth_token, OSM seed stub, server+janitor unit tests. See git history.

## Decisions & constraints
- FriendsPresence body is EMPTY: acting user from auth_token header, never body
  (Docs global convention #3). Response shape (Branch 6 consumes this EXACTLY):
  `{ code, message, friends: [{ user_id, object_id, object_name, latitude, longitude }] }`.
- Only friends currently holding presence appear (derived from live presence:{friend}
  key). Friends with no presence or a missing object row are silently skipped.
- Reuses friends:{caller} SET (Profiles owns/writes; Map only READS) — same set
  already used for SINTER privacy filtering.

## Blockers / open questions
- none.

## Notes / cross-service contracts
- Session: Map READS `session:{token}` JSON `{"user_id":..,"exp":..}`. Header `auth_token`.
- Friends: Map READS `friends:{user_id}` SET (Profiles owns). Members = string UUIDs.
- Presence keys owned by Map: `presence:{user_id}` (EX 900), `object:{id}:visitors`.
- object_type enum: PARK | DOG_PARK | DOG_BEACH.
  No other service should write these.
- object_type enum: PARK | DOG_PARK | DOG_BEACH (proto + DB CHECK agree).
- Ports 8084/8085 are Map's pick (docs don't assign) — deconflict at compose time.

## Future / Backlog: rich map objects (Google-Maps-style)
> Deferred, NOT scheduled. Large multi-step feature — plan separately before any
> code. This is a scannable spec to preserve intent, not an implementation.

Goal: turn a map object from a static point into a rich place with user
**comments/reviews**, **star ratings** (with an aggregate average + count), and
**photos** — the kind of detail panel Google Maps shows for a place.

### Data model (new tables, `map` schema, no cross-schema FKs)
- `map.reviews` — one row per user comment on an object:
  `id uuid pk`, `object_id uuid` (FK → `map.map_objects`), `user_id uuid`
  (auth-owned, no cross-schema FK — store as plain uuid), `body text`,
  `created_at`, `updated_at`, `hidden_at nullable` (moderation soft-delete).
  Index `(object_id, created_at desc)` for the popup feed.
- `map.ratings` — one row per (object, user), so a user rates once and can update:
  `object_id uuid`, `user_id uuid`, `stars smallint CHECK 1..5`, `created_at`,
  `updated_at`. PK `(object_id, user_id)` enforces one-vote-per-user.
- Photos: `map.review_photos` (or an object-level `map.object_photos`) —
  `id`, `object_id`, `user_id`, `url`/`storage_key`, `created_at`, `hidden_at`.
  Binary lives in object storage (S3/MinIO); DB holds only keys + metadata.
- Aggregates (avg stars, rating count, review count) are DERIVED — compute on read
  via SQL, or maintain a denormalized `map.object_stats` cache updated on write if
  read volume demands it. Start derived; optimize later.

### API (new RPCs on Map's proto + REST gateway)
- `ListReviews(object_id, paging)` → reviews + author display (join happens in
  frontend/Profiles; Map returns user_id) + photo refs.
- `AddReview(object_id, body, stars?, photo_refs?)`, `EditReview(review_id, …)`,
  `DeleteReview(review_id)` — author-only (or moderator).
- `SetRating(object_id, stars)` — upsert on `(object_id,user_id)`; `ClearRating`.
- `GetObjectStats(object_id)` → `{ avg_stars, rating_count, review_count }`;
  fold these into the existing `GetMapObject` response so the popup gets them in
  one round-trip.
- Photo upload: presigned-URL flow (client uploads to object storage, then posts
  the key), keeping large blobs off the gRPC/HTTP edge.

### Auth & ownership (reuse existing model — no new identity)
- Acting user comes from the `auth_token` header → `session:{token}` lookup,
  NEVER from the request body (same rule as the current 3 RPCs).
- Reads (ListReviews/GetObjectStats) can be public/anonymous; all writes require a
  valid session. Edit/Delete/SetRating are scoped to the owning `user_id`.
- Author display names/avatars are Profiles' data — Map returns `user_id`; the
  frontend (or a Profiles batch lookup) resolves identity. No cross-schema join.

### Moderation / abuse
- Soft-delete via `hidden_at` (never hard-delete user content immediately);
  moderator role can hide.
- Rate-limit writes per user (Valkey counter) to blunt spam.
- Report/flag flow → `map.review_flags`; auto-hide past a threshold, human review.
- Photo safety: size/type validation, optional async content scan before publish.
- One rating per user enforced by the `(object_id,user_id)` PK; edits overwrite.

### Frontend (popup expansion)
- Expand the map-object popup into a detail panel: header (name/type) +
  **avg stars + count**, a scrollable **reviews** list (author, stars, body, photos,
  relative time), and a **photo strip/grid**.
- Signed-in users get an "Add review" affordance: star picker, comment box, photo
  attach; plus edit/delete on their own review and a report action on others'.
- Anonymous users see reviews read-only with a sign-in prompt to contribute.

### Open questions to resolve at planning time
- Do photos attach to a review or to the object directly (or both)?
- Object-storage choice + presigned-URL lifecycle; thumbnailing.
- Whether to denormalize `object_stats` from day one vs. derive-on-read.
- Moderation ownership: does Map host it or does a shared moderation service?
