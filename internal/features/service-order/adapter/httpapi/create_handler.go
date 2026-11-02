package httpapi

import (
	"net/http"

	"automotive-workshop-api/internal/shared/apierror"
)

func (handler *serviceOrderHandler) create(w http.ResponseWriter, r *http.Request) {
	request, apiError := decodeJSON[CreateRequest](r)
	if apiError != nil {
		apierror.Write(w, apiError)
		return
	}

	if details := request.Validate(); len(details) > 0 {
		apierror.Write(w, apierror.Validation("invalid service order data", details...))
		return
	}

	result, err := handler.useCases.Opening.Create(r.Context(), request.toInput())
	if err != nil {
		writeServiceError(w, err)
		return
	}

	w.Header().Set("Location", "/api/v1/service-orders/"+result.Order.ID.String())
	writeJSON(w, http.StatusCreated, toResponse(result))
}
