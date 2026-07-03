// Package postgres is the Profiles service's long-term state owner: it owns the
// `profiles` schema (profiles, pets, friendships, friend_requests, blocks) and
// is the sole place that touches the database driver. Handlers depend on the
// narrow, intent-named methods here, never on pgx directly.
//
// Friendships are stored in BOTH directions (two rows) so the friends:{uid}
// cache and membership checks are single-sided lookups.
package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Pet mirrors a row in profiles.pets (without the surrogate id, which the store
// owns).
type Pet struct {
	Breed       string
	Name        string
	Sex         string // "M" | "F"
	IsCastrated bool
	Age         int32
}

// Profile mirrors a row in profiles.profiles plus its pets.
type Profile struct {
	UserID  string
	Login   string
	Name    string
	Surname string
	Email   string
	Phone   string
	Pets    []Pet
}

// FriendRequest is a row of profiles.friend_requests.
type FriendRequest struct {
	ID         string
	FromUserID string
	ToUserID   string
	Status     string // PENDING | ACCEPTED | DECLINED
}

// FriendRef is a friend user_id + login for ListFriends.
type FriendRef struct {
	UserID string
	Login  string
}

var ErrNotFound = errors.New("profiles: not found")

type Store struct {
	pool *pgxpool.Pool
}

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

// CreateProfile idempotently seeds an empty profile. On conflict it does
// nothing (retry-safe) — this is the Auth→Profiles handoff.
func (s *Store) CreateProfile(ctx context.Context, userID, login, email string) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO profiles.profiles (user_id, login, email)
		VALUES ($1, $2, $3)
		ON CONFLICT (user_id) DO NOTHING`,
		userID, login, email)
	if err != nil {
		return fmt.Errorf("postgres: create profile: %w", err)
	}
	return nil
}

// GetProfile returns the profile row + pets, or ErrNotFound.
func (s *Store) GetProfile(ctx context.Context, userID string) (*Profile, error) {
	var p Profile
	err := s.pool.QueryRow(ctx, `
		SELECT user_id, login, name, surname, email, phone
		FROM profiles.profiles WHERE user_id = $1`, userID).
		Scan(&p.UserID, &p.Login, &p.Name, &p.Surname, &p.Email, &p.Phone)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("postgres: get profile: %w", err)
	}

	rows, err := s.pool.Query(ctx, `
		SELECT breed, name, sex, is_castrated, age
		FROM profiles.pets WHERE user_id = $1 ORDER BY name`, userID)
	if err != nil {
		return nil, fmt.Errorf("postgres: get pets: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var pet Pet
		if err := rows.Scan(&pet.Breed, &pet.Name, &pet.Sex, &pet.IsCastrated, &pet.Age); err != nil {
			return nil, fmt.Errorf("postgres: scan pet: %w", err)
		}
		p.Pets = append(p.Pets, pet)
	}
	return &p, rows.Err()
}

// LoginFor returns the login for a user id (used for ListFriends projections).
func (s *Store) LoginFor(ctx context.Context, userID string) (string, error) {
	var login string
	err := s.pool.QueryRow(ctx, `SELECT login FROM profiles.profiles WHERE user_id = $1`, userID).Scan(&login)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	return login, err
}

// UpdateProfile updates name/surname/phone and replaces the pet set atomically.
// login and email are intentionally NOT updatable here.
func (s *Store) UpdateProfile(ctx context.Context, userID, name, surname, phone string, pets []Pet) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	ct, err := tx.Exec(ctx, `
		UPDATE profiles.profiles
		SET name = $2, surname = $3, phone = $4, updated_at = now()
		WHERE user_id = $1`, userID, name, surname, phone)
	if err != nil {
		return fmt.Errorf("postgres: update profile: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}

	if _, err := tx.Exec(ctx, `DELETE FROM profiles.pets WHERE user_id = $1`, userID); err != nil {
		return fmt.Errorf("postgres: clear pets: %w", err)
	}
	for _, pet := range pets {
		if _, err := tx.Exec(ctx, `
			INSERT INTO profiles.pets (id, user_id, breed, name, sex, is_castrated, age)
			VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			uuid.NewString(), userID, pet.Breed, pet.Name, pet.Sex, pet.IsCastrated, pet.Age); err != nil {
			return fmt.Errorf("postgres: insert pet: %w", err)
		}
	}
	return tx.Commit(ctx)
}

