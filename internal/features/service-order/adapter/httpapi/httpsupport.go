package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	"automotive-workshop-api/internal/shared/apierror"
)

// parseIntParam reads a query parameter as an integer, falling back to
// defaultValue when it is missing or malformed — same per-feature helper
// customer/vehicle/product each duplicate, no shared pagination package
// exists yet (specs/service-order-query/design.md §1.4).
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

// decodeJSON reads and decodes r's JSON body into T, returning a
// *apierror.Error (never a bare error) on failure, same helper shape as
// internal/features/customer/httpsupport.go.
func decodeJSON[T any](r *http.Request) (T, *apierror.Error) {
	var value T
	if err := json.NewDecoder(r.Body).Decode(&value); err != nil {
		return value, apierror.BadRequest("request body is not valid JSON")
	}
	return value, nil
}

// decodeOptionalJSON behaves like decodeJSON, except a zero-length request
// body (json.Decode's io.EOF) is treated as "no body sent" rather than an
// error — used by requests whose entire body is optional, like
// FinishExecutionRequest's endedAt (design.md §2.5).
func decodeOptionalJSON[T any](r *http.Request) (T, *apierror.Error) {
	var value T
	if err := json.NewDecoder(r.Body).Decode(&value); err != nil {
		if errors.Is(err, io.EOF) {
			return value, nil
		}
		return value, apierror.BadRequest("request body is not valid JSON")
	}
	return value, nil
}

// writeJSON writes body as JSON with the given HTTP status.
func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
