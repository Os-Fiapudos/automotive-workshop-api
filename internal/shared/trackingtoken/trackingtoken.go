package trackingtoken

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
)

// rawByteLength is the amount of entropy (256 bits) packed into every
// generated token — enough that guessing one is infeasible
// (specs/service-order-tracking/requirements.md §3.2).
const rawByteLength = 32

// Generate returns a new opaque, high-entropy token: rawByteLength random
// bytes from crypto/rand, hex-encoded. The caller receives this raw value
// exactly once; only its Hash is ever persisted.
func Generate() (string, error) {
	buf := make([]byte, rawByteLength)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

// Hash returns the hex-encoded SHA-256 hash of raw, used for storage and
// lookup so the raw token itself never touches the database
// (specs/service-order-tracking/requirements.md §0 item 4).
func Hash(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}
