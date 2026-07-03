-- Profiles service schema. Owns: profiles, pets, friendships, friend_requests,
-- blocks. Per the backend contract: one schema per service, string UUID ids,
-- citext for case-insensitive uniques, NO cross-schema foreign keys (so the
-- service stays independently deployable). Ids for users are minted by Auth and
-- arrive here via CreateProfile; we therefore do NOT FK pets/friendships/etc.
-- back to auth.credentials.

CREATE EXTENSION IF NOT EXISTS citext;

CREATE SCHEMA IF NOT EXISTS profiles;

-- One row per user. Seeded (empty) by CreateProfile on register; filled by
-- EditUser. login is immutable; email is copied from Auth and not editable here.
CREATE TABLE IF NOT EXISTS profiles.profiles (
    user_id    uuid PRIMARY KEY,
    login      citext NOT NULL,
    name       text   NOT NULL DEFAULT '',
    surname    text   NOT NULL DEFAULT '',
    email      citext NOT NULL DEFAULT '',
    phone      text   NOT NULL DEFAULT '',
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS profiles_login_uidx ON profiles.profiles (login);

-- 0..N pets per user. Replaced wholesale on EditUser.
CREATE TABLE IF NOT EXISTS profiles.pets (
    id           uuid PRIMARY KEY,
    user_id      uuid NOT NULL,
    breed        text NOT NULL DEFAULT '',
    name         text NOT NULL DEFAULT '',
    sex          char(1) NOT NULL DEFAULT 'M' CHECK (sex IN ('M', 'F')),
    is_castrated boolean NOT NULL DEFAULT false,
    age          int NOT NULL DEFAULT 0 CHECK (age >= 0)
);

CREATE INDEX IF NOT EXISTS pets_user_id_idx ON profiles.pets (user_id);

-- Friendship is stored in BOTH directions (two rows) so membership lookups and
-- the friends:{uid} cache rebuild are single-sided scans.
CREATE TABLE IF NOT EXISTS profiles.friendships (
    user_id    uuid NOT NULL,
    friend_id  uuid NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, friend_id)
);

CREATE INDEX IF NOT EXISTS friendships_friend_id_idx ON profiles.friendships (friend_id);

-- Pending / resolved friend requests. status: PENDING | ACCEPTED | DECLINED.
CREATE TABLE IF NOT EXISTS profiles.friend_requests (
    id           uuid PRIMARY KEY,
    from_user_id uuid NOT NULL,
    to_user_id   uuid NOT NULL,
    status       text NOT NULL DEFAULT 'PENDING' CHECK (status IN ('PENDING', 'ACCEPTED', 'DECLINED')),
    created_at   timestamptz NOT NULL DEFAULT now()
);

-- At most one PENDING request per ordered (from, to) pair.
CREATE UNIQUE INDEX IF NOT EXISTS friend_requests_pending_uidx
    ON profiles.friend_requests (from_user_id, to_user_id)
    WHERE status = 'PENDING';

CREATE INDEX IF NOT EXISTS friend_requests_to_pending_idx
    ON profiles.friend_requests (to_user_id) WHERE status = 'PENDING';

-- Directional block. A block by A on B prevents B from friend-requesting A and
-- hides presence in both directions (Map honors friends:{uid} which excludes
-- blocked users).
CREATE TABLE IF NOT EXISTS profiles.blocks (
    user_id         uuid NOT NULL,
    blocked_user_id uuid NOT NULL,
    created_at      timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, blocked_user_id)
);

CREATE INDEX IF NOT EXISTS blocks_blocked_user_id_idx ON profiles.blocks (blocked_user_id);
