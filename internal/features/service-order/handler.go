package serviceorder

import (
	"net/http"

	"automotive-workshop-api/internal/shared/apierror"
)

// RegisterRoutes registers every Service Order Opening endpoint on mux,
// using the same Go 1.22 method-aware ServeMux pattern as every other
// feature (see specs/service-order-opening/design.md §1.5).
func RegisterRoutes(mux *http.ServeMux, service *ServiceOrderService) {
	handler := &serviceOrderHandler{service: service}

	mux.HandleFunc("POST /api/v1/service-orders", handler.create)
}

type serviceOrderHandler struct {
	service *ServiceOrderService
}

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

	result, err := handler.service.Create(r.Context(), request.toInput())
	if err != nil {
		writeServiceError(w, err)
		return
	}

	w.Header().Set("Location", "/api/v1/service-orders/"+result.Order.ID.String())
	writeJSON(w, http.StatusCreated, toResponse(result))
}
