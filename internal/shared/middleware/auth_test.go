package middleware

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type fakeVerifier struct {
	userID string
	err    error
}

func (f fakeVerifier) Verify(string) (string, error) { return f.userID, f.err }

func protectedEcho(t *testing.T, calledUserID *string) http.Handler {
	t.Helper()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, ok := UserID(r.Context())
		if !ok {
			t.Fatal("UserID not found in context")
		}
		*calledUserID = id
	})
}

func TestRequireAuthMissingHeader(t *testing.T) {
	var called string
	h := RequireAuth(fakeVerifier{userID: "u1"})(protectedEcho(t, &called))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/x", nil))

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
	if called != "" {
		t.Fatal("next handler must not run")
	}
	if !strings.Contains(rec.Body.String(), `"UNAUTHORIZED"`) {
		t.Fatalf("body %q missing standard envelope", rec.Body.String())
	}
}

func TestRequireAuthWrongScheme(t *testing.T) {
	h := RequireAuth(fakeVerifier{userID: "u1"})(http.NotFoundHandler())
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/x", nil)
	req.Header.Set("Authorization", "Basic abc123")
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestRequireAuthInvalidToken(t *testing.T) {
	h := RequireAuth(fakeVerifier{err: errors.New("bad")})(http.NotFoundHandler())
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/x", nil)
	req.Header.Set("Authorization", "Bearer sometoken")
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestRequireAuthValidTokenInjectsUserID(t *testing.T) {
	var called string
	h := RequireAuth(fakeVerifier{userID: "user-42"})(protectedEcho(t, &called))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/x", nil)
	req.Header.Set("Authorization", "Bearer sometoken")
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if called != "user-42" {
		t.Fatalf("user id in context = %q, want user-42", called)
	}
}
