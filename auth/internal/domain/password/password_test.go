package password

import (
	"strings"
	"testing"
)

func TestHashVerifyRoundTrip(t *testing.T) {
	h := NewHasher(Params{}) // defaults
	encoded, err := h.Hash("correct horse battery staple")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if !strings.HasPrefix(encoded, "$argon2id$") {
		t.Fatalf("expected argon2id PHC string, got %q", encoded)
	}
	if strings.Contains(encoded, "correct horse") {
		t.Fatal("hash must not contain the plaintext password")
	}
	if err := Verify("correct horse battery staple", encoded); err != nil {
		t.Fatalf("verify should succeed: %v", err)
	}
}

func TestVerifyWrongPassword(t *testing.T) {
	h := NewHasher(Params{})
	encoded, err := h.Hash("s3cret-password")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if err := Verify("wrong-password", encoded); err != ErrMismatch {
		t.Fatalf("expected ErrMismatch, got %v", err)
	}
}

func TestHashesAreSaltedUnique(t *testing.T) {
	h := NewHasher(Params{})
	a, _ := h.Hash("same-password")
	b, _ := h.Hash("same-password")
	if a == b {
		t.Fatal("two hashes of the same password must differ (random salt)")
	}
	// both must still verify
	if err := Verify("same-password", a); err != nil {
		t.Fatalf("verify a: %v", err)
	}
	if err := Verify("same-password", b); err != nil {
		t.Fatalf("verify b: %v", err)
	}
}

func TestVerifyMalformedHash(t *testing.T) {
	cases := []string{
		"",
		"not-a-hash",
		"$argon2id$v=19$m=65536,t=3,p=2$badsalt", // too few fields
		"$bcrypt$v=19$m=65536,t=3,p=2$c2FsdA$aGFzaA",
	}
	for _, c := range cases {
		if err := Verify("x", c); err == nil {
			t.Fatalf("expected error for malformed hash %q", c)
		}
	}
}

func TestNewHasherFillsDefaults(t *testing.T) {
	h := NewHasher(Params{Iterations: 1})
	if h.p.Memory != Default.Memory || h.p.Parallelism != Default.Parallelism {
		t.Fatalf("zero params should fall back to Default, got %+v", h.p)
	}
	if h.p.Iterations != 1 {
		t.Fatalf("explicit param should be kept, got %d", h.p.Iterations)
	}
}
