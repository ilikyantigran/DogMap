# Map service — PROGRESS

Terse build log. Source of truth: `../Docs/02-Backend.md` (Service: Map, Presence
architecture, data model) + `../Docs/01-Idea.md` (locked decisions, Path 1).

## Done
- Scaffold from house-style templates: `cmd/map`, `configs/{local,docker}.yaml`,
  `internal/{app,domain,infra,presence}`, `pkg/api`, `vendor-proto`, Dockerfile,
  Makefile. Module `map-service`, Go 1.26. gRPC :8084 / HTTP :8085.
- Proto contract `api/map-service/v1/map.proto` → LoadMap, GetMapObject,
  ChangeMapObjectStatus + REST gateway annotations. `make generate` clean;
  swagger embedded in `internal/infra/docs`.
- Migration `migrations/0001_map_schema.{up,down}.sql`: `map.map_objects`
  (id uuid, object_type CHECK, name, `GEOGRAPHY(Point,4326)`, source_osm_id) +
  GiST index `map_objects_location_gix`. PostGIS extension. No cross-schema FKs.
- Postgres store (`internal/domain/postgres`): `ObjectsWithin` (ST_DWithin 5km via
  the geography GiST index), `ObjectByID`, `UpsertOSM` (idempotent, keyed by
  source_osm_id).
- Valkey store (`internal/domain/valkey`): presence keys per doc — MarkVisiting
  (SADD + SET EX 900, evicts stale prior object), MarkNotVisiting (SREM + DEL),
  Heartbeat (TTL refresh + re-SADD), CurrentObject, VisitorCount (SCARD),
  FriendIDsHere (SINTER visitors ∩ friends:{caller}), plus janitor helpers and
  `UserIDForToken` (reads session:{token}, read-only — Auth owns it).
- Server (`internal/app/map_server.go`): 3 RPCs. Acting user from `auth_token`
  header → session lookup, NEVER from body. Privacy view = count for all +
  friend_ids_here for caller; raw visitor set never returned. code/message
  envelope; gRPC status codes for errors.
- Presence janitor (`internal/presence`): sweeps object:*:visitors, drops members
  whose presence:{user} key expired. Polling MVP; keyspace-notif variant noted.
- App wiring (`internal/app/app.go`): telemetry → stores → server → janitor
  goroutine → gRPC + gateway (forwards auth_token) + /metrics + /swagger.
  Graceful shutdown drains the janitor.
- OSM seed job STUB (`cmd/seed-osm`): wired to UpsertOSM; fetch is a documented
  TODO. Safe no-op until Overpass fetch lands.
- Tests (test-first, all green): server unit tests (fakes) cover Path 1 steps 4-6
  (0→1→0 visitor count), privacy filtering, token-not-body identity, radius=5000
  pass-through, auth/validation/not-found/internal error mapping; janitor tests
  cover stale-only removal + no-op. `go vet` + `go build` + `go test ./...` clean.
- Milestone commit `af100c1` in worktree (full scaffold + tests).

## Next
- Integration test through the transport edge against real Postgres+PostGIS and
  Valkey (testcontainers/compose): config load → App.Run → HTTP gateway → stores.
  Proves wiring + that the GiST/ST_DWithin query really returns 5km results.
- Implement the OSM Overpass fetch in `cmd/seed-osm` (currently returns nil).
- `docker build` verification with values_docker.yaml (needs Docker daemon).
- Optional: a migration runner (`cmd/migrate`) or document applying
  `migrations/*.sql` via the shared DB tool.

## Notes / cross-service contracts (other agents must match)
- Session: Map READS `session:{token}` (JSON `{"user_id":..,"exp":..}`) — Auth
  must write exactly that shape/key. Header name is `auth_token`.
- Friends: Map READS `friends:{user_id}` SET for SINTER — Profiles owns/writes it.
  Members must be the same string UUIDs used everywhere.
- Presence keys owned by Map: `presence:{user_id}` (EX 900), `object:{id}:visitors`.
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
