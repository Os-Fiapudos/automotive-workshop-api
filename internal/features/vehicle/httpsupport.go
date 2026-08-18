package vehicle

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/google/uuid"

	"automotive-workshop-api/internal/shared/apierror"
)

const (
	defaultPage     = 1
	defaultPageSize = 20
	maxPageSize     = 100
)

// customerNotFound is a 404 whose code is CUSTOMER_NOT_FOUND rather than the
// vehicle's own NOT_FOUND, so a client can tell "the vehicle id doesn't
// exist" apart from "the customerId you're linking/browsing doesn't exist"
// (design.md §1.4). apierror.NotFound hardcodes "NOT_FOUND", so this is
// built directly from apierror.Error's exported fields instead — no change
// to the shared apierror package, which customer also depends on.
func customerNotFound(message string) *apierror.Error {
	return &apierror.Error{Status: http.StatusNotFound, Code: "CUSTOMER_NOT_FOUND", Message: message}
}

// decodeJSON reads and decodes r's JSON body into T, returning a
// *apierror.Error (never a bare error) on failure so callers can hand it
// straight to apierror.Write without a separate translation step. Duplicated
// from customer/httpsupport.go rather than shared — see
// specs/customer-management/design.md §1.5 on why this plumbing stays local
// to each feature until a third caller needs it.
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

// parseUUIDField parses a UUID-shaped request body field, returning a
// field-tagged validation detail (not a bare BadRequest) on failure, so it
// composes with CreateRequest.Validate()'s field-level error reporting.
func parseUUIDField(field, value string) (uuid.UUID, *apierror.Detail) {
	id, err := uuid.Parse(value)
	if err != nil {
		return uuid.UUID{}, &apierror.Detail{Field: field, Message: "must be a valid UUID"}
	}
	return id, nil
}

// parseIntParam reads the named query parameter as an int, falling back to
// defaultValue when it is absent or not a valid integer — pagination is a
// display aid, not a business rule worth failing a request over (mirrors
// customer/httpsupport.go).
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
// shared JSON error envelope and HTTP status (design.md §1.4).
func writeServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		apierror.Write(w, apierror.NotFound("vehicle not found"))
	case errors.Is(err, ErrCustomerNotFound):
		apierror.Write(w, customerNotFound("referenced customer does not exist"))
	case errors.Is(err, ErrCustomerInactive):
		apierror.Write(w, apierror.Conflict("CUSTOMER_INACTIVE", "referenced customer is inactive"))
	case errors.Is(err, ErrDuplicatePlate):
		apierror.Write(w, apierror.Conflict("DUPLICATE_LICENSE_PLATE", "license plate already belongs to another vehicle"))
	case errors.Is(err, ErrInvalidPlate):
		apierror.Write(w, apierror.Validation("invalid vehicle data", apierror.Detail{Field: "licensePlate", Message: err.Error()}))
	case errors.Is(err, ErrInvalidYear):
		apierror.Write(w, apierror.Validation("invalid vehicle data", apierror.Detail{Field: "year", Message: err.Error()}))
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

// parsePagination reads/clamps the page and pageSize query params, shared by
// list and listByCustomer (mirrors customer/handler.go's list method).
func parsePagination(r *http.Request) (page, pageSize int) {
	page = parseIntParam(r, "page", defaultPage)
	if page < 1 {
		page = defaultPage
	}

	pageSize = parseIntParam(r, "pageSize", defaultPageSize)
	if pageSize < 1 {
		pageSize = defaultPageSize
	}
	if pageSize > maxPageSize {
		pageSize = maxPageSize
	}

	return page, pageSize
}

// toListResponse builds the paginated envelope shared by list and
// listByCustomer.
func toListResponse(vehicles []*Vehicle, page, pageSize, total int) ListResponse {
	responses := make([]Response, 0, len(vehicles))
	for _, vehicle := range vehicles {
		responses = append(responses, toResponse(vehicle))
	}

	totalPages := 0
	if pageSize > 0 {
		totalPages = (total + pageSize - 1) / pageSize
	}

	return ListResponse{
		Data:       responses,
		Page:       page,
		PageSize:   pageSize,
		Total:      total,
		TotalPages: totalPages,
	}
}
