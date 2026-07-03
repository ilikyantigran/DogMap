# Auth service — build progress

Terse running log. Update after every meaningful step.

## Done
- Scaffold: `auth/` module (`go mod init auth-service`), house-style tree
  (cmd/configs/internal/{app,clients,domain,infra}/pkg/api/api/migrations),
  vendor-proto + telemetry.go copied from knp-service.
- Proto: `api/auth/v1/auth.proto` (Register/Login/Logout + gateway HTTP
  annotations) and `api/profiles/v1/profiles.proto` (CreateProfile downstream
  slice Auth depends on).
- Generate: `make generate` clean → `pkg/api/auth/v1/*` (pb, grpc, gw, swagger)
  + `pkg/api/profiles/v1/*` client stub; swagger copied into infra/docs.
- Migration: `migrations/0001_init.{up,down}.sql` — `auth` schema, citext ext,
  `auth.credentials(user_id uuid pk, login citext unique, email citext unique,
  password_hash text, created_at timestamptz)`. No cross-schema FKs.
- Domain — password: Argon2id hasher (PHC-encoded, random salt, constant-time
  Verify). Unit tests green.
- Domain — valkey: session store — opaque token gen, `session:{token}`
  create/lookup(sliding TTL)/delete.
- Domain — postgres: credentials store — InsertCredential (ErrDuplicate on
  unique violation), FindByLoginOrEmail (citext, ErrNotFound).
- Clients: Profiles gRPC client (CreateProfile handoff).
- Server: Register/Login/Logout on narrow store/client interfaces. Argon2id
  hashing, duplicate rejection (no field hint), generic bad-creds (no
  enumeration oracle), token-from-`auth_token`-header for Logout, idempotent
  CreateProfile handoff. Unit tests green (incl. acceptance Path-1 register→login).
- App wiring: telemetry → Profiles client → postgres+valkey stores → server →
  gRPC + HTTP gateway (/metrics, /swagger/), metadata annotator maps
  `auth_token` header → gRPC metadata. Graceful shutdown.
- Config: Config struct + values_local/values_docker (identical keys).
- Dockerfile (multi-stage → distroless, CONFIG_PATH=values_docker) + Makefile
  (vendor-proto + generate).
- Verify: `go build ./...` clean, `go vet ./...` clean, `go test ./...` green.
  Binary boots: config loads, telemetry up (`service:auth`), Profiles client
  dials lazily; fails only at Postgres ping (no local PG) — wiring proven.
- Committed to worktree (milestone): "Scaffold Auth microservice ...".

## Next
- (optional, out of current scope) integration test through the HTTP edge
  against real Postgres+Valkey (testcontainers) — deferred; needs Docker infra.
- Profiles/Map teams: match the cross-service contract points (see summary).

## Notes
- Ports: gRPC 8080, HTTP 8081.
- Error envelope: app-level `code` (0=ok) + `message`; codes enumerated in
  internal/app/auth_server.go.
- Session TTL 24h sliding; Argon2id 64MiB/t=3/p=2 (config-tunable).
- `profiles.proto` here is a *stub slice* — the Profiles service owns the real
  proto; keep package/message shapes in sync.
