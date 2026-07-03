package postgres

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Integration tests for the real Postgres store. They run against a live
// database only when PROFILES_TEST_DSN is set, e.g.:
//
//	PROFILES_TEST_DSN='postgres://postgres:postgres@localhost:5432/dogmap?sslmode=disable' \
//	  go test ./internal/domain/postgres/...
//
// Without it they skip, so `go test ./...` stays green on machines with no DB.
// The test applies the 0001 migration, then exercises the queries that matter:
// idempotent CreateProfile, pet replacement, the two-directional friend graph,
// the pending-request rules, and the block cascade.

func testStore(t *testing.T) *Store {
	t.Helper()
	dsn := os.Getenv("PROFILES_TEST_DSN")
	if dsn == "" {
		t.Skip("PROFILES_TEST_DSN not set; skipping Postgres integration test")
	}
	ctx := context.Background()

	// Apply the migration (up) fresh.
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	for _, name := range []string{"0001_init.down.sql", "0001_init.up.sql"} {
		sql, err := os.ReadFile(filepath.Join("migrations", name))
		if err != nil {
			t.Fatalf("read migration %s: %v", name, err)
		}
		if _, err := pool.Exec(ctx, string(sql)); err != nil {
			t.Fatalf("apply %s: %v", name, err)
		}
	}
	pool.Close()

	s, err := NewStore(ctx, dsn)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	t.Cleanup(s.Close)
	return s
}

func TestIntegration_CreateProfileIdempotent(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	id := uuid.NewString()

	if err := s.CreateProfile(ctx, id, "Alice", "a@x.io"); err != nil {
		t.Fatal(err)
	}
	// Retry with different values must NOT clobber the seeded row.
	if err := s.CreateProfile(ctx, id, "Alice", "other@x.io"); err != nil {
		t.Fatal(err)
	}
	p, err := s.GetProfile(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if p.Login != "Alice" || p.Email != "a@x.io" {
		t.Fatalf("idempotent create clobbered row: %+v", p)
	}
}

func TestIntegration_UpdateProfileReplacesPets(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	id := uuid.NewString()
	if err := s.CreateProfile(ctx, id, "Bob", "b@x.io"); err != nil {
		t.Fatal(err)
	}
	pets := []Pet{{Breed: "Poodle", Name: "Bruno", Sex: "M", IsCastrated: true, Age: 3}}
	if err := s.UpdateProfile(ctx, id, "Bob", "Jones", "+100", pets); err != nil {
		t.Fatal(err)
	}
	p, _ := s.GetProfile(ctx, id)
	if p.Name != "Bob" || p.Surname != "Jones" || len(p.Pets) != 1 || p.Pets[0].Name != "Bruno" {
		t.Fatalf("update wrong: %+v", p)
	}
	// Replace with an empty set.
	if err := s.UpdateProfile(ctx, id, "Bob", "Jones", "+100", nil); err != nil {
		t.Fatal(err)
	}
	p, _ = s.GetProfile(ctx, id)
	if len(p.Pets) != 0 {
		t.Fatalf("pets not replaced: %+v", p.Pets)
	}
}

func TestIntegration_FriendGraphAndBlockCascade(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	a, b := uuid.NewString(), uuid.NewString()
	if err := s.CreateProfile(ctx, a, "A", "a2@x.io"); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateProfile(ctx, b, "B", "b2@x.io"); err != nil {
		t.Fatal(err)
	}

	// a → b request, then b accepts.
	reqID, err := s.CreateFriendRequest(ctx, a, b)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.PendingBetween(ctx, a, b); err != nil {
		t.Fatalf("pending not found: %v", err)
	}
	if err := s.AcceptFriendRequest(ctx, reqID, a, b); err != nil {
		t.Fatal(err)
	}

	// Friendship exists both ways.
	if ok, _ := s.AreFriends(ctx, a, b); !ok {
		t.Fatal("a→b friendship missing")
	}
	if ok, _ := s.AreFriends(ctx, b, a); !ok {
		t.Fatal("b→a friendship missing")
	}
	ids, _ := s.FriendIDs(ctx, a)
	if len(ids) != 1 || ids[0] != b {
		t.Fatalf("FriendIDs(a) = %v", ids)
	}

	// a blocks b → friendship gone both ways, block recorded, future request blocked.
	if err := s.Block(ctx, a, b); err != nil {
		t.Fatal(err)
	}
	if ok, _ := s.AreFriends(ctx, a, b); ok {
		t.Fatal("block did not remove a→b friendship")
	}
	if ok, _ := s.AreFriends(ctx, b, a); ok {
		t.Fatal("block did not remove b→a friendship")
	}
	if ok, _ := s.IsBlockedEitherWay(ctx, a, b); !ok {
		t.Fatal("block not recorded")
	}

	// Unblock clears the block.
	if err := s.Unblock(ctx, a, b); err != nil {
		t.Fatal(err)
	}
	if ok, _ := s.IsBlockedEitherWay(ctx, a, b); ok {
		t.Fatal("unblock failed")
	}
}

func TestIntegration_PendingRequestCancelledByBlock(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	a, b := uuid.NewString(), uuid.NewString()
	_ = s.CreateProfile(ctx, a, "A3", "a3@x.io")
	_ = s.CreateProfile(ctx, b, "B3", "b3@x.io")

	if _, err := s.CreateFriendRequest(ctx, a, b); err != nil {
		t.Fatal(err)
	}
	if err := s.Block(ctx, b, a); err != nil {
		t.Fatal(err)
	}
	// Pending request must be gone after the block cascade.
	if _, err := s.PendingBetween(ctx, a, b); err == nil {
		t.Fatal("pending request should be cancelled by block")
	}
}
