package servicetracking

import (
	"encoding/json"
	"errors"
	"net/http"

	"automotive-workshop-api/internal/shared/apierror"
)

// writeServiceError maps a service-level error to the project's shared JSON
// error envelope (design.md §8). Reuses apierror, the envelope this
// feature's closest sibling (internal/features/service-order) already uses
// (CLAUDE.md §8).
func writeServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrOrderNotFound):
		apierror.Write(w, apierror.NotFound("service order not found"))
	case errors.Is(err, ErrTokenInvalid):
		apierror.Write(w, apierror.Unauthorized("INVALID_TRACKING_TOKEN", "tracking token is missing, invalid, or does not grant access to this service order"))
	default:
		apierror.Write(w, apierror.Internal("unexpected error"))
	}
}

// writeJSON writes body as JSON with the given HTTP status.
func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
