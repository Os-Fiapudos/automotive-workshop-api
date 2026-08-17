package auth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"automotive-workshop-api/internal/shared/middleware"
)

// newTestHandler reuses the fakes from service_test.go.
func newTestHandler(t *testing.T) *Handler {
	t.Helper()
	return NewHandler(newTestService(t))
}

// failingIssuer simulates an internal error unrelated to credentials (e.g. a
// signing failure), used to exercise the 500 path in Login.
type failingIssuer struct{}

func (failingIssuer) Generate(string) (string, error) { return "", errors.New("signing key down") }
func (failingIssuer) TTL() time.Duration              { return time.Hour }

// failingFinder simulates a database error unrelated to "user not found",
// used to exercise the 500 path in Login and Me.
type failingFinder struct{}

func (failingFinder) FindByEmail(context.Context, string) (*User, error) {
	return nil, errors.New("db down")
}

func (failingFinder) FindByID(context.Context, string) (*User, error) {
	return nil, errors.New("db down")
}

// fakeVerifier lets tests inject an authenticated user id into the request
// context the same way middleware.RequireAuth does in production, without
// reaching into middleware's unexported context key.
type fakeVerifier struct {
	userID string
}

func (f fakeVerifier) Verify(string) (string, error) { return f.userID, nil }

func TestLoginHandlerSuccess(t *testing.T) {
	h := newTestHandler(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/auth/login",
		strings.NewReader(`{"email":"admin@workshop.local","password":"right-password"}`))
	h.Login(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	var body struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		ExpiresIn   int64  `json:"expires_in"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if body.AccessToken == "" || body.TokenType != "Bearer" || body.ExpiresIn != 3600 {
		t.Fatalf("body = %+v", body)
	}
}

func TestLoginHandlerInvalidCredentialsGenericBody(t *testing.T) {
	h := newTestHandler(t)

	respond := func(payload string) (int, string) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/api/v1/auth/login", strings.NewReader(payload))
		h.Login(rec, req)
		return rec.Code, rec.Body.String()
	}

	unknownStatus, unknownBody := respond(`{"email":"nobody@x.com","password":"whatever"}`)
	wrongStatus, wrongBody := respond(`{"email":"admin@workshop.local","password":"wrong"}`)

	if unknownStatus != 401 || wrongStatus != 401 {
		t.Fatalf("statuses = %d/%d, want 401/401", unknownStatus, wrongStatus)
	}
	if unknownBody != wrongBody {
		t.Fatalf("bodies differ (user enumeration!):\n%s\n%s", unknownBody, wrongBody)
	}
}

func TestLoginHandlerMalformedJSON(t *testing.T) {
	h := newTestHandler(t)
	rec := httptest.NewRecorder()
	h.Login(rec, httptest.NewRequest("POST", "/api/v1/auth/login", strings.NewReader(`{oops`)))
	if rec.Code != 400 {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestLoginHandlerMissingFields(t *testing.T) {
	h := newTestHandler(t)
	rec := httptest.NewRecorder()
	h.Login(rec, httptest.NewRequest("POST", "/api/v1/auth/login", strings.NewReader(`{"email":"a@b.c"}`)))
	if rec.Code != 400 {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestMeHandlerWithoutContextUserID(t *testing.T) {
	h := newTestHandler(t)
	rec := httptest.NewRecorder()
	h.Me(rec, httptest.NewRequest("GET", "/api/v1/auth/me", nil))
	if rec.Code != 401 {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestLoginHandlerMissingPassword(t *testing.T) {
	h := newTestHandler(t)
	rec := httptest.NewRecorder()
	h.Login(rec, httptest.NewRequest("POST", "/api/v1/auth/login", strings.NewReader(`{"password":"whatever"}`)))
	if rec.Code != 400 {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

// TestLoginHandlerInternalError covers the 500 path: valid credentials, but
// token issuance fails for reasons unrelated to the credentials themselves.
// The response must be the generic INTERNAL_ERROR envelope, never leaking the
// underlying error text (BR5).
func TestLoginHandlerInternalError(t *testing.T) {
	h := NewHandler(NewService(newTestService(t).users, failingIssuer{}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/auth/login",
		strings.NewReader(`{"email":"admin@workshop.local","password":"right-password"}`))
	h.Login(rec, req)

	if rec.Code != 500 {
		t.Fatalf("status = %d, want 500; body: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if strings.Contains(body, "signing key down") {
		t.Fatalf("body leaks internal error text: %s", body)
	}
	var parsed struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &parsed); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if parsed.Error.Code != "INTERNAL_ERROR" || parsed.Error.Message != "internal error" {
		t.Fatalf("body = %+v", parsed)
	}
}

// TestMeHandlerInternalError covers the 500 path: an authenticated user id is
// present in context (via the same RequireAuth wrapper used in production),
// but the lookup itself fails for a reason unrelated to "user not found".
func TestMeHandlerInternalError(t *testing.T) {
	h := NewHandler(NewService(failingFinder{}, fakeIssuer{}))
	wrapped := middleware.RequireAuth(fakeVerifier{userID: "u-1"})(http.HandlerFunc(h.Me))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/auth/me", nil)
	req.Header.Set("Authorization", "Bearer sometoken")
	wrapped.ServeHTTP(rec, req)

	if rec.Code != 500 {
		t.Fatalf("status = %d, want 500; body: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if strings.Contains(body, "db down") {
		t.Fatalf("body leaks internal error text: %s", body)
	}
	var parsed struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &parsed); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if parsed.Error.Code != "INTERNAL_ERROR" || parsed.Error.Message != "internal error" {
		t.Fatalf("body = %+v", parsed)
	}
}
