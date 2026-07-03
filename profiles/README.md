# Profiles service

DogMap's **Profiles** microservice (Go): user profiles, pets, the friend graph,
blocking, and the `friends:{user_id}` Valkey cache that Map reads for presence
privacy. Owns the `profiles` Postgres schema. Never reports presence itself.

Contract source of truth: `../Docs/02-Backend.md` (Service: Profiles) and
`../Docs/01-Idea.md` (privacy rules, acceptance paths).

## Layout (house style)

```
api/profiles/v1/profiles.proto   proto contract (edge RPCs + internal CreateProfile)
cmd/profiles/main.go             thin entrypoint
configs/values_{local,docker}.yaml
internal/app/                    App object + ProfilesService impl
internal/domain/postgres/        `profiles` schema store + migrations
internal/domain/valkey/          friends:{uid} cache + session read
internal/infra/{config,docs,telemetry}
pkg/api/profiles/v1/             generated gRPC/gateway/swagger (importable by other services)
```

Ports: gRPC **8082**, HTTP **8083** (REST gateway + `/metrics` + `/swagger/`).

## Regenerate API from proto

```sh
make vendor-proto   # once: fetch third-party protos into vendor-proto/
make generate       # protoc → pkg/api + copy swagger into internal/infra/docs
```

## Run tests

```sh
go test ./...                                  # unit + transport tests (no DB needed)

# Postgres integration tests (apply migration + real queries) need a live DB:
PROFILES_TEST_DSN='postgres://postgres:postgres@localhost:5432/dogmap?sslmode=disable' \
  go test ./internal/domain/postgres/...
```

Without `PROFILES_TEST_DSN` the integration tests skip, so the default run is
green on a machine with no database.

## Run the service

Needs Postgres (with the `citext` extension) and Valkey reachable at the
addresses in `configs/values_local.yaml`. Apply the migration in
`internal/domain/postgres/migrations/0001_init.up.sql`, then:

```sh
go run ./cmd/profiles          # CONFIG_PATH defaults to ./configs/values_local.yaml
```

Docker: `docker build -t profiles .` (sets `CONFIG_PATH=/app/configs/values_docker.yaml`).

## Cross-service contract points

- **Auth → Profiles:** call `CreateProfile(user_id, login, email)` over gRPC after
  register. Idempotent (retry-safe), **not** on the HTTP edge.
- **Profiles → Map:** Profiles keeps `friends:{user_id}` (Valkey SET) fresh on
  accept / remove / block. Map computes `friend_ids_here` via
  `SINTER(object:{id}:visitors, friends:{caller})`. Profiles never writes presence.
- **Sessions:** `session:{token}` (JSON `{ "user_id": ... }`) is owned by Auth;
  Profiles only reads it to resolve the acting user from the `auth_token` header.
