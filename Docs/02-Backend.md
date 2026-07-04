# DogMap — Backend

> Source of truth for backend design & contracts. Companion: `01-Idea.md`, `03-Frontend.md`.
> Status: MVP design locked (2026-07-02). Supersedes `Backend.docx`.
> Contracts below are the **edge/REST-gateway** shapes (what the frontend sees).
> Internally services talk **gRPC via proto**; these JSON shapes map 1:1 to proto messages.

## Stack

- **Language:** Go 1.26
- **Long-term storage:** **Postgres + PostGIS** (spatial index for the 5km radius query;
  relational integrity for users & the friends graph).
- **Cache / ephemeral:** **Valkey** (sessions, presence, cached friend sets).
- **Transport:** gRPC (proto contracts) between services; each service also exposes an
  **HTTP port + Swagger**.
- **Observability/SRE:** Prometheus (metrics), Grafana (dashboards), Jaeger (tracing).
- **Topology:** three separate services (Auth, Profiles, Map). One Postgres instance,
  **one schema per service, no cross-schema FKs** (so a service can be split out later).

## Global conventions (apply to every service)

1. **Ids are string UUIDs** everywhere (`user_id`, `map_object_id`, `friend_request_id`, …).
2. **Auth = opaque session token** issued by Auth service, stored in Valkey
   (`session:{token} -> {user_id, exp}`). Sent as header `auth_token`.
   Logout deletes the key → instant revocation. TTL e.g. 24h, sliding.
3. **The acting user id is derived from the token, never from the request body.**
   Body ids are only *target* ids (`user_id_target`, `map_object_id`, …).
4. **Passwords** are hashed with **Argon2id** (never stored/transmitted in plaintext
   beyond the TLS-protected register/login call).
5. **PII (email, phone) is friends-only** — see `GetUserInfo`.
6. Every response carries `code` (int) + `message` (string) on error paths.

## Enums

```
object_type : PARK | DOG_PARK | DOG_BEACH
presence_action : VISITING | NOT_VISITING
friend_status : NONE | PENDING_OUT | PENDING_IN | FRIENDS | BLOCKED
```

`user status` ("on a walk") is **not stored** — it is derived: a user is "on a walk"
iff they currently hold presence in any map object (a live Valkey key).

## Presence architecture (core of the app)

Ephemeral, in Valkey, never in Postgres:

```
Mark VISITING(object_id):
  SADD  object:{object_id}:visitors {user_id}
  SET   presence:{user_id} {object_id} EX 900          # 15 min TTL
Client heartbeat (poll while on Map page, ~2-3 min):
  refresh presence:{user_id} TTL; re-ensure SADD
Mark NOT_VISITING / TTL expiry:
  SREM  object:{object_id}:visitors {user_id}; DEL presence:{user_id}

LoadMap(caller):
  for each object in radius:
    visitor_count  = SCARD object:{id}:visitors
    friend_ids_here = SINTER(object:{id}:visitors, friends:{caller})   # cached friend set
```

A small janitor (or keyspace-notification handler) removes a user from
`object:*:visitors` when `presence:{user_id}` expires, keeping counts honest.

**Presence TTL = 15 min. Client heartbeat = every 2–3 min.**

---

## Service: Auth

Registration, authentication, session lifecycle. Owns credentials only
(login, email, password hash, user_id). On register it triggers profile creation
in Profiles (see handoff below).

### Register
Request:
```json
{ "login": "string", "email": "string", "password": "string" }
```
Response:
```json
{ "code": 0, "message": "string", "user_id": "uuid" }
```
- Rejects duplicate login or email. Hashes password (Argon2id).
- On success, creates the empty profile in Profiles (see handoff).

### Login
Request (need `(login OR email) AND password`):
```json
{ "login": "string", "email": "string", "password": "string" }
```
Response:
```json
{ "code": 0, "message": "string", "token": "opaque", "user_id": "uuid" }
```
- On failure: no token, error code + message (bad credentials / no such user).
  Do **not** reveal which field was wrong.

### Logout
Request: header `auth_token`. No body user_id (derived from token).
Response:
```json
{ "code": 0, "message": "string" }
```
- Deletes `session:{token}` in Valkey → token unusable.

### Auth → Profiles handoff
On successful Register, Auth calls Profiles `CreateProfile(user_id, login, email)`
(gRPC) to seed an empty profile row. Synchronous for MVP; make it idempotent so it
can be retried. (Later: outbox/event if you want true decoupling.)

---

## Service: Profiles

User profiles, friend graph, blocking. Never returns presence itself — the Map
service owns presence — but exposes the **friend set** (cached to Valkey `friends:{uid}`)
that Map uses for privacy filtering.

### GetUserInfo
Request: header `auth_token`.
```json
{ "user_id_target": "uuid" }
```
Response — **full** (self or friend):
```json
{
  "user_id": "uuid", "login": "string", "name": "string", "surname": "string",
  "email": "string", "phone": "string",
  "pets": [ { "breed":"string","name":"string","sex":"M|F","is_castrated":true,"age":0 } ],
  "on_walk": true, "current_object_id": "uuid|null", "friend_status": "FRIENDS"
}
```
Response — **reduced** (non-friend): omit `email`, `phone`; may omit `current_object_id`.

