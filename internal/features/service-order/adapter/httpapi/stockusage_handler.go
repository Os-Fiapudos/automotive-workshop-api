package httpapi

import (
	"net/http"

	"github.com/google/uuid"

	"automotive-workshop-api/internal/shared/apierror"
)

// registerStockUsage handles POST /api/v1/service-orders/{id}/stock-movements
// (specs/service-order-stock-usage/).
func (handler *serviceOrderHandler) registerStockUsage(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		apierror.Write(w, apierror.NotFound("service order not found"))
		return
	}

	request, apiError := decodeJSON[RegisterStockUsageRequest](r)
	if apiError != nil {
		apierror.Write(w, apiError)
		return
	}
	if details := request.Validate(); len(details) > 0 {
		apierror.Write(w, apierror.Validation("invalid stock usage data", details...))
		return
	}

	movements, err := handler.useCases.StockUsage.RegisterStockUsage(r.Context(), id, request.toItems())
	if err != nil {
		writeServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, toStockMovementListResponse(movements))
}

// listStockMovements handles GET /api/v1/service-orders/{id}/stock-movements
// (specs/service-order-stock-usage/).
func (handler *serviceOrderHandler) listStockMovements(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		apierror.Write(w, apierror.NotFound("service order not found"))
		return
	}

	movements, err := handler.useCases.StockUsage.ListStockMovements(r.Context(), id)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, toStockMovementListResponse(movements))
}

// reverseStockMovement handles
// POST /api/v1/service-orders/{id}/stock-movements/{movementId}/reversal
// (specs/service-order-stock-usage/).
func (handler *serviceOrderHandler) reverseStockMovement(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		apierror.Write(w, apierror.NotFound("service order not found"))
		return
	}
	movementID, err := uuid.Parse(r.PathValue("movementId"))
	if err != nil {
		apierror.Write(w, apierror.NotFound("stock movement not found"))
		return
	}

	reversal, err := handler.useCases.StockUsage.ReverseStockMovement(r.Context(), id, movementID)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, toStockMovementResponse(reversal))
}
