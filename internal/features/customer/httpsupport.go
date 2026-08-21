package customer

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/google/uuid"

	"automotive-workshop-api/internal/shared/apierror"
)

// decodeJSON reads and decodes r's JSON body into T, returning a
// *apierror.Error (never a bare error) on failure so callers can hand it
// straight to apierror.Write without a separate translation step.
func decodeJSON[T any](r *http.Request) (T, *apierror.Error) {
	var value T
	if err := json.NewDecoder(r.Body).Decode(&value); err != nil {
		return value, apierror.BadRequest("request body is not valid JSON")
	}
	return value, nil
}

// parseUUIDPathValue parses the named path parameter as a UUID, returning a
// *apierror.Error on failure.
func parseUUIDPathValue(r *http.Request, key string) (uuid.UUID, *apierror.Error) {
	id, err := uuid.Parse(r.PathValue(key))
	if err != nil {
		return uuid.UUID{}, apierror.BadRequest(key + " must be a valid UUID")
	}
	return id, nil
}

// parseIntParam reads the named query parameter as an int, falling back to
// defaultValue when it is absent or not a valid integer. Pagination is a
// display aid, not a business rule worth failing a request over
// (design.md §1.5), so an unparseable value is not an error — it just falls
// back to defaultValue.
func parseIntParam(r *http.Request, key string, defaultValue int) int {
	rawValue := r.URL.Query().Get(key)
	if rawValue == "" {
		return defaultValue
	}
	parsedValue, err := strconv.Atoi(rawValue)
	if err != nil {
		return defaultValue
	}
	return parsedValue
}

// writeServiceError maps a service/domain-level error to the project's
// shared JSON error envelope and HTTP status (design.md §1.5).
func writeServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		apierror.Write(w, apierror.NotFound("customer not found"))
	case errors.Is(err, ErrDuplicateDocument):
		apierror.Write(w, apierror.Conflict("DUPLICATE_DOCUMENT", "document already belongs to another customer"))
	case errors.Is(err, ErrDuplicateEmail):
		apierror.Write(w, apierror.Conflict("DUPLICATE_EMAIL", "email already belongs to another customer"))
	case errors.Is(err, ErrInvalidDocument):
		apierror.Write(w, apierror.Validation("invalid customer data", apierror.Detail{Field: "document", Message: err.Error()}))
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
