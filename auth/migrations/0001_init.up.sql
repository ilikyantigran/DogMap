-- Auth service schema. Owns credentials only (login, email, password hash,
-- user_id). No cross-schema foreign keys — a service must stay independently
-- deployable (DogMap topology rule). Ids are string UUIDs everywhere.

CREATE SCHEMA IF NOT EXISTS auth;

-- citext gives case-insensitive uniqueness for login and email so "Test1" and
-- "test1" (and mixed-case emails) can't both register.
CREATE EXTENSION IF NOT EXISTS citext;

CREATE TABLE IF NOT EXISTS auth.credentials (
    user_id       uuid        PRIMARY KEY,
    login         citext      NOT NULL UNIQUE,
    email         citext      NOT NULL UNIQUE,
    password_hash text        NOT NULL,          -- Argon2id PHC string; never plaintext
    created_at    timestamptz NOT NULL DEFAULT now()
);
