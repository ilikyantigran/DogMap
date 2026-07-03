# Profiles service — build progress

Terse running log. Update after every meaningful step.

## Done
- Read source-of-truth contracts (`Docs/02-Backend.md` Profiles section, `Docs/01-Idea.md`) and house-style skill refs.
- Scaffolded module `profiles/` in house layout (cmd/configs/internal/pkg/api).
- Copied portable templates (telemetry verbatim; main/docs/Dockerfile/Makefile placeholder-substituted).
- Wrote proto `api/profiles/v1/profiles.proto`: all 8 edge RPCs (HTTP-annotated) + internal CreateProfile (no HTTP annotation).
- `make generate` clean → generated pb/grpc/gateway/swagger into `pkg/api/`.
- Config (`internal/infra/config`) + both `configs/*.yaml` (postgres + valkey sections).
- Migration `0001_init` for `profiles` schema: profiles, pets, friendships (both-dir), friend_requests, blocks. citext, no cross-schema FKs.
- Postgres store (`internal/domain/postgres`): profile CRUD, friend graph, block cascade, friends-cache source queries.
- Valkey store (`internal/domain/valkey`): `friends:{uid}` SET cache + read-only `session:{token}` resolution.
- Server (`internal/app/profiles_server.go`): all RPCs, token-derived identity, PII full-vs-reduced, friend-graph rules, block cascade, cache refresh.
- App wiring (`internal/app/app.go`): stores → server → gRPC + HTTP gateway (auth_token header matcher) + metrics + swagger.
- `go build ./...` passes.
- Unit tests (`profiles_server_test.go`) with fake store/cache: identity, PII scoping, friend rules, block cascade, idempotent CreateProfile.

- Unit tests green (19 server + 2 config).
- Postgres integration test (skip-if-no-`PROFILES_TEST_DSN`): applies migration, exercises idempotent create, pet replacement, two-directional friend graph, block cascade, pending-request cancellation.
- End-to-end transport test: real gRPC + grpc-gateway; POST /v1/profiles/get proves reduced-for-non-friend, unauthorized-without-token, full-PII-for-friend through the HTTP edge.
- `go vet ./...` clean; binary builds; boot from values_local loads config + telemetry, fails cleanly at Postgres connect (correct wiring order).
- Verified swagger exposes exactly the 8 edge RPCs; CreateProfile absent from swagger + gateway (internal gRPC only).
- Committed scaffold milestone (`f75ecc0`).

## Next
- (Needs live deps — not available in this sandbox) run Postgres integration tests + full `go run` boot with Valkey/Postgres up; `docker build`.
- Auth agent: generate the Profiles client stub and call `CreateProfile` idempotently on register.

## Notes
- Ports: gRPC 8082, HTTP 8083.
- Acting user resolved from `auth_token` header via Valkey `session:{token}` (namespace owned by Auth; read-only here).
- CreateProfile intentionally NOT in the HTTP gateway (internal gRPC only).
- Profiles never reports presence; keeps `friends:{uid}` fresh for Map's SINTER.
