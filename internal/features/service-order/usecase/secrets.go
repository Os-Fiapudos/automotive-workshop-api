package usecase

import (
	"crypto/sha256"
	"crypto/subtle"
)

// secretMatches compares a presented shared secret with the configured one.
// Both are hashed before the constant-time comparison so neither the
// content nor the length of the secret leaks through timing. An empty
// configured or presented value never matches.
func secretMatches(configured, presented string) bool {
	if configured == "" || presented == "" {
		return false
	}
	expected := sha256.Sum256([]byte(configured))
	actual := sha256.Sum256([]byte(presented))
	return subtle.ConstantTimeCompare(expected[:], actual[:]) == 1
}
