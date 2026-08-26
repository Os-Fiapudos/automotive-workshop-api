package customer

import (
	"net/http"

	"automotive-workshop-api/internal/shared/apierror"
)

const (
	defaultPage     = 1
	defaultPageSize = 20
	maxPageSize     = 100
)

// RegisterRoutes registers every Customer Management endpoint on mux, using
// Go 1.22's method-aware ServeMux patterns (see
// specs/customer-management/design.md §1.5) — no third-party router.
// requireAuth wraps every route (RNF02 / specs/auth/design.md §7's "every
// non-public route requires auth" convention — see CLAUDE.md §17.2, the open
// decision this closes); it may be nil, same nil-safe convention as
// internal/features/product.RegisterRoutes, for callers that deliberately
// need an unauthenticated router (e.g. isolated fixture setup in tests).
func RegisterRoutes(mux *http.ServeMux, service *CustomerService, requireAuth func(http.Handler) http.Handler) {
	handler := &customerHandler{service: service}

	wrap := func(h http.HandlerFunc) http.Handler {
		if requireAuth != nil {
			return requireAuth(h)
		}
		return h
	}

	mux.Handle("POST /api/v1/customers", wrap(handler.create))
	mux.Handle("GET /api/v1/customers", wrap(handler.list))
	mux.Handle("GET /api/v1/customers/document/{document}", wrap(handler.getByDocument))
	mux.Handle("GET /api/v1/customers/{id}", wrap(handler.getByID))
	mux.Handle("PATCH /api/v1/customers/{id}", wrap(handler.update))
	mux.Handle("DELETE /api/v1/customers/{id}", wrap(handler.deactivate))
}

// customerHandler holds only what every endpoint needs (the service).
// Request decoding/parsing and error → HTTP status mapping live in
// httpsupport.go, not here, so each method below reads as: parse input,
// call the service, write the response.
type customerHandler struct {
	service *CustomerService
}

func (handler *customerHandler) create(w http.ResponseWriter, r *http.Request) {
	request, apiError := decodeJSON[CreateRequest](r)
	if apiError != nil {
		apierror.Write(w, apiError)
		return
	}

	if details := request.Validate(); len(details) > 0 {
		apierror.Write(w, apierror.Validation("invalid customer data", details...))
		return
	}

	customer, err := handler.service.Create(r.Context(), request.Name, request.Document, request.Phone, request.Email)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	w.Header().Set("Location", "/api/v1/customers/"+customer.ID.String())
	writeJSON(w, http.StatusCreated, toResponse(customer))
}

func (handler *customerHandler) getByID(w http.ResponseWriter, r *http.Request) {
	id, apiError := parseUUIDPathValue(r, "id")
	if apiError != nil {
		apierror.Write(w, apiError)
		return
	}

	customer, err := handler.service.Get(r.Context(), id)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, toResponse(customer))
}

func (handler *customerHandler) getByDocument(w http.ResponseWriter, r *http.Request) {
	customer, err := handler.service.GetByDocument(r.Context(), r.PathValue("document"))
	if err != nil {
		writeServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, toResponse(customer))
}

func (handler *customerHandler) list(w http.ResponseWriter, r *http.Request) {
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

	customers, total, err := handler.service.List(r.Context(), page, pageSize)
	if err != nil {
		apierror.Write(w, apierror.Internal("failed to list customers"))
		return
	}

	responses := make([]Response, 0, len(customers))
	for _, customer := range customers {
		responses = append(responses, toResponse(customer))
	}

	totalPages := 0
	if pageSize > 0 {
		totalPages = (total + pageSize - 1) / pageSize
	}

	writeJSON(w, http.StatusOK, ListResponse{
		Data:       responses,
		Page:       page,
		PageSize:   pageSize,
		Total:      total,
		TotalPages: totalPages,
	})
}

func (handler *customerHandler) update(w http.ResponseWriter, r *http.Request) {
	id, apiError := parseUUIDPathValue(r, "id")
	if apiError != nil {
		apierror.Write(w, apiError)
		return
	}

	request, apiError := decodeJSON[UpdateRequest](r)
	if apiError != nil {
		apierror.Write(w, apiError)
		return
	}

	customer, err := handler.service.Update(r.Context(), id, UpdateInput{
		Name:     request.Name,
		Document: request.Document,
		Phone:    request.Phone,
		Email:    request.Email,
	})
	if err != nil {
		writeServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, toResponse(customer))
}

func (handler *customerHandler) deactivate(w http.ResponseWriter, r *http.Request) {
	id, apiError := parseUUIDPathValue(r, "id")
	if apiError != nil {
		apierror.Write(w, apiError)
		return
	}

	customer, err := handler.service.Deactivate(r.Context(), id)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, toResponse(customer))
}
