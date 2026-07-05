// Package valkey is Auth's session-state owner. Sessions are opaque tokens
// (DogMap global convention 2): session:{token} -> {user_id, exp}, sent as the
// auth_token header, sliding TTL (~24h). Logout deletes the key → instant
// revocation. Nothing here is stored in Postgres.
package valkey

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"

	"github.com/valkey-io/valkey-go"
)

// sessionData is the JSON value stored at session:{token}. The exp is advisory
// (the Valkey TTL is authoritative for revocation); it's kept so callers can see
// the absolute expiry without a separate TTL round-trip.
type sessionData struct {
	UserID string `json:"user_id"`
	Exp    int64  `json:"exp"` // unix seconds
}

// Store owns Auth's session keys in Valkey.
type Store struct {
	client valkey.Client
}

func NewStore(address string) (*Store, error) {
	client, err := valkey.NewClient(valkey.ClientOption{InitAddress: []string{address}})
	if err != nil {
		return nil, err
	}
	return &Store{client: client}, nil
}

func (s *Store) Close() { s.client.Close() }

func sessionKey(token string) string { return "session:" + token }

func verifyKey(token string) string { return "verify:" + token }

// NewToken returns a fresh, high-entropy, URL-safe opaque token. Opaque means it
// carries no claims — the mapping to a user lives only in Valkey.
func NewToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("valkey: read token entropy: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// CreateSession stores session:{token} -> {user_id, exp} with the given TTL and
// returns the token.
func (s *Store) CreateSession(ctx context.Context, userID string, ttl time.Duration) (string, error) {
	token, err := NewToken()
	if err != nil {
		return "", err
	}
	payload, err := json.Marshal(sessionData{
		UserID: userID,
		Exp:    time.Now().Add(ttl).Unix(),
	})
	if err != nil {
		return "", fmt.Errorf("valkey: marshal session: %w", err)
	}
	cmd := s.client.B().Set().Key(sessionKey(token)).Value(string(payload)).
		ExSeconds(int64(ttl.Seconds())).Build()
	if err := s.client.Do(ctx, cmd).Error(); err != nil {
		return "", fmt.Errorf("valkey: create session: %w", err)
	}
	return token, nil
}

// Lookup returns the user id for a token and refreshes the sliding TTL. It
// returns ("", false, nil) for an unknown/expired token — callers treat that as
// unauthenticated, not an error.
func (s *Store) Lookup(ctx context.Context, token string, ttl time.Duration) (string, bool, error) {
	if token == "" {
		return "", false, nil
	}
	raw, err := s.client.Do(ctx, s.client.B().Get().Key(sessionKey(token)).Build()).ToString()
	if err != nil {
		if valkey.IsValkeyNil(err) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("valkey: lookup session: %w", err)
	}
	var data sessionData
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		return "", false, fmt.Errorf("valkey: unmarshal session: %w", err)
	}
	// Sliding TTL: extend the window on use (best effort).
	_ = s.client.Do(ctx, s.client.B().Expire().Key(sessionKey(token)).
		Seconds(int64(ttl.Seconds())).Build()).Error()
	return data.UserID, true, nil
}

// DeleteSession removes session:{token} → the token is instantly unusable. It is
// idempotent (deleting a missing key is a no-op).
func (s *Store) DeleteSession(ctx context.Context, token string) error {
	if token == "" {
		return nil
	}
	return s.client.Do(ctx, s.client.B().Del().Key(sessionKey(token)).Build()).Error()
}

// --- Email-verification tokens (verify:{token} -> user_id) -------------------
//
// A separate keyspace from sessions: single-use, ~24h TTL, carries no claims.
// Register mints one; VerifyEmail consumes it (GET then DEL). Once consumed or
// expired the link is dead.

// CreateVerifyToken stores verify:{token} -> user_id with the given TTL and
// returns the opaque token to embed in the confirmation link.
func (s *Store) CreateVerifyToken(ctx context.Context, userID string, ttl time.Duration) (string, error) {
	token, err := NewToken()
	if err != nil {
		return "", err
	}
	cmd := s.client.B().Set().Key(verifyKey(token)).Value(userID).
		ExSeconds(int64(ttl.Seconds())).Build()
	if err := s.client.Do(ctx, cmd).Error(); err != nil {
		return "", fmt.Errorf("valkey: create verify token: %w", err)
	}
	return token, nil
}

// ConsumeVerifyToken looks up the user id for a verification token and deletes
// the key (single-use). It returns ("", false, nil) for an unknown/expired
// token — callers treat that as an invalid link, not an error.
func (s *Store) ConsumeVerifyToken(ctx context.Context, token string) (string, bool, error) {
	if token == "" {
		return "", false, nil
	}
	userID, err := s.client.Do(ctx, s.client.B().Get().Key(verifyKey(token)).Build()).ToString()
	if err != nil {
		if valkey.IsValkeyNil(err) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("valkey: consume verify token: %w", err)
	}
	// Burn the token so the link can't be replayed. Best effort: the account is
	// still marked verified even if the delete races.
	_ = s.client.Do(ctx, s.client.B().Del().Key(verifyKey(token)).Build()).Error()
	return userID, true, nil
}
