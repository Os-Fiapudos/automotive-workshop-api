package serviceorder

import (
	"encoding/json"
	"errors"
	"net/http"

	"automotive-workshop-api/internal/shared/apierror"
)

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
