// Package handlers_test: HTTP integration tests. They require DATABASE_URL
// pointing at a database with docs/schema.sql + docs/seed.sql applied and are
// skipped when it is not set.
package handlers_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"automotive-workshop-api/internal/features/auth"
	"automotive-workshop-api/internal/shared/database"
	"automotive-workshop-api/internal/shared/middleware"
	"automotive-workshop-api/internal/shared/token"
)

const testSecret = "integration-test-secret"

// newTestMux mirrors the wiring in cmd/api/main.go.
func newTestMux(t *testing.T) *http.ServeMux {
	t.Helper()
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("DATABASE_URL not set; skipping integration tests")
	}
	pool, err := database.Connect(context.Background(), url)
	if err != nil {
		t.Fatalf("database connect: %v", err)
	}
	t.Cleanup(pool.Close)

	tokens := token.NewManager(testSecret, time.Hour)
	handler := auth.NewHandler(auth.NewService(auth.NewRepository(pool), tokens))
	requireAuth := middleware.RequireAuth(tokens)

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/auth/login", handler.Login)
	mux.Handle("GET /api/v1/auth/me", requireAuth(http.HandlerFunc(handler.Me)))
	return mux
}

func doLogin(t *testing.T, mux *http.ServeMux, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/auth/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	mux.ServeHTTP(rec, req)
	return rec
}

func loginToken(t *testing.T, mux *http.ServeMux) string {
	t.Helper()
	rec := doLogin(t, mux, `{"email":"admin@workshop.local","password":"admin123"}`)
	if rec.Code != 200 {
		t.Fatalf("login status = %d, body: %s", rec.Code, rec.Body.String())
	}
	var body struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid login JSON: %v", err)
	}
	return body.AccessToken
}

func getMe(t *testing.T, mux *http.ServeMux, authorization string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/auth/me", nil)
	if authorization != "" {
		req.Header.Set("Authorization", authorization)
	}
	mux.ServeHTTP(rec, req)
	return rec
}

// AC1: valid credentials -> 200 + JWT with expiration.
func TestLoginWithValidCredentials(t *testing.T) {
	mux := newTestMux(t)
	tok := loginToken(t, mux)
	userID, err := token.NewManager(testSecret, time.Hour).Verify(tok)
	if err != nil {
		t.Fatalf("returned token does not verify: %v", err)
	}
	if userID == "" {
		t.Fatal("token subject is empty")
	}
}

// AC2 + BR4: invalid credentials -> 401, identical generic body for
// unknown e-mail and wrong password.
func TestLoginWithInvalidCredentials(t *testing.T) {
	mux := newTestMux(t)
	unknown := doLogin(t, mux, `{"email":"ghost@workshop.local","password":"admin123"}`)
	wrongPass := doLogin(t, mux, `{"email":"admin@workshop.local","password":"not-it"}`)

	if unknown.Code != 401 || wrongPass.Code != 401 {
		t.Fatalf("statuses = %d/%d, want 401/401", unknown.Code, wrongPass.Code)
	}
	if unknown.Body.String() != wrongPass.Body.String() {
		t.Fatalf("bodies differ (user enumeration):\n%s\n%s", unknown.Body.String(), wrongPass.Body.String())
	}
}

// AC3: protected route without token -> 401.
func TestMeWithoutToken(t *testing.T) {
	mux := newTestMux(t)
	if rec := getMe(t, mux, ""); rec.Code != 401 {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

// AC4: invalid and expired tokens -> 401.
func TestMeWithInvalidToken(t *testing.T) {
	mux := newTestMux(t)
	if rec := getMe(t, mux, "Bearer this.is.garbage"); rec.Code != 401 {
		t.Fatalf("garbage token: status = %d, want 401", rec.Code)
	}
}

func TestMeWithExpiredToken(t *testing.T) {
	mux := newTestMux(t)
	valid := loginToken(t, mux)
	userID, err := token.NewManager(testSecret, time.Hour).Verify(valid)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	expired, err := token.NewManager(testSecret, -time.Minute).Generate(userID)
	if err != nil {
		t.Fatalf("generate expired: %v", err)
	}
	if rec := getMe(t, mux, "Bearer "+expired); rec.Code != 401 {
		t.Fatalf("expired token: status = %d, want 401", rec.Code)
	}
}

// FR4: valid token -> 200 with the user's public data (and no hash).
func TestMeWithValidToken(t *testing.T) {
	mux := newTestMux(t)
	rec := getMe(t, mux, "Bearer "+loginToken(t, mux))
	if rec.Code != 200 {
		t.Fatalf("status = %d, body: %s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if body["email"] != "admin@workshop.local" {
		t.Fatalf("email = %v", body["email"])
	}
	if _, leaked := body["password_hash"]; leaked {
		t.Fatal("password_hash leaked in /me response")
	}
	if strings.Contains(rec.Body.String(), "$2a$") {
		t.Fatal("bcrypt hash leaked in /me response body")
	}
}
