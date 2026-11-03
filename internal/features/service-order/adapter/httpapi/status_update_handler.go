package httpapi

import (
	"net/http"

	"github.com/google/uuid"

	"automotive-workshop-api/internal/features/service-order/domain"
	"automotive-workshop-api/internal/shared/apierror"
)

// updateStatusExternal handles POST /api/v1/service-orders/{id}/status
// (specs/service-order-status-update/design.md §4). The secret is checked
// before {id} or the body are looked at, and the body is validated before
// the order is loaded, so a 401/400 never reveals or changes anything.
func (handler *serviceOrderHandler) updateStatusExternal(w http.ResponseWriter, r *http.Request) {
	if !handler.useCases.Lifecycle.AuthenticateExternalStatusUpdate(r.Header.Get(externalStatusSecretHeader)) {
		apierror.Write(w, apierror.Unauthorized("INVALID_EXTERNAL_SECRET", "external status secret is missing or invalid"))
		return
	}

	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		apierror.Write(w, apierror.NotFound("service order not found"))
		return
	}

	request, apiError := decodeJSON[ExternalStatusUpdateRequest](r)
	if apiError != nil {
		apierror.Write(w, apiError)
		return
	}
	if details := request.Validate(); len(details) > 0 {
		apierror.Write(w, apierror.Validation("invalid status update", details...))
		return
	}

	result, err := handler.useCases.Lifecycle.UpdateStatusExternal(r.Context(), id, domain.Status(request.Status), request.Notes)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, toStatusUpdateResponse(result))
}
