package valkey

import (
	"encoding/base64"
	"testing"
)

// NewToken is the pure, testable-without-Valkey part of the session store: it
// must produce high-entropy, URL-safe, unique opaque tokens (opaque = no claims).
func TestNewToken_UniqueURLSafe(t *testing.T) {
	seen := make(map[string]struct{}, 1000)
	for i := 0; i < 1000; i++ {
		tok, err := NewToken()
		if err != nil {
			t.Fatalf("NewToken: %v", err)
		}
		if tok == "" {
			t.Fatal("empty token")
		}
		// 32 random bytes → 43 chars of raw-url base64
		if _, err := base64.RawURLEncoding.DecodeString(tok); err != nil {
			t.Fatalf("token not url-safe base64: %q (%v)", tok, err)
		}
		if len(tok) < 40 {
			t.Fatalf("token too short (low entropy?): %q", tok)
		}
		if _, dup := seen[tok]; dup {
			t.Fatalf("duplicate token generated: %q", tok)
		}
		seen[tok] = struct{}{}
	}
}

func TestSessionKey(t *testing.T) {
	if got := sessionKey("abc"); got != "session:abc" {
		t.Fatalf("sessionKey = %q, want session:abc", got)
	}
}
