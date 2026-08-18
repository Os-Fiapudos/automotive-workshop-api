package apierror

import (
	"encoding/json"
	"net/http"
)

// Detail points a validation failure at the specific request field that
// caused it.
type Detail struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// Error is the project's single API error shape. Status is the HTTP status
// to write and is not serialized itself — it is expressed via the response
// status line.
type Error struct {
	Status  int      `json:"-"`
	Code    string   `json:"code"`
	Message string   `json:"message"`
	Details []Detail `json:"details,omitempty"`
}

func (apiError *Error) Error() string { return apiError.Message }

// NotFound builds a 404 error.
func NotFound(message string) *Error {
	return &Error{Status: http.StatusNotFound, Code: "NOT_FOUND", Message: message}
}

// Conflict builds a 409 error with a feature-specific code (e.g.
// "DUPLICATE_DOCUMENT").
func Conflict(code, message string) *Error {
	return &Error{Status: http.StatusConflict, Code: code, Message: message}
}

// Validation builds a 400 error for invalid/missing request fields, with
// optional per-field details.
func Validation(message string, details ...Detail) *Error {
	return &Error{Status: http.StatusBadRequest, Code: "VALIDATION_ERROR", Message: message, Details: details}
}

// BadRequest builds a 400 error for a malformed request body.
func BadRequest(message string) *Error {
	return &Error{Status: http.StatusBadRequest, Code: "INVALID_BODY", Message: message}
}

// Internal builds a 500 error for unexpected failures (e.g. a database error
// that isn't a recognized business error).
func Internal(message string) *Error {
	return &Error{Status: http.StatusInternalServerError, Code: "INTERNAL_ERROR", Message: message}
}

type envelope struct {
	Error *Error `json:"error"`
}

// Write writes err to w as the standard JSON error envelope, using err.Status
// as the HTTP status.
func Write(w http.ResponseWriter, err *Error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(err.Status)
	_ = json.NewEncoder(w).Encode(envelope{Error: err})
}
