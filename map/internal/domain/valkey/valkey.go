// Package valkey owns Map's ephemeral presence state. Presence is NEVER stored in
// Postgres — it lives here with a TTL so a user is dropped automatically when
// their app closes.
//
// Keys (from Docs/02-Backend.md):
//
//	presence:{user_id}            -> object_id, EX = presence TTL (15 min)
//	object:{object_id}:visitors   -> SET of user ids currently visiting
//	friends:{user_id}             -> SET, cached friend graph (owned by Profiles;
//	                                 Map only READS it for privacy filtering)
package valkey

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/valkey-io/valkey-go"
)

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

func presenceKey(user string) string   { return "presence:" + user }
func visitorsKey(object string) string { return fmt.Sprintf("object:%s:visitors", object) }
func friendsKey(user string) string    { return "friends:" + user }
func sessionKey(token string) string   { return "session:" + token }

// UserIDForToken resolves an opaque session token to the acting user id by
// reading session:{token} (owned by Auth: `{"user_id":..,"exp":..}`). Map only
// READS this key — it never issues or revokes sessions. Returns "" for an
// unknown/expired token. This is how Map derives the acting identity from the
// header instead of trusting the request body.
func (s *Store) UserIDForToken(ctx context.Context, token string) (string, error) {
	get := s.client.B().Get().Key(sessionKey(token)).Build()
	raw, err := s.client.Do(ctx, get).AsBytes()
	if valkey.IsValkeyNil(err) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("get session: %w", err)
	}
	var sess struct {
		UserID string `json:"user_id"`
	}
	if err := json.Unmarshal(raw, &sess); err != nil {
		return "", fmt.Errorf("decode session: %w", err)
	}
	return sess.UserID, nil
}

// MarkVisiting records the user as VISITING the object:
//
//	SADD object:{object}:visitors {user}
//	SET  presence:{user} {object} EX ttl
//
// If the user was already visiting a *different* object, that stale membership is
// removed first so a user is present in at most one object at a time.
func (s *Store) MarkVisiting(ctx context.Context, user, object string, ttl time.Duration) error {
	if prev, err := s.CurrentObject(ctx, user); err == nil && prev != "" && prev != object {
		_ = s.removeMembership(ctx, user, prev)
	}

	sadd := s.client.B().Sadd().Key(visitorsKey(object)).Member(user).Build()
	if err := s.client.Do(ctx, sadd).Error(); err != nil {
		return fmt.Errorf("sadd visitor: %w", err)
	}
	set := s.client.B().Set().Key(presenceKey(user)).Value(object).
		ExSeconds(int64(ttl.Seconds())).Build()
	if err := s.client.Do(ctx, set).Error(); err != nil {
		return fmt.Errorf("set presence: %w", err)
	}
	return nil
}

// MarkNotVisiting clears the user's presence for the object:
//
//	SREM object:{object}:visitors {user}
//	DEL  presence:{user}
func (s *Store) MarkNotVisiting(ctx context.Context, user, object string) error {
	if err := s.removeMembership(ctx, user, object); err != nil {
		return err
	}
	del := s.client.B().Del().Key(presenceKey(user)).Build()
	if err := s.client.Do(ctx, del).Error(); err != nil {
		return fmt.Errorf("del presence: %w", err)
	}
	return nil
}

func (s *Store) removeMembership(ctx context.Context, user, object string) error {
	srem := s.client.B().Srem().Key(visitorsKey(object)).Member(user).Build()
	if err := s.client.Do(ctx, srem).Error(); err != nil {
		return fmt.Errorf("srem visitor: %w", err)
	}
	return nil
}

// Heartbeat refreshes the presence TTL and re-ensures SADD membership, matching
// the client heartbeat (~every 2-3 min) described in the presence architecture.
// It is a no-op-safe way to keep a live user present without re-marking.
func (s *Store) Heartbeat(ctx context.Context, user, object string, ttl time.Duration) error {
	return s.MarkVisiting(ctx, user, object, ttl)
}

// CurrentObject returns the object the user currently holds presence in, or ""
// if none (their presence key has expired or was never set). This is how
// "on a walk" is derived — from a live key, never from stored state.
func (s *Store) CurrentObject(ctx context.Context, user string) (string, error) {
	get := s.client.B().Get().Key(presenceKey(user)).Build()
	res, err := s.client.Do(ctx, get).ToString()
	if valkey.IsValkeyNil(err) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("get presence: %w", err)
	}
	return res, nil
}

// VisitorCount returns SCARD object:{object}:visitors — shown to EVERYONE.
func (s *Store) VisitorCount(ctx context.Context, object string) (int, error) {
	scard := s.client.B().Scard().Key(visitorsKey(object)).Build()
	n, err := s.client.Do(ctx, scard).AsInt64()
	if err != nil {
		return 0, fmt.Errorf("scard visitors: %w", err)
	}
	return int(n), nil
}

// FriendIDsHere returns SINTER(object:{object}:visitors, friends:{caller}) — the
// caller's friends currently at the object. This is the ONLY visitor identity a
// client ever receives; the raw visitor set is never returned.
func (s *Store) FriendIDsHere(ctx context.Context, object, caller string) ([]string, error) {
	sinter := s.client.B().Sinter().Key(visitorsKey(object), friendsKey(caller)).Build()
	ids, err := s.client.Do(ctx, sinter).AsStrSlice()
	if err != nil {
		return nil, fmt.Errorf("sinter friends here: %w", err)
	}
	return ids, nil
}

// Visitors returns the raw visitor set for an object. INTERNAL USE ONLY (the
// janitor reconciles it); it must NEVER be surfaced to a client.
func (s *Store) Visitors(ctx context.Context, object string) ([]string, error) {
	smembers := s.client.B().Smembers().Key(visitorsKey(object)).Build()
	ids, err := s.client.Do(ctx, smembers).AsStrSlice()
	if err != nil {
		return nil, fmt.Errorf("smembers visitors: %w", err)
	}
	return ids, nil
}

// HasPresence reports whether presence:{user} is still live. Used by the janitor
// to decide whether a visitor-set membership is stale.
func (s *Store) HasPresence(ctx context.Context, user string) (bool, error) {
	exists := s.client.B().Exists().Key(presenceKey(user)).Build()
	n, err := s.client.Do(ctx, exists).AsInt64()
	if err != nil {
		return false, fmt.Errorf("exists presence: %w", err)
	}
	return n > 0, nil
}

// VisitorSetKeys returns all object:*:visitors keys. Used by the janitor to sweep
// every object's visitor set. (SCAN-based; fine for MVP volumes.)
func (s *Store) VisitorSetKeys(ctx context.Context) ([]string, error) {
	var (
		cursor uint64
		keys   []string
	)
	for {
		cmd := s.client.B().Scan().Cursor(cursor).Match("object:*:visitors").Count(256).Build()
		entry, err := s.client.Do(ctx, cmd).AsScanEntry()
		if err != nil {
			return nil, fmt.Errorf("scan visitor keys: %w", err)
		}
		keys = append(keys, entry.Elements...)
		cursor = entry.Cursor
		if cursor == 0 {
			break
		}
	}
	return keys, nil
}

// RemoveStaleVisitor removes a user from a specific object visitor set. Used by
// the janitor when presence:{user} has expired but the SET membership lingers.
func (s *Store) RemoveStaleVisitor(ctx context.Context, visitorsSetKey, user string) error {
	srem := s.client.B().Srem().Key(visitorsSetKey).Member(user).Build()
	if err := s.client.Do(ctx, srem).Error(); err != nil {
		return fmt.Errorf("srem stale visitor: %w", err)
	}
	return nil
}
