// Package presence contains the presence janitor: a background reconciler that
// keeps visitor counts honest.
//
// Valkey sets do not auto-expire their members, so when presence:{user} lapses
// (TTL expiry, or the app closed without sending NOT_VISITING) the user can
// linger in object:{id}:visitors and inflate visitor_count. The janitor sweeps
// every object:*:visitors set and drops any member whose presence key is gone.
//
// This is the polling implementation for MVP. Docs/02-Backend.md notes a future
// real-time variant driven by Valkey keyspace notifications; the Reconcile logic
// is identical, only the trigger changes.
package presence

import (
	"context"
	"log/slog"
	"strings"
	"time"
)

// Cleaner is the narrow slice of the Valkey store the janitor needs.
type Cleaner interface {
	VisitorSetKeys(ctx context.Context) ([]string, error)
	Visitors(ctx context.Context, object string) ([]string, error)
	HasPresence(ctx context.Context, user string) (bool, error)
	RemoveStaleVisitor(ctx context.Context, visitorsSetKey, user string) error
}

// Janitor periodically reconciles visitor sets against live presence keys.
type Janitor struct {
	store    Cleaner
	interval time.Duration
}

func NewJanitor(store Cleaner, interval time.Duration) *Janitor {
	if interval <= 0 {
		interval = time.Minute
	}
	return &Janitor{store: store, interval: interval}
}

// Run reconciles on a ticker until ctx is cancelled. Intended to run in a
// goroutine owned by App; returns when ctx is done.
func (j *Janitor) Run(ctx context.Context) {
	ticker := time.NewTicker(j.interval)
	defer ticker.Stop()

	slog.Info("presence janitor started", "interval", j.interval.String())
	for {
		select {
		case <-ctx.Done():
			slog.Info("presence janitor stopped")
			return
		case <-ticker.C:
			if err := j.Reconcile(ctx); err != nil {
				slog.Error("presence reconcile failed", "err", err)
			}
		}
	}
}

// Reconcile makes one full pass: for every object:*:visitors set, remove any
// member whose presence:{user} key has expired. Returns the first error but
// keeps going through the rest of the sweep so one bad key can't stall cleanup.
func (j *Janitor) Reconcile(ctx context.Context) error {
	keys, err := j.store.VisitorSetKeys(ctx)
	if err != nil {
		return err
	}

	var firstErr error
	removed := 0
	for _, setKey := range keys {
		object := objectIDFromKey(setKey)
		visitors, err := j.store.Visitors(ctx, object)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		for _, user := range visitors {
			live, err := j.store.HasPresence(ctx, user)
			if err != nil {
				if firstErr == nil {
					firstErr = err
				}
				continue
			}
			if live {
				continue
			}
			if err := j.store.RemoveStaleVisitor(ctx, setKey, user); err != nil {
				if firstErr == nil {
					firstErr = err
				}
				continue
			}
			removed++
		}
	}
	if removed > 0 {
		slog.Info("presence janitor reconciled", "removed", removed, "sets", len(keys))
	}
	return firstErr
}

// objectIDFromKey extracts {id} from "object:{id}:visitors".
func objectIDFromKey(setKey string) string {
	s := strings.TrimPrefix(setKey, "object:")
	return strings.TrimSuffix(s, ":visitors")
}