// --- friend graph queries ---

// AreFriends reports whether a friendship row a→b exists.
func (s *Store) AreFriends(ctx context.Context, a, b string) (bool, error) {
	var exists bool
	err := s.pool.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM profiles.friendships WHERE user_id = $1 AND friend_id = $2)`,
		a, b).Scan(&exists)
	return exists, err
}

// IsBlockedEitherWay reports whether a blocked b or b blocked a.
func (s *Store) IsBlockedEitherWay(ctx context.Context, a, b string) (bool, error) {
	var exists bool
	err := s.pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM profiles.blocks
			WHERE (user_id = $1 AND blocked_user_id = $2)
			   OR (user_id = $2 AND blocked_user_id = $1))`,
		a, b).Scan(&exists)
	return exists, err
}

// HasBlocked reports whether blocker has blocked blocked.
func (s *Store) HasBlocked(ctx context.Context, blocker, blocked string) (bool, error) {
	var exists bool
	err := s.pool.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM profiles.blocks WHERE user_id = $1 AND blocked_user_id = $2)`,
		blocker, blocked).Scan(&exists)
	return exists, err
}

// PendingBetween returns a pending request in either direction between a and b,
// or ErrNotFound.
func (s *Store) PendingBetween(ctx context.Context, a, b string) (*FriendRequest, error) {
	var fr FriendRequest
	err := s.pool.QueryRow(ctx, `
		SELECT id, from_user_id, to_user_id, status
		FROM profiles.friend_requests
		WHERE status = 'PENDING'
		  AND ((from_user_id = $1 AND to_user_id = $2) OR (from_user_id = $2 AND to_user_id = $1))
		LIMIT 1`, a, b).
		Scan(&fr.ID, &fr.FromUserID, &fr.ToUserID, &fr.Status)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &fr, nil
}

// CreateFriendRequest inserts a PENDING request and returns its id.
func (s *Store) CreateFriendRequest(ctx context.Context, from, to string) (string, error) {
	id := uuid.NewString()
	_, err := s.pool.Exec(ctx, `
		INSERT INTO profiles.friend_requests (id, from_user_id, to_user_id, status)
		VALUES ($1, $2, $3, 'PENDING')`, id, from, to)
	if err != nil {
		return "", fmt.Errorf("postgres: create friend request: %w", err)
	}
	return id, nil
}

// GetPendingRequest fetches a PENDING request by id, or ErrNotFound.
func (s *Store) GetPendingRequest(ctx context.Context, id string) (*FriendRequest, error) {
	var fr FriendRequest
	err := s.pool.QueryRow(ctx, `
		SELECT id, from_user_id, to_user_id, status
		FROM profiles.friend_requests WHERE id = $1 AND status = 'PENDING'`, id).
		Scan(&fr.ID, &fr.FromUserID, &fr.ToUserID, &fr.Status)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &fr, nil
}

// AcceptFriendRequest marks the request ACCEPTED and inserts both friendship
// directions, atomically.
func (s *Store) AcceptFriendRequest(ctx context.Context, id, a, b string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `
		UPDATE profiles.friend_requests SET status = 'ACCEPTED' WHERE id = $1`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO profiles.friendships (user_id, friend_id) VALUES ($1, $2), ($2, $1)
		ON CONFLICT DO NOTHING`, a, b); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// DeclineFriendRequest marks a request DECLINED.
