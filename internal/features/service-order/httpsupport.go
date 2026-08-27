package serviceorder

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

// writeServiceError maps a service/domain-level error to the project's
// shared JSON error envelope and HTTP status (design.md §1.5).
func writeServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrCustomerNotFound):
		apierror.Write(w, apierror.NotFound("customer not found"))
	case errors.Is(err, ErrCustomerInactive):
		apierror.Write(w, apierror.Conflict("CUSTOMER_INACTIVE", "customer is inactive"))
	case errors.Is(err, ErrVehicleNotFound):
		apierror.Write(w, apierror.NotFound("vehicle not found"))
	case errors.Is(err, ErrVehicleInactive):
		apierror.Write(w, apierror.Conflict("VEHICLE_INACTIVE", "vehicle is inactive"))
	case errors.Is(err, ErrVehicleNotOwnedByCustomer):
		apierror.Write(w, apierror.Conflict("VEHICLE_NOT_OWNED_BY_CUSTOMER", "vehicle does not belong to the identified customer"))
	case errors.Is(err, ErrRequestedServiceNotFound):
		apierror.Write(w, apierror.NotFound("requested service not found"))
	case errors.Is(err, ErrInvalidAggregate):
		apierror.Write(w, apierror.Validation("invalid service order data"))
	case errors.Is(err, ErrServiceOrderNotFound):
		apierror.Write(w, apierror.NotFound("service order not found"))
	case errors.Is(err, ErrInvalidStatusTransition):
		apierror.Write(w, apierror.Conflict("INVALID_STATUS_TRANSITION", "service order is not in a status that allows this operation"))
	case errors.Is(err, ErrDiagnosisNotStarted):
		apierror.Write(w, apierror.Conflict("DIAGNOSIS_NOT_STARTED", "diagnosis has not started for this service order"))
	case errors.Is(err, ErrQuoteAlreadyDecided):
		apierror.Write(w, apierror.Conflict("QUOTE_ALREADY_DECIDED", "quote has already been decided and cannot be altered"))
	case errors.Is(err, ErrQuoteNotFound):
		apierror.Write(w, apierror.NotFound("quote not found"))
	case errors.Is(err, ErrEmptyQuote):
		apierror.Write(w, apierror.Validation("quote must have at least one item"))
	case errors.Is(err, ErrInvalidQuantity):
		apierror.Write(w, apierror.Validation("item quantity must be greater than zero"))
	case errors.Is(err, ErrProductNotFound):
		apierror.Write(w, apierror.NotFound("product not found"))
	case errors.Is(err, ErrProductInactive):
		apierror.Write(w, apierror.Conflict("PRODUCT_INACTIVE", "product is inactive"))
	case errors.Is(err, ErrServiceExecutionNotFound):
		apierror.Write(w, apierror.NotFound("service execution not found"))
	case errors.Is(err, ErrServiceExecutionAlreadyFinished):
		apierror.Write(w, apierror.Conflict("SERVICE_EXECUTION_ALREADY_FINISHED", "service execution has already been finished"))
	case errors.Is(err, ErrServiceExecutionEndBeforeStart):
		apierror.Write(w, apierror.Validation("service execution end date cannot be before its start date"))
	case errors.Is(err, ErrExecutionsNotCompleted):
		apierror.Write(w, apierror.Conflict("EXECUTIONS_NOT_COMPLETED", "service order has pending service executions and cannot be finalized"))
	case errors.Is(err, ErrTrackingTokenInvalid):
		apierror.Write(w, apierror.Unauthorized("INVALID_TRACKING_TOKEN", "tracking token is invalid, revoked, or does not match this service order"))
	case errors.Is(err, ErrEmptyStockUsage):
		apierror.Write(w, apierror.Validation("stock usage must include at least one item"))
	case errors.Is(err, ErrInsufficientStock):
		apierror.Write(w, apierror.Conflict("INSUFFICIENT_STOCK", "insufficient stock balance for this product"))
	case errors.Is(err, ErrStockMovementNotFound):
		apierror.Write(w, apierror.NotFound("stock movement not found"))
	case errors.Is(err, ErrStockMovementAlreadyReversed):
		apierror.Write(w, apierror.Conflict("STOCK_MOVEMENT_ALREADY_REVERSED", "stock movement has already been reversed"))
	case errors.Is(err, ErrStockMovementNotReversible):
		apierror.Write(w, apierror.Conflict("STOCK_MOVEMENT_NOT_REVERSIBLE", "stock movement is not reversible"))
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