### FindUserByLogin
`POST /v1/profiles/find-by-login`. Request: header `auth_token`; acting user from
the token, **never the body**.
```json
{ "login": "string" }
```
- Find-a-friend-by-login discovery lookup (frontend "add friend by login" search).
- Lookup is **case-insensitive** (`login` is `citext`).
- Returns the same shape as `GetUserInfo` but **always reduced** (no `email`/`phone`,
  no `current_object_id`, `has_pii=false`) — even when the looked-up user is already
  a friend. A discovery endpoint must never be a PII surface; full details for real
  friends stay behind `GetUserInfo`. `friend_status` **is** still computed for the
  caller so the frontend can render the right action (send request / pending / friends).
- No user with that login → **not-found envelope** `{ "code": 404, "message": "user not found" }`
  (not a transport error).

### EditUser
Request: header `auth_token`. Acting user = token owner; **no user_id in body**.
```json
{
  "name": "string", "surname": "string", "phone": "string",
  "pets": [ { "breed":"string","name":"string","sex":"M|F","is_castrated":true,"age":0 } ]
}
```
- `login`/`email` are **not** editable here (login immutable; email change is a
  separate verified flow, post-MVP). Returns the updated full profile.

### SendFriendRequest
```json
{ "user_id_target": "uuid" }
```
Response: `{ "code":0, "message":"string", "friend_request_id":"uuid" }`
- Rejected if blocked, already friends, or a pending request exists.

### SendFriendResponse
```json
{ "friend_request_id": "uuid", "resolution": true }
```
Response: `{ "code":0, "message":"string" }`
- On accept: create friendship (both directions), refresh `friends:{uid}` caches.

### ListFriends
Request: header `auth_token`. No body id.
Response:
```json
{
  "friends": [ { "user_id":"uuid", "login":"string", "on_walk":true, "current_object_id":"uuid|null" } ],
  "incoming_requests": [ { "from_user_id":"uuid", "from_login":"string", "friend_request_id":"uuid" } ],
  "outgoing_requests": [ { "to_user_id":"uuid", "friend_request_id":"uuid" } ]
}
```

### RemoveFriend / BlockUser / UnblockUser  (safety — MVP)
```json
{ "user_id_target": "uuid" }
```
Response: `{ "code":0, "message":"string" }`
- Block removes any friendship + pending requests and prevents future requests/presence visibility.

### CreateProfile (internal, called by Auth)
```json
{ "user_id":"uuid", "login":"string", "email":"string" }
```
- Idempotent. Not exposed at the edge.

---

## Service: Map

Map objects + presence. Applies the privacy model (counts for all, friend ids for caller).

### LoadMap
Request: header `auth_token`.
```json
{ "longitude": 0.0, "latitude": 0.0 }
```
Response — objects within **5km** (`ST_DWithin(location, point, 5000)`):
```json
{
  "objects": [
    {
      "id": "uuid",
      "object_type": "PARK|DOG_PARK|DOG_BEACH",
      "longitude": 0.0, "latitude": 0.0,
      "visitor_count": 3,
      "friend_ids_here": ["uuid"],
      "viewer_visiting": false
    }
  ]
}
```
- `visitor_count` for everyone; `friend_ids_here` computed for the caller
  (`SINTER` visitors ∩ `friends:{caller}`). **Never** return the raw visitor list.
- `viewer_visiting` is true when the **caller** currently holds presence in this
  object (from `presence:{caller}`). Lets the client show the right toggle state
  and avoid re-marking after a page refresh.

### GetMapObject
Request: header `auth_token`. `{ "id": "uuid" }`
Response: same object shape as above (single object, with `visitor_count` +
`friend_ids_here`).

### ChangeMapObjectStatus
Request: header `auth_token`. Acting user = token owner; **no user_id in body**.
```json
{ "id": "uuid", "action": "VISITING|NOT_VISITING" }
```
Response: the updated object shape (id, type, coords, `visitor_count`, `friend_ids_here`).
- `VISITING` → add presence + 15-min TTL. `NOT_VISITING` → remove presence.

---

## Data model (Postgres, per-schema)

**auth schema**
- `credentials(user_id uuid pk, login citext unique, email citext unique, password_hash text, created_at)`

**profiles schema**
- `profiles(user_id uuid pk, login citext, name, surname, email citext, phone, updated_at)`
- `pets(id uuid pk, user_id uuid, breed, name, sex char(1), is_castrated bool, age int)`
- `friendships(user_id uuid, friend_id uuid, created_at, pk(user_id,friend_id))`
- `friend_requests(id uuid pk, from_user_id, to_user_id, status, created_at)`
- `blocks(user_id uuid, blocked_user_id uuid, pk(user_id,blocked_user_id))`

**map schema**
- `map_objects(id uuid pk, object_type text, name text, location geography(Point,4326), source_osm_id bigint)`
  with `CREATE INDEX ... USING GIST (location);`

**Valkey**
- `session:{token} -> {user_id, exp}`
- `presence:{user_id} -> object_id` (EX 900)
- `object:{object_id}:visitors` (SET)
- `friends:{user_id}` (SET, cache of the friend graph)

## Map data seeding (OSM)

One-time / periodic import job: query Overpass API (or process a regional OSM
extract) for `leisure=park`, `leisure=dog_park`, and dog-friendly beaches; upsert
into `map_objects` keyed by `source_osm_id`. Read-only for MVP (no user edits).

## Open items / future

- Swap polling → WebSocket/SSE for push (keep the store abstraction on the client).
- Email verification + email-change flow.
- Rate limiting on Auth + friend requests.
- Real-time presence janitor via Valkey keyspace notifications.
