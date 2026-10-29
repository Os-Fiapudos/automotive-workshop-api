package httpapi

import (
	"net/http"

	"github.com/google/uuid"

	"automotive-workshop-api/internal/shared/apierror"
)

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

	quote, err := handler.useCases.Quotes.ComposeQuote(r.Context(), id, request.toInputs())
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

	quote, err := handler.useCases.Quotes.GetQuote(r.Context(), id)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, toQuoteResponse(quote))
}

// sendQuote handles POST /api/v1/service-orders/{id}/quote/send
// (specs/service-order-quote-decision/).
func (handler *serviceOrderHandler) sendQuote(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		apierror.Write(w, apierror.NotFound("service order not found"))
		return
	}

	quote, err := handler.useCases.Quotes.SendQuote(r.Context(), id)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, toQuoteResponse(quote))
}
