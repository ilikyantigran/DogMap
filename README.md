# DogMap

Coordinate dog walks: see who's walking their dog near you right now, at dog-relevant
map objects (parks, dog-parks, dog-friendly beaches). Point-of-interest presence, not
live GPS. See `Docs/` for the full design (`01-Idea.md`, `02-Backend.md`, `03-Frontend.md`).

## Layout

| Path | What |
|---|---|
| `auth/` | Auth service (Go) — register/login/logout, opaque Valkey sessions, Argon2id |
| `profiles/` | Profiles service (Go) — profiles, pets, friends/blocks, `friends:{uid}` cache |
| `map/` | Map service (Go + PostGIS) — 5km radius, ephemeral presence, privacy filter |
| `frontend/` | Web app (Vue 3 + TS + Vite + Pinia + Leaflet/OSM) |
| `docker-compose.yml` | Full backend stack for local verification |

Ports (host): Auth `8080`/`8081`, Profiles `8082`/`8083`, Map `8084`/`8085` (gRPC/HTTP).
Postgres `5432`, Valkey `6379`. Each service serves Swagger at `http://localhost:<http>/swagger/`.

## Run the backend stack

Prereq: Docker with Compose v2.

```bash
docker compose up --build
```

This starts Postgres+PostGIS, Valkey, and the three services. On **first** boot the
per-service schemas are applied automatically (via Postgres `initdb`), and services
wait for Postgres/Valkey to be healthy before starting.

Check it:

```bash
# Swagger UIs
open http://localhost:8081/swagger/   # auth
open http://localhost:8083/swagger/   # profiles
open http://localhost:8085/swagger/   # map

# End-to-end smoke: register -> login (opaque token) -> use it
curl -s localhost:8081/v1/auth/register -d '{"login":"Test1","email":"t1@example.com","password":"pw"}'
curl -s localhost:8081/v1/auth/login    -d '{"login":"Test1","password":"pw"}'
```

Teardown:

```bash
docker compose down      # stop, keep data
docker compose down -v   # stop and wipe the DB (schemas re-apply on next up)
```

> **Schemas & migrations.** The services don't run migrations on boot; this stack
> applies them via `initdb`, which only runs on a fresh volume. After editing a
> migration, run `docker compose down -v` then `up` again. For real deployments,
> swap in a proper migration runner (golang-migrate/goose) as a startup step.

## Run the frontend (dev)

The frontend is a Vite dev app; its dev proxy routes each `/v1` prefix to the
service HTTP ports above, so run it on the host against the composed backend:

```bash
cd frontend
npm install
npm run dev        # http://localhost:5173
npm test           # 28 vitest specs
npm run build      # vue-tsc typecheck + production build
```

## Integration tests against the live DB

Unit tests need no DB (`go test ./internal/... ./cmd/...` in each service). The
DB-backed integration tests skip unless a DSN is provided — point them at the
composed Postgres:

```bash
# Profiles (e.g. citext case-insensitive lookup for FindUserByLogin)
cd profiles && PROFILES_TEST_DSN='postgres://postgres:postgres@localhost:5432/dogmap?sslmode=disable' \
  go test ./internal/domain/postgres/... -v
```

## Seed map objects (optional)

`map/cmd/seed-osm` upserts parks/dog-parks/beaches from OpenStreetMap. The Overpass
fetch is currently a documented stub — implement it, then:

```bash
cd map && CONFIG_PATH=./configs/values_local.yaml go run ./cmd/seed-osm
```

## Notes

- One Postgres instance, one schema per service, no cross-schema FKs — services stay
  independently deployable.
- Sessions/presence/friend-cache live in Valkey; only durable data is in Postgres.
- There is no single API gateway yet — the frontend dev proxy fans out to the three
  service ports. A production BFF/gateway is a later step.
