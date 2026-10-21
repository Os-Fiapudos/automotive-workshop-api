package httpapi

import (
	"net/http"

	"github.com/google/uuid"

	"automotive-workshop-api/internal/shared/apierror"
)

// finalizeOrder handles POST /api/v1/service-orders/{id}/finalize.
func (handler *serviceOrderHandler) finalizeOrder(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		apierror.Write(w, apierror.NotFound("service order not found"))
		return
	}

	order, err := handler.useCases.Lifecycle.FinalizeOrder(r.Context(), id)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, toServiceOrderStatusResponse(order))
}

// deliverOrder handles POST /api/v1/service-orders/{id}/deliver.
func (handler *serviceOrderHandler) deliverOrder(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		apierror.Write(w, apierror.NotFound("service order not found"))
		return
	}

	order, err := handler.useCases.Lifecycle.DeliverOrder(r.Context(), id)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, toServiceOrderStatusResponse(order))
}
