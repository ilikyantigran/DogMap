// Package password hashes and verifies credentials with Argon2id (DogMap
// global convention 4: passwords are hashed with Argon2id, never stored or
// transmitted in plaintext beyond the TLS-protected register/login call).
//
// Hashes are self-describing PHC strings:
//
//	$argon2id$v=19$m=<mem>,t=<iters>,p=<par>$<b64salt>$<b64hash>
//
// so parameters can evolve without a migration — Verify reads them from the
// stored hash.
package password

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// Params tunes the Argon2id cost. Zero values fall back to Default.
type Params struct {
	Memory      uint32 // KiB
	Iterations  uint32
	Parallelism uint8
	SaltLen     uint32
	KeyLen      uint32
}

// Default is a sensible interactive-login cost (OWASP-ish): 64 MiB, 3 passes.
var Default = Params{
	Memory:      64 * 1024,
	Iterations:  3,
	Parallelism: 2,
	SaltLen:     16,
	KeyLen:      32,
}

// ErrMismatch is returned by Verify when the password does not match the hash.
var ErrMismatch = errors.New("password: hash mismatch")

// Hasher produces and verifies Argon2id hashes with a fixed set of params.
type Hasher struct{ p Params }

// NewHasher returns a Hasher, filling any zero-valued params from Default.
func NewHasher(p Params) *Hasher {
	if p.Memory == 0 {
		p.Memory = Default.Memory
	}
	if p.Iterations == 0 {
		p.Iterations = Default.Iterations
	}
	if p.Parallelism == 0 {
		p.Parallelism = Default.Parallelism
	}
	if p.SaltLen == 0 {
		p.SaltLen = Default.SaltLen
	}
	if p.KeyLen == 0 {
		p.KeyLen = Default.KeyLen
	}
	return &Hasher{p: p}
}

// Hash returns a PHC-encoded Argon2id hash of password using a fresh random salt.
func (h *Hasher) Hash(password string) (string, error) {
	salt := make([]byte, h.p.SaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("password: read salt: %w", err)
	}
	key := argon2.IDKey([]byte(password), salt, h.p.Iterations, h.p.Memory, h.p.Parallelism, h.p.KeyLen)
	return fmt.Sprintf(
		"$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, h.p.Memory, h.p.Iterations, h.p.Parallelism,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key),
	), nil
}

// Verify reports whether password matches the given PHC-encoded hash. The
// comparison is constant-time. It returns ErrMismatch on a clean non-match and a
// distinct error only if the encoded hash is malformed.
func Verify(password, encoded string) error {
	mem, iters, par, salt, want, err := decode(encoded)
	if err != nil {
		return err
	}
	got := argon2.IDKey([]byte(password), salt, iters, mem, par, uint32(len(want)))
	if subtle.ConstantTimeCompare(got, want) == 1 {
		return nil
	}
	return ErrMismatch
}

func decode(encoded string) (mem, iters uint32, par uint8, salt, hash []byte, err error) {
	parts := strings.Split(encoded, "$")
	// ["", "argon2id", "v=19", "m=..,t=..,p=..", salt, hash]
	if len(parts) != 6 || parts[1] != "argon2id" {
		return 0, 0, 0, nil, nil, errors.New("password: invalid hash format")
	}
	var version int
	if _, err = fmt.Sscanf(parts[2], "v=%d", &version); err != nil {
		return 0, 0, 0, nil, nil, errors.New("password: invalid version")
	}
	if version != argon2.Version {
		return 0, 0, 0, nil, nil, fmt.Errorf("password: unsupported version %d", version)
	}
	if _, err = fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &mem, &iters, &par); err != nil {
		return 0, 0, 0, nil, nil, errors.New("password: invalid params")
	}
	if salt, err = base64.RawStdEncoding.DecodeString(parts[4]); err != nil {
		return 0, 0, 0, nil, nil, errors.New("password: invalid salt")
	}
	if hash, err = base64.RawStdEncoding.DecodeString(parts[5]); err != nil {
		return 0, 0, 0, nil, nil, errors.New("password: invalid hash")
	}
	return mem, iters, par, salt, hash, nil
}
