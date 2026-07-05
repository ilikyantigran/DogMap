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

## Feature: email confirmation on registration (branch release/auth-email-confirm)
_Added 2026-07-05._ Full vertical slice (Auth backend + frontend). Backend green
(`go test ./...`), `make generate` reproducible.

Backend done:
- Migration `0002_email_verified.{up,down}.sql` — `auth.credentials.email_verified
  bool NOT NULL DEFAULT false`. Mounted in docker-compose initdb as
  `11-auth-email-verified.sql`.
- Proto: new RPCs `VerifyEmail` (`POST /v1/auth/verify`) + `ResendVerification`
  (`POST /v1/auth/resend-verification`); regenerated.
- postgres: `Credential.EmailVerified`; `FindByEmail`, `MarkEmailVerified`;
  `FindByLoginOrEmail` now selects `email_verified`.
- valkey: `verify:{token} -> user_id` single-use tokens —
  `CreateVerifyToken` / `ConsumeVerifyToken` (GET+DEL).
- domain/email: stdlib `net/smtp` `SMTPSender` (no auth/TLS, Mailpit-shaped) +
  `NoopSender` (logs link when smtp.host empty).
- server: new codes `codeEmailNotVerified=6`, `codeBadToken=7`. Register inserts
  unverified, keeps idempotent CreateProfile handoff, then best-effort sends the
  verify email (send/token failure does NOT fail register). Login gates on
  `EmailVerified` AFTER password verify (no enumeration oracle). `VerifyEmail`
  consumes token + marks verified. `ResendVerification` always returns generic ok
  (no email-enumeration oracle); only sends for existing unverified accounts.
- config: `smtp.{host,port,from}` + `app_base_url` + `auth.verify_ttl_seconds` in
  both yamls (docker → mailpit:1025, app_base_url http://localhost:5173).
- docker-compose: `mailpit` service (axllent/mailpit, 8025 UI + 1025 SMTP); auth
  depends_on mailpit.

Frontend: see frontend/PROGRESS.md (register "check email" state, /verify page,
login resend button).

## Next
- (optional, out of current scope) integration test through the HTTP edge
  against real Postgres+Valkey+Mailpit (testcontainers) — deferred.
- Profiles/Map teams: match the cross-service contract points (see summary).

## Notes
- Ports: gRPC 8080, HTTP 8081.
- Error envelope: app-level `code` (0=ok) + `message`; codes enumerated in
  internal/app/auth_server.go (0 ok, 1 taken, 2 bad-request, 3 bad-creds,
  4 no-session, 5 internal, 6 email-not-verified, 7 bad-token).
- Session TTL 24h sliding; verify-token TTL 24h single-use; Argon2id
  64MiB/t=3/p=2 (config-tunable).
- Verify link shape: `${app_base_url}/verify?token=` (token URL-escaped).
- Local `go run` uses NoopSender (empty smtp.host) — link is logged, not mailed.
- `make generate` needs `make vendor-proto` first (vendor-proto is gitignored).
- `profiles.proto` here is a *stub slice* — the Profiles service owns the real
  proto; keep package/message shapes in sync.
