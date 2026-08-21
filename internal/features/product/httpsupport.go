package product

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/google/uuid"

	"automotive-workshop-api/internal/shared/apierror"
)

// decodeJSON reads and decodes request body into target struct T, returning an apierror.Error on failure.
//
// Args:
//
//	r(*http.Request): incoming HTTP request
//
// Returns:
//
//	value(T): decoded struct
//	apiErr(*apierror.Error): error response if JSON is malformed
func decodeJSON[T any](r *http.Request) (T, *apierror.Error) {
	var value T
	if err := json.NewDecoder(r.Body).Decode(&value); err != nil {
		return value, apierror.BadRequest("request body is not valid JSON")
	}
	return value, nil
}

// parseUUIDPathValue parses a path parameter as a valid UUID.
//
// Args:
//
//	r(*http.Request): incoming HTTP request
//	key(string): path parameter name
//
// Returns:
//
//	id(uuid.UUID): parsed UUID
//	apiErr(*apierror.Error): error if invalid UUID format
func parseUUIDPathValue(r *http.Request, key string) (uuid.UUID, *apierror.Error) {
	id, err := uuid.Parse(r.PathValue(key))
	if err != nil {
		return uuid.UUID{}, apierror.BadRequest(key + " must be a valid UUID")
	}
	return id, nil
}

// parseIntParam reads a query parameter as an integer or returns defaultValue if missing or invalid.
//
// Args:
//
//	r(*http.Request): incoming HTTP request
//	key(string): query parameter key
//	defaultValue(int): fallback value
//
// Returns:
//
//	param(int): parsed query parameter or default value
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

// writeServiceError maps product service/domain errors to standardized JSON apierror envelope.
//
// Args:
//
//	w(http.ResponseWriter): HTTP response writer
//	err(error): service or domain error
//
// Returns:
//
//	(none)
func writeServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		apierror.Write(w, apierror.NotFound("product not found"))
	case errors.Is(err, ErrDuplicateCode):
		apierror.Write(w, apierror.Conflict("DUPLICATE_CODE", "product code already belongs to another product"))
	case errors.Is(err, ErrInsufficientStock):
		apierror.Write(w, apierror.Conflict("INSUFFICIENT_STOCK", "insufficient stock balance for exit adjustment"))
	case errors.Is(err, ErrInvalidQuantity):
		apierror.Write(w, apierror.Validation("invalid stock adjustment data", apierror.Detail{Field: "quantity", Message: err.Error()}))
	case errors.Is(err, ErrEmptyReason):
		apierror.Write(w, apierror.Validation("invalid stock adjustment data", apierror.Detail{Field: "reason", Message: err.Error()}))
	case errors.Is(err, ErrInvalidMovementType):
		apierror.Write(w, apierror.Validation("invalid stock adjustment data", apierror.Detail{Field: "type", Message: err.Error()}))
	case errors.Is(err, ErrStockDirectUpdateNotAllowed):
		apierror.Write(w, apierror.Validation("invalid product data", apierror.Detail{Field: "currentStock", Message: err.Error()}))
	case errors.Is(err, ErrInvalidUnitPrice):
		apierror.Write(w, apierror.Validation("invalid product data", apierror.Detail{Field: "unitPrice", Message: err.Error()}))
	case errors.Is(err, ErrInvalidStock):
		apierror.Write(w, apierror.Validation("invalid product data", apierror.Detail{Field: "currentStock", Message: err.Error()}))
	case errors.Is(err, ErrInvalidType):
		apierror.Write(w, apierror.Validation("invalid product data", apierror.Detail{Field: "type", Message: err.Error()}))
	case errors.Is(err, ErrInactiveProduct):
		apierror.Write(w, apierror.Validation("invalid product status", apierror.Detail{Field: "status", Message: err.Error()}))
	case errors.Is(err, ErrProductInUse):
		apierror.Write(w, apierror.Conflict("PRODUCT_IN_USE", err.Error()))
	default:
		apierror.Write(w, apierror.Internal("unexpected error"))
	}
}

// writeJSON writes body as JSON response with the given status code.
//
// Args:
//
//	w(http.ResponseWriter): HTTP response writer
//	status(int): HTTP status code
//	body(any): response body payload
//
// Returns:
//
//	(none)
func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
