// Package valkey is the Profiles service's ephemeral state owner. It owns the
// `friends:{user_id}` SET cache that Map reads (via SINTER) for presence privacy
// filtering, and it *reads* `session:{token}` (a namespace owned by Auth) to
// resolve the acting user from the opaque session token.
package valkey

import (
	"context"
	"encoding/json"
	"fmt"

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

func friendsKey(userID string) string { return fmt.Sprintf("friends:%s", userID) }
func sessionKey(token string) string  { return fmt.Sprintf("session:%s", token) }

// session is the JSON stored by Auth at session:{token}. We only need user_id.
type session struct {
	UserID string `json:"user_id"`
}

// ResolveSession returns the user_id for an opaque session token, or "" if the
// token is missing/expired. Read-only: Profiles never writes sessions.
func (s *Store) ResolveSession(ctx context.Context, token string) (string, error) {
	cmd := s.client.B().Get().Key(sessionKey(token)).Build()
	raw, err := s.client.Do(ctx, cmd).ToString()
	if err != nil {
		if valkey.IsValkeyNil(err) {
			return "", nil
		}
		return "", err
	}
	var sess session
	if err := json.Unmarshal([]byte(raw), &sess); err != nil {
		return "", fmt.Errorf("valkey: bad session payload: %w", err)
	}
	return sess.UserID, nil
}

// SetFriends replaces the friends:{user_id} SET with exactly friendIDs. Done as
// DEL + SADD in a pipeline so Map never sees a partial set.
func (s *Store) SetFriends(ctx context.Context, userID string, friendIDs []string) error {
	del := s.client.B().Del().Key(friendsKey(userID)).Build()
	if len(friendIDs) == 0 {
		return s.client.Do(ctx, del).Error()
	}
	add := s.client.B().Sadd().Key(friendsKey(userID)).Member(friendIDs...).Build()
	for _, res := range s.client.DoMulti(ctx, del, add) {
		if err := res.Error(); err != nil {
			return err
		}
	}
	return nil
}

// Friends returns the cached friend set for a user (for tests / Map parity checks).
func (s *Store) Friends(ctx context.Context, userID string) ([]string, error) {
	cmd := s.client.B().Smembers().Key(friendsKey(userID)).Build()
	res, err := s.client.Do(ctx, cmd).AsStrSlice()
	if err != nil {
		if valkey.IsValkeyNil(err) {
			return nil, nil
		}
		return nil, err
	}
	return res, nil
}
