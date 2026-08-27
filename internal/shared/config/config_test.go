package config

import (
	"strings"
	"testing"
	"time"
)

const validJWTSecret = "0123456789abcdef0123456789abcdef" // 33 bytes, over the 32-byte minimum

func TestLoadSuccessWithDefaults(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://user:pass@localhost:5432/db")
	t.Setenv("JWT_SECRET", validJWTSecret)
	t.Setenv("JWT_TTL", "")
	t.Setenv("PORT", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.DatabaseURL != "postgres://user:pass@localhost:5432/db" {
		t.Errorf("DatabaseURL = %q", cfg.DatabaseURL)
	}
	if cfg.JWTSecret != validJWTSecret {
		t.Errorf("JWTSecret = %q", cfg.JWTSecret)
	}
	if cfg.Port != "8080" {
		t.Errorf("Port default = %q, want 8080", cfg.Port)
	}
	if cfg.JWTTTL != defaultJWTTTL {
		t.Errorf("JWTTTL default = %v, want %v", cfg.JWTTTL, defaultJWTTTL)
	}
}

func TestLoadReadsOptionalOverrides(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://user:pass@localhost:5432/db")
	t.Setenv("JWT_SECRET", validJWTSecret)
	t.Setenv("JWT_TTL", "2h")
	t.Setenv("PORT", "9090")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Port != "9090" {
		t.Errorf("Port = %q, want 9090", cfg.Port)
	}
	if cfg.JWTTTL != 2*time.Hour {
		t.Errorf("JWTTTL = %v, want 2h", cfg.JWTTTL)
	}
}

func TestLoadRequiresDatabaseURL(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	t.Setenv("JWT_SECRET", validJWTSecret)

	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "DATABASE_URL") {
		t.Fatalf("expected a DATABASE_URL error, got: %v", err)
	}
}

func TestLoadRequiresJWTSecret(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://user:pass@localhost:5432/db")
	t.Setenv("JWT_SECRET", "")

	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "JWT_SECRET") {
		t.Fatalf("expected a JWT_SECRET error, got: %v", err)
	}
}

// TestLoadRejectsShortJWTSecret guards against VULN-05
// (docs/owasp-vulnerability-and-coverage-report.md): a short/trivial secret
// undermines HS256 signing regardless of everything else being configured
// correctly, so Load must fail fast on it exactly like it already does for a
// missing DATABASE_URL/JWT_SECRET.
func TestLoadRejectsShortJWTSecret(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://user:pass@localhost:5432/db")
	t.Setenv("JWT_SECRET", "too-short")

	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "JWT_SECRET") {
		t.Fatalf("expected a JWT_SECRET length error, got: %v", err)
	}
}

func TestLoadRejectsInvalidJWTTTL(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://user:pass@localhost:5432/db")
	t.Setenv("JWT_SECRET", validJWTSecret)
	t.Setenv("JWT_TTL", "not-a-duration")

	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "JWT_TTL") {
		t.Fatalf("expected a JWT_TTL error, got: %v", err)
	}
}
