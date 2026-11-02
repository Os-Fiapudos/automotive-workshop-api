package handlers_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"automotive-workshop-api/internal/features/auth"
	"automotive-workshop-api/internal/shared/database"
)

// TestBootstrapAdminThenLogin covers specs/database-migrations/requirements.md
// AC8 against the real database: the bootstrapped administrator can log in,
// and bootstrapping the same e-mail again changes nothing.
func TestBootstrapAdminThenLogin(t *testing.T) {
	mux := newTestMux(t) // skips without DATABASE_URL
	ctx := context.Background()
	pool, err := database.NewPool(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		t.Fatalf("database connect: %v", err)
	}
	t.Cleanup(pool.Close)

	email := fmt.Sprintf("bootstrap-%d@workshop.test", time.Now().UnixNano())
	const firstPassword = "first-bootstrap-password"
	t.Cleanup(func() {
		if _, err := pool.Exec(context.Background(), "DELETE FROM users WHERE email = $1", email); err != nil {
			t.Errorf("cleanup: %v", err)
		}
	})
	repository := auth.NewRepository(pool)

	first, err := auth.BootstrapAdmin(ctx, repository, auth.AdminBootstrapInput{Email: email, Password: firstPassword})
	if err != nil {
		t.Fatalf("first bootstrap: %v", err)
	}
	if !first.Created || first.Code == 0 {
		t.Fatalf("first bootstrap result = %+v, want a created user with a code", first)
	}
	if rec := doLogin(t, mux, fmt.Sprintf(`{"email":%q,"password":%q}`, email, firstPassword)); rec.Code != 200 {
		t.Fatalf("login with the bootstrapped credentials: status %d, body %s", rec.Code, rec.Body.String())
	}

	second, err := auth.BootstrapAdmin(ctx, repository, auth.AdminBootstrapInput{Email: email, Password: "another-password-entirely"})
	if err != nil {
		t.Fatalf("second bootstrap: %v", err)
	}
	if second.Created {
		t.Fatalf("second bootstrap created a user again: %+v", second)
	}
	if rec := doLogin(t, mux, fmt.Sprintf(`{"email":%q,"password":%q}`, email, firstPassword)); rec.Code != 200 {
		t.Fatalf("the original password stopped working after a second bootstrap: status %d", rec.Code)
	}
	if rec := doLogin(t, mux, fmt.Sprintf(`{"email":%q,"password":%q}`, email, "another-password-entirely")); rec.Code != 401 {
		t.Fatalf("the second bootstrap replaced the password: status %d, want 401", rec.Code)
	}

	var users int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM users WHERE email = $1", email).Scan(&users); err != nil {
		t.Fatalf("count users: %v", err)
	}
	if users != 1 {
		t.Fatalf("users with the bootstrapped e-mail = %d, want 1", users)
	}
}
