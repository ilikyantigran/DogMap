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
