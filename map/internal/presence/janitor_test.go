package presence

import (
	"context"
	"testing"
)

type fakeCleaner struct {
	sets    map[string][]string // "object:{id}:visitors" -> members
	live    map[string]bool     // user -> presence key still live
	removed []removal
}

type removal struct{ setKey, user string }

func (f *fakeCleaner) VisitorSetKeys(context.Context) ([]string, error) {
	keys := make([]string, 0, len(f.sets))
	for k := range f.sets {
		keys = append(keys, k)
	}
	return keys, nil
}

func (f *fakeCleaner) Visitors(_ context.Context, object string) ([]string, error) {
	return f.sets["object:"+object+":visitors"], nil
}

func (f *fakeCleaner) HasPresence(_ context.Context, user string) (bool, error) {
	return f.live[user], nil
}

func (f *fakeCleaner) RemoveStaleVisitor(_ context.Context, setKey, user string) error {
	f.removed = append(f.removed, removal{setKey, user})
	// reflect the removal so a re-run is idempotent
	members := f.sets[setKey]
	kept := members[:0]
	for _, m := range members {
		if m != user {
			kept = append(kept, m)
		}
	}
	f.sets[setKey] = kept
	return nil
}

// The janitor drops members whose presence:{user} key has expired and keeps live ones.
func TestReconcile_RemovesOnlyStaleVisitors(t *testing.T) {
	f := &fakeCleaner{
		sets: map[string][]string{
			"object:park:visitors":  {"alice", "bob"}, // alice live, bob expired
			"object:beach:visitors": {"carol"},        // carol expired
		},
		live: map[string]bool{"alice": true},
	}
	j := NewJanitor(f, 0)

	if err := j.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	if len(f.removed) != 2 {
		t.Fatalf("removed %d, want 2 (bob, carol): %+v", len(f.removed), f.removed)
	}
	for _, r := range f.removed {
		if r.user == "alice" {
			t.Errorf("removed a LIVE visitor (alice)")
		}
	}
}

func TestReconcile_NoStaleIsNoOp(t *testing.T) {
	f := &fakeCleaner{
		sets: map[string][]string{"object:park:visitors": {"alice"}},
		live: map[string]bool{"alice": true},
	}
	j := NewJanitor(f, 0)
	if err := j.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if len(f.removed) != 0 {
		t.Errorf("removed %+v, want none", f.removed)
	}
}

func TestObjectIDFromKey(t *testing.T) {
	if got := objectIDFromKey("object:abc-123:visitors"); got != "abc-123" {
		t.Errorf("objectIDFromKey = %q, want abc-123", got)
	}
}
