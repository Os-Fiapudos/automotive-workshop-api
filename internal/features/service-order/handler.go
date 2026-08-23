package serviceorder

import (
	"net/http"

	"github.com/google/uuid"

	"automotive-workshop-api/internal/shared/apierror"
)

// RegisterRoutes registers every Service Order Opening/Diagnosis/Quote
// endpoint on mux, using the same Go 1.22 method-aware ServeMux pattern as
// every other feature (see specs/service-order-opening/design.md §1.5).
// requireAuth wraps the diagnosis/quote routes added by
// specs/service-order-diagnosis-quote/ (requirements.md §7.4); the original
// order-creation route is left unwrapped, matching its own still-open
// authentication decision (CLAUDE.md §1). requireAuth may be nil, same
// nil-safe convention as internal/features/product.RegisterRoutes.
func RegisterRoutes(mux *http.ServeMux, service *ServiceOrderService, requireAuth func(http.Handler) http.Handler) {
	handler := &serviceOrderHandler{service: service}

	wrap := func(h http.HandlerFunc) http.Handler {
		if requireAuth != nil {
			return requireAuth(h)
		}
		return h
	}

	mux.HandleFunc("POST /api/v1/service-orders", handler.create)
	mux.Handle("POST /api/v1/service-orders/{id}/diagnosis", wrap(handler.startDiagnosis))
	mux.Handle("PUT /api/v1/service-orders/{id}/quote", wrap(handler.composeQuote))
	mux.Handle("GET /api/v1/service-orders/{id}/quote", wrap(handler.getQuote))
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

func (handler *serviceOrderHandler) startDiagnosis(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		apierror.Write(w, apierror.NotFound("service order not found"))
		return
	}

	order, err := handler.service.StartDiagnosis(r.Context(), id)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, toServiceOrderStatusResponse(order))
}

func (handler *serviceOrderHandler) composeQuote(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		apierror.Write(w, apierror.NotFound("service order not found"))
		return
	}

	request, apiError := decodeJSON[ComposeQuoteRequest](r)
	if apiError != nil {
		apierror.Write(w, apiError)
		return
	}

	if details := request.Validate(); len(details) > 0 {
		apierror.Write(w, apierror.Validation("invalid quote data", details...))
		return
	}

	quote, err := handler.service.ComposeQuote(r.Context(), id, request.toInputs())
	if err != nil {
		writeServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, toQuoteResponse(quote))
}

func (handler *serviceOrderHandler) getQuote(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		apierror.Write(w, apierror.NotFound("service order not found"))
		return
	}

	quote, err := handler.service.GetQuote(r.Context(), id)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, toQuoteResponse(quote))
}
