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

func TestLoadResendConfiguration(t *testing.T) {
	cases := []struct {
		name    string
		key     string
		from    string
		wantErr bool
	}{
		{"disabled when neither is set", "", "", false},
		{"disabled when only the sender is set", "", "oficina@example.com", false},
		{"key without sender is rejected", "re_key", "", true},
		{"complete pair", "re_key", "oficina@example.com", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("DATABASE_URL", "postgres://user:pass@localhost:5432/db")
			t.Setenv("JWT_SECRET", validJWTSecret)
			t.Setenv("RESEND_API_KEY", tc.key)
			t.Setenv("RESEND_FROM_EMAIL", tc.from)

			cfg, err := Load()
			if tc.wantErr {
				if err == nil || !strings.Contains(err.Error(), "RESEND_FROM_EMAIL") {
					t.Fatalf("expected RESEND_FROM_EMAIL error, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if cfg.ResendAPIKey != tc.key || cfg.ResendFromEmail != tc.from {
				t.Errorf("got key=%q from=%q", cfg.ResendAPIKey, cfg.ResendFromEmail)
			}
		})
	}
}

func setAdminBootstrapEnv(t *testing.T, databaseURL, email, password, name string) {
	t.Helper()
	t.Setenv("DATABASE_URL", databaseURL)
	t.Setenv("ADMIN_EMAIL", email)
	t.Setenv("ADMIN_PASSWORD", password)
	t.Setenv("ADMIN_NAME", name)
}

func TestLoadAdminBootstrapReadsVariables(t *testing.T) {
	setAdminBootstrapEnv(t, "postgres://user:pass@localhost:5432/db", "ops@workshop.example", "a-long-password", "Ops")
	// The bootstrap command issues no token, so JWT_SECRET must not be required.
	t.Setenv("JWT_SECRET", "")

	cfg, err := LoadAdminBootstrap()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := AdminBootstrapConfig{
		DatabaseURL:   "postgres://user:pass@localhost:5432/db",
		AdminName:     "Ops",
		AdminEmail:    "ops@workshop.example",
		AdminPassword: "a-long-password",
	}
	if cfg != want {
		t.Errorf("cfg = %+v, want %+v", cfg, want)
	}
}

func TestLoadAdminBootstrapNameIsOptional(t *testing.T) {
	setAdminBootstrapEnv(t, "postgres://user:pass@localhost:5432/db", "ops@workshop.example", "a-long-password", "")
	cfg, err := LoadAdminBootstrap()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.AdminName != "" {
		t.Errorf("AdminName = %q, want empty (auth applies the default)", cfg.AdminName)
	}
}

func TestLoadAdminBootstrapRequiresVariables(t *testing.T) {
	const password = "a-long-password"
	cases := []struct {
		missing                   string
		databaseURL, email, admin string
	}{
		{"DATABASE_URL", "", "ops@workshop.example", password},
		{"ADMIN_EMAIL", "postgres://localhost/db", "", password},
		{"ADMIN_PASSWORD", "postgres://localhost/db", "ops@workshop.example", ""},
	}
	for _, tc := range cases {
		t.Run(tc.missing, func(t *testing.T) {
			setAdminBootstrapEnv(t, tc.databaseURL, tc.email, tc.admin, "")
			_, err := LoadAdminBootstrap()
			if err == nil || !strings.Contains(err.Error(), tc.missing) {
				t.Fatalf("err = %v, want it to name %s", err, tc.missing)
			}
			if strings.Contains(err.Error(), password) {
				t.Errorf("error leaks the password: %v", err)
			}
		})
	}
}
