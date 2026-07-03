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
