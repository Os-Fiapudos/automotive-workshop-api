package config

import (
	"fmt"
	"os"
	"time"
)

const defaultJWTTTL = time.Hour

// minJWTSecretLength is the minimum byte length accepted for JWT_SECRET. A
// short/trivial secret undermines HS256 signing regardless of everything
// else being correct — the signature's strength is entirely a function of
// this value's entropy (docs/owasp-vulnerability-and-coverage-report.md
// VULN-05). 32 bytes (256 bits) matches the entropy already used for
// tracking tokens (internal/shared/trackingtoken).
const minJWTSecretLength = 32

// Config holds the process-wide configuration read from the environment.
type Config struct {
	DatabaseURL string
	Port        string
	JWTSecret   string
	JWTTTL      time.Duration
}

// Load reads DATABASE_URL and JWT_SECRET (both required — the process fails
// fast without them, per specs/auth/design.md §5), and PORT/JWT_TTL
// (optional, defaulting to "8080" and 1h respectively) from the environment.
func Load() (Config, error) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		return Config{}, fmt.Errorf("DATABASE_URL environment variable is not set")
	}

	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		return Config{}, fmt.Errorf("JWT_SECRET environment variable is not set")
	}
	if len(jwtSecret) < minJWTSecretLength {
		return Config{}, fmt.Errorf("JWT_SECRET must be at least %d bytes long (got %d)", minJWTSecretLength, len(jwtSecret))
	}

	jwtTTL := defaultJWTTTL
	if raw := os.Getenv("JWT_TTL"); raw != "" {
		parsed, err := time.ParseDuration(raw)
		if err != nil {
			return Config{}, fmt.Errorf("invalid JWT_TTL %q: %w", raw, err)
		}
		jwtTTL = parsed
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	return Config{DatabaseURL: databaseURL, Port: port, JWTSecret: jwtSecret, JWTTTL: jwtTTL}, nil
}
