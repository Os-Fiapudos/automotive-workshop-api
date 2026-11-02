package httpapi

import (
	"errors"
	"net/http"

	"automotive-workshop-api/internal/features/service-order/domain"
	"automotive-workshop-api/internal/shared/apierror"
)

// writeServiceError maps a service/domain-level error to the project's
// shared JSON error envelope and HTTP status (design.md §1.5).
func writeServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domain.ErrCustomerNotFound):
		apierror.Write(w, apierror.NotFound("customer not found"))
	case errors.Is(err, domain.ErrCustomerInactive):
		apierror.Write(w, apierror.Conflict("CUSTOMER_INACTIVE", "customer is inactive"))
	case errors.Is(err, domain.ErrVehicleNotFound):
		apierror.Write(w, apierror.NotFound("vehicle not found"))
	case errors.Is(err, domain.ErrVehicleInactive):
		apierror.Write(w, apierror.Conflict("VEHICLE_INACTIVE", "vehicle is inactive"))
	case errors.Is(err, domain.ErrVehicleNotOwnedByCustomer):
		apierror.Write(w, apierror.Conflict("VEHICLE_NOT_OWNED_BY_CUSTOMER", "vehicle does not belong to the identified customer"))
	case errors.Is(err, domain.ErrRequestedServiceNotFound):
		apierror.Write(w, apierror.NotFound("requested service not found"))
	case errors.Is(err, domain.ErrRequestedProductNotFound):
		apierror.Write(w, &apierror.Error{Status: http.StatusNotFound, Code: "PRODUCT_NOT_FOUND", Message: "requested product not found or inactive"})
	case errors.Is(err, domain.ErrInvalidRequestedProductQuantity):
		apierror.Write(w, apierror.Validation("requested product quantity must be a positive integer"))
	case errors.Is(err, domain.ErrDuplicateRequestedProduct):
		apierror.Write(w, apierror.Validation("a product may appear only once in requestedProducts"))
	case errors.Is(err, domain.ErrInvalidAggregate):
		apierror.Write(w, apierror.Validation("invalid service order data"))
	case errors.Is(err, domain.ErrServiceOrderNotFound):
		apierror.Write(w, apierror.NotFound("service order not found"))
	case errors.Is(err, domain.ErrInvalidStatusTransition):
		apierror.Write(w, apierror.Conflict("INVALID_STATUS_TRANSITION", "service order is not in a status that allows this operation"))
	case errors.Is(err, domain.ErrDiagnosisNotStarted):
		apierror.Write(w, apierror.Conflict("DIAGNOSIS_NOT_STARTED", "diagnosis has not started for this service order"))
	case errors.Is(err, domain.ErrQuoteAlreadyDecided):
		apierror.Write(w, apierror.Conflict("QUOTE_ALREADY_DECIDED", "quote has already been decided and cannot be altered"))
	case errors.Is(err, domain.ErrQuoteNotFound):
		apierror.Write(w, apierror.NotFound("quote not found"))
	case errors.Is(err, domain.ErrEmptyQuote):
		apierror.Write(w, apierror.Validation("quote must have at least one item"))
	case errors.Is(err, domain.ErrInvalidQuantity):
		apierror.Write(w, apierror.Validation("item quantity must be greater than zero"))
	case errors.Is(err, domain.ErrQuoteAmountOutOfRange):
		apierror.Write(w, apierror.Validation("quote amount exceeds the maximum of 9999999999.99"))
	case errors.Is(err, domain.ErrProductNotFound):
		apierror.Write(w, apierror.NotFound("product not found"))
	case errors.Is(err, domain.ErrProductInactive):
		apierror.Write(w, apierror.Conflict("PRODUCT_INACTIVE", "product is inactive"))
	case errors.Is(err, domain.ErrServiceExecutionNotFound):
		apierror.Write(w, apierror.NotFound("service execution not found"))
	case errors.Is(err, domain.ErrServiceExecutionAlreadyFinished):
		apierror.Write(w, apierror.Conflict("SERVICE_EXECUTION_ALREADY_FINISHED", "service execution has already been finished"))
	case errors.Is(err, domain.ErrServiceExecutionEndBeforeStart):
		apierror.Write(w, apierror.Validation("service execution end date cannot be before its start date"))
	case errors.Is(err, domain.ErrExecutionsNotCompleted):
		apierror.Write(w, apierror.Conflict("EXECUTIONS_NOT_COMPLETED", "service order has pending service executions and cannot be finalized"))
	case errors.Is(err, domain.ErrTrackingTokenInvalid):
		apierror.Write(w, apierror.Unauthorized("INVALID_TRACKING_TOKEN", "tracking token is invalid, revoked, or does not match this service order"))
	case errors.Is(err, domain.ErrInvalidQuoteDecision):
		apierror.Write(w, apierror.Validation("decision must be approved or rejected"))
	case errors.Is(err, domain.ErrUnsupportedExternalStatus):
		apierror.Write(w, apierror.Validation("status must be one of IN_DIAGNOSIS, COMPLETED, DELIVERED"))
	case errors.Is(err, domain.ErrInvalidStatusUpdateNotes):
		apierror.Write(w, apierror.Validation("notes must be at most 500 characters"))
	case errors.Is(err, domain.ErrEmptyStockUsage):
		apierror.Write(w, apierror.Validation("stock usage must include at least one item"))
	case errors.Is(err, domain.ErrInsufficientStock):
		apierror.Write(w, apierror.Conflict("INSUFFICIENT_STOCK", "insufficient stock balance for this product"))
	case errors.Is(err, domain.ErrStockMovementNotFound):
		apierror.Write(w, apierror.NotFound("stock movement not found"))
	case errors.Is(err, domain.ErrStockMovementAlreadyReversed):
		apierror.Write(w, apierror.Conflict("STOCK_MOVEMENT_ALREADY_REVERSED", "stock movement has already been reversed"))
	case errors.Is(err, domain.ErrStockMovementNotReversible):
		apierror.Write(w, apierror.Conflict("STOCK_MOVEMENT_NOT_REVERSIBLE", "stock movement is not reversible"))
	default:
		apierror.Write(w, apierror.Internal("unexpected error"))
	}
}
