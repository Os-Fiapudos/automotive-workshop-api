package httpapi

import (
	"net/http"
	"strconv"

	"github.com/google/uuid"

	"automotive-workshop-api/internal/features/service-order/domain"
	"automotive-workshop-api/internal/shared/apierror"
)

// approveQuote handles
// POST /api/v1/acompanhamento/{codigo}/orcamento/aprovar
// (specs/service-order-quote-decision/). Never wrapped in requireAuth — it
// validates its own tracking token instead (RF12), same convention as
// service-order-tracking's GET /api/v1/acompanhamento/{codigo}.
func (handler *serviceOrderHandler) approveQuote(w http.ResponseWriter, r *http.Request) {
	code, err := strconv.ParseInt(r.PathValue("codigo"), 10, 64)
	if err != nil {
		// A non-numeric {codigo} can never match a real service_orders.code,
		// same reasoning service-order-tracking's handler already applies.
		apierror.Write(w, apierror.NotFound("service order not found"))
		return
	}

	order, quote, err := handler.useCases.Quotes.ApproveQuote(r.Context(), code, r.Header.Get(trackingTokenHeader))
	if err != nil {
		writeServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, toQuoteDecisionResponse(order, quote))
}

// rejectQuote handles
// POST /api/v1/acompanhamento/{codigo}/orcamento/reprovar
// (specs/service-order-quote-decision/). See approveQuote for the
// authentication rationale.
func (handler *serviceOrderHandler) rejectQuote(w http.ResponseWriter, r *http.Request) {
	code, err := strconv.ParseInt(r.PathValue("codigo"), 10, 64)
	if err != nil {
		apierror.Write(w, apierror.NotFound("service order not found"))
		return
	}

	order, quote, err := handler.useCases.Quotes.RejectQuote(r.Context(), code, r.Header.Get(trackingTokenHeader))
	if err != nil {
		writeServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, toQuoteDecisionResponse(order, quote))
}

// decideQuoteExternal handles POST /api/v1/service-orders/{id}/quote/decision
// (specs/service-order-quote-decision/design.md §5.3). The shared secret is
// checked first — before {id} or the body are even looked at — so a caller
// with a missing/wrong secret gets a 401 and learns nothing about whether
// the order exists, and no decision is applied (AC17).
func (handler *serviceOrderHandler) decideQuoteExternal(w http.ResponseWriter, r *http.Request) {
	if !handler.useCases.Quotes.AuthenticateExternalNotification(r.Header.Get(externalNotificationSecretHeader)) {
		apierror.Write(w, apierror.Unauthorized("INVALID_EXTERNAL_SECRET", "external notification secret is missing or invalid"))
		return
	}

	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		apierror.Write(w, apierror.NotFound("service order not found"))
		return
	}

	request, apiError := decodeJSON[ExternalQuoteDecisionRequest](r)
	if apiError != nil {
		apierror.Write(w, apiError)
		return
	}
	decision, ok := request.toQuoteStatus()
	if !ok {
		writeServiceError(w, domain.ErrInvalidQuoteDecision)
		return
	}

	order, quote, err := handler.useCases.Quotes.DecideQuoteExternal(r.Context(), id, decision)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, toQuoteDecisionResponse(order, quote))
}
