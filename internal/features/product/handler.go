package product

import (
	"net/http"

	"automotive-workshop-api/internal/shared/apierror"
)

const (
	defaultPage     = 1
	defaultPageSize = 20
	maxPageSize     = 100
)

// RegisterRoutes registers product catalog and stock endpoints on mux, applying requireAuth middleware if provided.
//
// Args:
//
//	mux(*http.ServeMux): HTTP request multiplexer
//	service(*ProductService): product service orchestrator
//	requireAuth(func(http.Handler) http.Handler): optional JWT authentication middleware
//
// Returns:
//
//	(none)
func RegisterRoutes(mux *http.ServeMux, service *ProductService, requireAuth func(http.Handler) http.Handler) {
	handler := &productHandler{service: service}

	wrap := func(h http.HandlerFunc) http.Handler {
		if requireAuth != nil {
			return requireAuth(h)
		}
		return h
	}

	mux.Handle("POST /api/v1/produtos", wrap(handler.create))
	mux.Handle("GET /api/v1/produtos", wrap(handler.list))
	mux.Handle("GET /api/v1/produtos/{id}", wrap(handler.getByID))
	mux.Handle("PATCH /api/v1/produtos/{id}", wrap(handler.update))
	mux.Handle("DELETE /api/v1/produtos/{id}", wrap(handler.deactivate))

	// Stock adjustment and balance routes (RF03, RF10, RNF07)
	mux.Handle("POST /api/v1/produtos/{id}/estoque/ajustes", wrap(handler.adjustStock))
	mux.Handle("GET /api/v1/produtos/{id}/estoque", wrap(handler.getStockBalance))
	mux.Handle("GET /api/v1/produtos/{id}/movimentacoes", wrap(handler.listMovements))
}

type productHandler struct {
	service *ProductService
}

func (handler *productHandler) create(w http.ResponseWriter, r *http.Request) {
	request, apiError := decodeJSON[CreateRequest](r)
	if apiError != nil {
		apierror.Write(w, apiError)
		return
	}

	if details := request.Validate(); len(details) > 0 {
		apierror.Write(w, apierror.Validation("invalid product data", details...))
		return
	}

	unitPrice := 0.0
	if request.UnitPrice != nil {
		unitPrice = *request.UnitPrice
	}

	currentStock := 0
	if request.CurrentStock != nil {
		currentStock = *request.CurrentStock
	}

	product, err := handler.service.Create(r.Context(), request.Code, request.Name, request.Description, unitPrice, currentStock, request.Type)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	w.Header().Set("Location", "/api/v1/produtos/"+product.ID.String())
	writeJSON(w, http.StatusCreated, toResponse(product))
}

func (handler *productHandler) getByID(w http.ResponseWriter, r *http.Request) {
	id, apiError := parseUUIDPathValue(r, "id")
	if apiError != nil {
		apierror.Write(w, apiError)
		return
	}

	product, err := handler.service.Get(r.Context(), id)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, toResponse(product))
}

func (handler *productHandler) list(w http.ResponseWriter, r *http.Request) {
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

	var productType *string
	if rawType := r.URL.Query().Get("type"); rawType != "" {
		productType = &rawType
	}

	var status *string
	if rawStatus := r.URL.Query().Get("status"); rawStatus != "" {
		status = &rawStatus
	}

	products, total, err := handler.service.List(r.Context(), page, pageSize, productType, status)
	if err != nil {
		apierror.Write(w, apierror.Internal("failed to list products"))
		return
	}

	responses := make([]Response, 0, len(products))
	for _, p := range products {
		responses = append(responses, toResponse(p))
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

func (handler *productHandler) update(w http.ResponseWriter, r *http.Request) {
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

	if details := request.Validate(); len(details) > 0 {
		apierror.Write(w, apierror.Validation("invalid product data", details...))
		return
	}

	product, err := handler.service.Update(r.Context(), id, UpdateInput{
		Name:         request.Name,
		Description:  request.Description,
		UnitPrice:    request.UnitPrice,
		CurrentStock: request.CurrentStock,
		Type:         request.Type,
	})
	if err != nil {
		writeServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, toResponse(product))
}

func (handler *productHandler) deactivate(w http.ResponseWriter, r *http.Request) {
	id, apiError := parseUUIDPathValue(r, "id")
	if apiError != nil {
		apierror.Write(w, apiError)
		return
	}

	product, err := handler.service.Deactivate(r.Context(), id)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, toResponse(product))
}

func (handler *productHandler) adjustStock(w http.ResponseWriter, r *http.Request) {
	id, apiError := parseUUIDPathValue(r, "id")
	if apiError != nil {
		apierror.Write(w, apiError)
		return
	}

	request, apiError := decodeJSON[StockAdjustmentRequest](r)
	if apiError != nil {
		apierror.Write(w, apiError)
		return
	}

	if details := request.Validate(); len(details) > 0 {
		apierror.Write(w, apierror.Validation("invalid stock adjustment data", details...))
		return
	}

	_, movement, err := handler.service.AdjustStock(r.Context(), id, request.Type, request.Quantity, request.Reason)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, toStockMovementResponse(movement))
}

func (handler *productHandler) getStockBalance(w http.ResponseWriter, r *http.Request) {
	id, apiError := parseUUIDPathValue(r, "id")
	if apiError != nil {
		apierror.Write(w, apiError)
		return
	}

	product, err := handler.service.GetStockBalance(r.Context(), id)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, toStockBalanceResponse(product))
}

func (handler *productHandler) listMovements(w http.ResponseWriter, r *http.Request) {
	id, apiError := parseUUIDPathValue(r, "id")
	if apiError != nil {
		apierror.Write(w, apiError)
		return
	}

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

	movements, total, err := handler.service.ListMovements(r.Context(), id, page, pageSize)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	responses := make([]StockMovementResponse, 0, len(movements))
	for _, m := range movements {
		responses = append(responses, toStockMovementResponse(m))
	}

	totalPages := 0
	if pageSize > 0 {
		totalPages = (total + pageSize - 1) / pageSize
	}

	writeJSON(w, http.StatusOK, StockMovementListResponse{
		Data:       responses,
		Page:       page,
		PageSize:   pageSize,
		Total:      total,
		TotalPages: totalPages,
	})
}
