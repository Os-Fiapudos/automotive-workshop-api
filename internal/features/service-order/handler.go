package serviceorder

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"automotive-workshop-api/internal/shared/apierror"
	"automotive-workshop-api/internal/shared/document"
)

const (
	defaultPage     = 1
	defaultPageSize = 20
	maxPageSize     = 100
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

	// Added by specs/service-order-query/ (RNF02: every route here requires
	// a valid JWT, unlike the create route above). {id} accepts either a
	// UUID or a sequential code (design.md §1.2) — a separate
	// GET .../code/{code} route is not implementable under this prefix, see
	// that section for why.
	mux.Handle("GET /api/v1/service-orders", wrap(handler.list))
	mux.Handle("GET /api/v1/service-orders/{id}", wrap(handler.get))

	// Added by specs/service-order-execution/ — all four requireAuth-wrapped,
	// per specs/auth/design.md §7's "every new route requires auth" convention.
	mux.Handle("POST /api/v1/service-orders/{id}/executions", wrap(handler.startExecution))
	mux.Handle("POST /api/v1/service-orders/{id}/executions/{executionId}/finish", wrap(handler.finishExecution))
	mux.Handle("POST /api/v1/service-orders/{id}/finalize", wrap(handler.finalizeOrder))
	mux.Handle("POST /api/v1/service-orders/{id}/deliver", wrap(handler.deliverOrder))
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

// list handles GET /api/v1/service-orders (specs/service-order-query/,
// requirements.md BR1-BR3).
func (handler *serviceOrderHandler) list(w http.ResponseWriter, r *http.Request) {
	page := parseIntParam(r, "page", defaultPage)
	if page < 1 {
		page = defaultPage
	}
	pageSize := parseIntParam(r, "pageSize", defaultPageSize)
	if pageSize < 1 {
		pageSize = defaultPageSize
	}
	if pageSize > maxPageSize {
		pageSize = maxPageSize
	}

	filter, details := parseListFilter(r)
	if len(details) > 0 {
		apierror.Write(w, apierror.Validation("invalid filter", details...))
		return
	}

	items, total, err := handler.service.List(r.Context(), filter, page, pageSize)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, toListResponse(items, page, pageSize, total))
}

// parseListFilter reads/validates the GET /api/v1/service-orders query
// filters (design.md §1.3). Structural validation (is "code" a number, is
// "status" one of the six known values, is "createdFrom"/"createdTo" a valid
// RFC3339 date-time) happens here, mirroring CreateRequest.Validate()'s
// handler-side call in this same package.
func parseListFilter(r *http.Request) (ListFilter, []apierror.Detail) {
	var filter ListFilter
	var details []apierror.Detail
	query := r.URL.Query()

	if raw := strings.TrimSpace(query.Get("code")); raw != "" {
		code, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			details = append(details, apierror.Detail{Field: "code", Message: "must be a valid integer"})
		} else {
			filter.Code = &code
		}
	}

	if raw := strings.TrimSpace(query.Get("status")); raw != "" {
		if !isKnownStatus(raw) {
			details = append(details, apierror.Detail{Field: "status", Message: "must be one of RECEBIDA, EM_DIAGNOSTICO, AGUARDANDO_APROVACAO, EM_EXECUCAO, FINALIZADA, ENTREGUE"})
		} else {
			filter.Status = &raw
		}
	}

	if raw := strings.TrimSpace(query.Get("document")); raw != "" {
		filter.CustomerDocument = document.Normalize(raw)
	}

	if raw := strings.TrimSpace(query.Get("licensePlate")); raw != "" {
		filter.LicensePlate = strings.ToUpper(raw)
	}

	if raw := strings.TrimSpace(query.Get("createdFrom")); raw != "" {
		parsed, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			details = append(details, apierror.Detail{Field: "createdFrom", Message: "must be an RFC3339 date-time"})
		} else {
			filter.CreatedFrom = &parsed
		}
	}

	if raw := strings.TrimSpace(query.Get("createdTo")); raw != "" {
		parsed, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			details = append(details, apierror.Detail{Field: "createdTo", Message: "must be an RFC3339 date-time"})
		} else {
			filter.CreatedTo = &parsed
		}
	}

	return filter, details
}

// get handles GET /api/v1/service-orders/{id} (specs/service-order-query/).
// {id} accepts either the order's technical UUID or its sequential code — a
// UUID and a bare integer can never be mistaken for each other, so trying
// uuid.Parse first and falling back to strconv.ParseInt is unambiguous
// (design.md §1.2/§1.9: a separate GET .../code/{code} route is not
// implementable under this prefix, this merged route replaces it).
func (handler *serviceOrderHandler) get(w http.ResponseWriter, r *http.Request) {
	rawID := r.PathValue("id")

	if id, err := uuid.Parse(rawID); err == nil {
		detail, err := handler.service.GetDetail(r.Context(), id)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, toDetailResponse(detail))
		return
	}

	code, err := strconv.ParseInt(rawID, 10, 64)
	if err != nil {
		apierror.Write(w, apierror.NotFound("service order not found"))
		return
	}

	detail, err := handler.service.GetDetailByCode(r.Context(), code)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, toDetailResponse(detail))
}

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

	execution, err := handler.service.StartExecution(r.Context(), id, serviceID)
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

	execution, err := handler.service.FinishExecution(r.Context(), id, executionID, request.EndedAt)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, toServiceExecutionResponse(execution))
}

// finalizeOrder handles POST /api/v1/service-orders/{id}/finalize.
func (handler *serviceOrderHandler) finalizeOrder(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		apierror.Write(w, apierror.NotFound("service order not found"))
		return
	}

	order, err := handler.service.FinalizeOrder(r.Context(), id)
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

	order, err := handler.service.DeliverOrder(r.Context(), id)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, toServiceOrderStatusResponse(order))
}