func (s *Store) DeclineFriendRequest(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE profiles.friend_requests SET status = 'DECLINED' WHERE id = $1`, id)
	return err
}

// RemoveFriendship deletes both friendship directions between a and b.
func (s *Store) RemoveFriendship(ctx context.Context, a, b string) error {
	_, err := s.pool.Exec(ctx, `
		DELETE FROM profiles.friendships
		WHERE (user_id = $1 AND friend_id = $2) OR (user_id = $2 AND friend_id = $1)`, a, b)
	return err
}

// Block records blocker→blocked and cascades: removes any friendship (both
// directions) and cancels any pending requests between the two.
func (s *Store) Block(ctx context.Context, blocker, blocked string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `
		INSERT INTO profiles.blocks (user_id, blocked_user_id) VALUES ($1, $2)
		ON CONFLICT DO NOTHING`, blocker, blocked); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		DELETE FROM profiles.friendships
		WHERE (user_id = $1 AND friend_id = $2) OR (user_id = $2 AND friend_id = $1)`,
		blocker, blocked); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE profiles.friend_requests SET status = 'DECLINED'
		WHERE status = 'PENDING'
		  AND ((from_user_id = $1 AND to_user_id = $2) OR (from_user_id = $2 AND to_user_id = $1))`,
		blocker, blocked); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// Unblock removes blocker→blocked.
func (s *Store) Unblock(ctx context.Context, blocker, blocked string) error {
	_, err := s.pool.Exec(ctx, `
		DELETE FROM profiles.blocks WHERE user_id = $1 AND blocked_user_id = $2`, blocker, blocked)
	return err
}

// FriendIDs returns the user's friend ids (used to rebuild friends:{uid}).
func (s *Store) FriendIDs(ctx context.Context, userID string) ([]string, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT friend_id FROM profiles.friendships WHERE user_id = $1`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// Friends returns friend refs (id + login) for ListFriends.
func (s *Store) Friends(ctx context.Context, userID string) ([]FriendRef, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT f.friend_id, p.login
		FROM profiles.friendships f
		JOIN profiles.profiles p ON p.user_id = f.friend_id
		WHERE f.user_id = $1
		ORDER BY p.login`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []FriendRef
	for rows.Next() {
		var fr FriendRef
		if err := rows.Scan(&fr.UserID, &fr.Login); err != nil {
			return nil, err
		}
		out = append(out, fr)
	}
	return out, rows.Err()
}

// IncomingPending returns pending requests addressed to userID (with from_login).
func (s *Store) IncomingPending(ctx context.Context, userID string) ([]FriendRequest, map[string]string, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT fr.id, fr.from_user_id, fr.to_user_id, p.login
		FROM profiles.friend_requests fr
		LEFT JOIN profiles.profiles p ON p.user_id = fr.from_user_id
		WHERE fr.to_user_id = $1 AND fr.status = 'PENDING'
		ORDER BY fr.created_at`, userID)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	var out []FriendRequest
	logins := map[string]string{}
	for rows.Next() {
		var fr FriendRequest
		var login *string
		if err := rows.Scan(&fr.ID, &fr.FromUserID, &fr.ToUserID, &login); err != nil {
			return nil, nil, err
		}
		fr.Status = "PENDING"
		if login != nil {
			logins[fr.FromUserID] = *login
		}
		out = append(out, fr)
	}
	return out, logins, rows.Err()
}

// OutgoingPending returns pending requests sent by userID.
func (s *Store) OutgoingPending(ctx context.Context, userID string) ([]FriendRequest, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, from_user_id, to_user_id
		FROM profiles.friend_requests
		WHERE from_user_id = $1 AND status = 'PENDING'
		ORDER BY created_at`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []FriendRequest
	for rows.Next() {
		var fr FriendRequest
		if err := rows.Scan(&fr.ID, &fr.FromUserID, &fr.ToUserID); err != nil {
			return nil, err
		}
		fr.Status = "PENDING"
		out = append(out, fr)
	}
	return out, rows.Err()
}
