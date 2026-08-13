package auth

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

// newTestHandler reuses the fakes from service_test.go.
func newTestHandler(t *testing.T) *Handler {
	t.Helper()
	return NewHandler(newTestService(t))
}

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
