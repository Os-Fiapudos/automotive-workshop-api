package httpapi

import (
	"net/http"

	"github.com/google/uuid"

	"automotive-workshop-api/internal/shared/apierror"
)

// startExecution handles POST /api/v1/service-orders/{id}/executions
// (specs/service-order-execution/).
func (handler *serviceOrderHandler) startExecution(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		apierror.Write(w, apierror.NotFound("service order not found"))
		return
	}

	request, apiError := decodeJSON[StartExecutionRequest](r)
	if apiError != nil {
		apierror.Write(w, apiError)
		return
	}
	if details := request.Validate(); len(details) > 0 {
		apierror.Write(w, apierror.Validation("invalid service execution data", details...))
		return
	}

	serviceID, err := uuid.Parse(request.ServiceID)
	if err != nil {
		apierror.Write(w, apierror.NotFound("requested service not found"))
		return
	}

	execution, err := handler.useCases.Executions.StartExecution(r.Context(), id, serviceID)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	w.Header().Set("Location", "/api/v1/service-orders/"+id.String()+"/executions/"+execution.ID.String())
	writeJSON(w, http.StatusCreated, toServiceExecutionResponse(execution))
}

// finishExecution handles
// POST /api/v1/service-orders/{id}/executions/{executionId}/finish.
func (handler *serviceOrderHandler) finishExecution(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		apierror.Write(w, apierror.NotFound("service order not found"))
		return
	}
	executionID, err := uuid.Parse(r.PathValue("executionId"))
	if err != nil {
		apierror.Write(w, apierror.NotFound("service execution not found"))
		return
	}

	request, apiError := decodeOptionalJSON[FinishExecutionRequest](r)
	if apiError != nil {
		apierror.Write(w, apiError)
		return
	}

	execution, err := handler.useCases.Executions.FinishExecution(r.Context(), id, executionID, request.EndedAt)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, toServiceExecutionResponse(execution))
}
