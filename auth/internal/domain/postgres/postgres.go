// Package postgres is Auth's long-term-storage owner. It owns exactly one table
// in the `auth` schema — credentials(user_id, login, email, password_hash,
// created_at) — and nothing else: no cross-schema foreign keys, per the DogMap
// topology rule (one schema per service, independently deployable).
package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrDuplicate is returned by InsertCredential when the login or email already
// exists (unique violation). The caller maps it to the "already taken" edge error
// without leaking which field collided beyond what Register needs.
var ErrDuplicate = errors.New("postgres: login or email already exists")

// ErrNotFound is returned by FindByLoginOrEmail when no credential matches.
var ErrNotFound = errors.New("postgres: credential not found")

// Credential is a row of the auth.credentials table.
type Credential struct {
	UserID        string
	Login         string
	Email         string
	PasswordHash  string
	EmailVerified bool // set true once the user follows the emailed verify link
}

// Store wraps a pgx pool scoped to the auth schema.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore opens a connection pool from a Postgres DSN.
func NewStore(ctx context.Context, dsn string) (*Store, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("postgres: connect: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("postgres: ping: %w", err)
	}
	return &Store{pool: pool}, nil
}

func (s *Store) Close() { s.pool.Close() }

// InsertCredential inserts a new credential row. Returns ErrDuplicate if the
// login or email is already taken (unique constraint). Ids are string UUIDs
// minted by the caller.
func (s *Store) InsertCredential(ctx context.Context, c Credential) error {
	const q = `
		INSERT INTO auth.credentials (user_id, login, email, password_hash)
		VALUES ($1, $2, $3, $4)`
	_, err := s.pool.Exec(ctx, q, c.UserID, c.Login, c.Email, c.PasswordHash)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" { // unique_violation
			return ErrDuplicate
		}
		return fmt.Errorf("postgres: insert credential: %w", err)
	}
	return nil
}

// FindByLoginOrEmail looks up a credential by login OR email (citext columns are
// case-insensitive). Either argument may be empty; at least one should be set.
// Returns ErrNotFound when nothing matches.
func (s *Store) FindByLoginOrEmail(ctx context.Context, login, email string) (Credential, error) {
	const q = `
		SELECT user_id, login, email, password_hash, email_verified
		FROM auth.credentials
		WHERE ($1 <> '' AND login = $1) OR ($2 <> '' AND email = $2)
		LIMIT 1`
	var c Credential
	err := s.pool.QueryRow(ctx, q, login, email).
		Scan(&c.UserID, &c.Login, &c.Email, &c.PasswordHash, &c.EmailVerified)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Credential{}, ErrNotFound
		}
		return Credential{}, fmt.Errorf("postgres: find credential: %w", err)
	}
	return c, nil
}

// FindByEmail looks up a credential by email only (citext, case-insensitive).
// Used by ResendVerification. Returns ErrNotFound when nothing matches.
func (s *Store) FindByEmail(ctx context.Context, email string) (Credential, error) {
	const q = `
		SELECT user_id, login, email, password_hash, email_verified
		FROM auth.credentials
		WHERE email = $1
		LIMIT 1`
	var c Credential
	err := s.pool.QueryRow(ctx, q, email).
		Scan(&c.UserID, &c.Login, &c.Email, &c.PasswordHash, &c.EmailVerified)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Credential{}, ErrNotFound
		}
		return Credential{}, fmt.Errorf("postgres: find by email: %w", err)
	}
	return c, nil
}

// MarkEmailVerified flips email_verified to true for the given user. Idempotent:
// re-marking an already-verified user is a harmless no-op update.
func (s *Store) MarkEmailVerified(ctx context.Context, userID string) error {
	const q = `UPDATE auth.credentials SET email_verified = true WHERE user_id = $1`
	if _, err := s.pool.Exec(ctx, q, userID); err != nil {
		return fmt.Errorf("postgres: mark email verified: %w", err)
	}
	return nil
}
