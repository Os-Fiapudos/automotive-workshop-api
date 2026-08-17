package middleware

import (
	"context"
	"net/http"
	"strings"

	"automotive-workshop-api/internal/shared/httpx"
)

// TokenVerifier is satisfied by *token.Manager.
type TokenVerifier interface {
	Verify(tokenString string) (string, error)
}

type userIDKey struct{}

// UserID returns the authenticated user's id injected by RequireAuth.
func UserID(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(userIDKey{}).(string)
	return id, ok
}

// RequireAuth rejects requests without a valid Bearer token (401) and injects
// the token's user id into the request context (FR3). Never logs the token (BR5).
func RequireAuth(tokens TokenVerifier) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tokenString, found := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
			if !found || tokenString == "" {
				httpx.Error(w, http.StatusUnauthorized, "UNAUTHORIZED", "missing or invalid authorization header")
				return
			}
			userID, err := tokens.Verify(tokenString)
			if err != nil {
				httpx.Error(w, http.StatusUnauthorized, "UNAUTHORIZED", "invalid or expired token")
				return
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userIDKey{}, userID)))
		})
	}
}
