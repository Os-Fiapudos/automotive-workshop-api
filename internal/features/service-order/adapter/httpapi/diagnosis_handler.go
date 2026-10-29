package httpapi

import (
	"net/http"

	"github.com/google/uuid"

	"automotive-workshop-api/internal/shared/apierror"
)

func (handler *serviceOrderHandler) startDiagnosis(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		apierror.Write(w, apierror.NotFound("service order not found"))
		return
	}

	order, err := handler.useCases.Lifecycle.StartDiagnosis(r.Context(), id)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, toServiceOrderStatusResponse(order))
}
