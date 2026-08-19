package servicecatalog

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"time"

	"automotive-workshop-api/internal/shared/httpx"
)

var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

type Handler struct {
	catalog *Catalog
}

func NewHandler(catalog *Catalog) *Handler { return &Handler{catalog: catalog} }

type createRequest struct {
	Code          *int64   `json:"code"`
	Name          string   `json:"name"`
	Description   string   `json:"description"`
	Price         *float64 `json:"price"`
	EstimatedTime *int     `json:"estimated_time"`
}

type updateRequest struct {
	Name          *string  `json:"name"`
	Description   *string  `json:"description"`
	Price         *float64 `json:"price"`
	EstimatedTime *int     `json:"estimated_time"`
}

type serviceResponse struct {
	ID            string    `json:"id"`
	Code          int64     `json:"code"`
	Name          string    `json:"name"`
	Description   string    `json:"description"`
	Price         float64   `json:"price"`
	EstimatedTime *int      `json:"estimated_time"`
	Active        bool      `json:"active"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type listResponse struct {
	Items []serviceResponse `json:"items"`
}

func toResponse(s *Service) serviceResponse {
	return serviceResponse{
		ID:            s.ID,
		Code:          s.Code,
		Name:          s.Name,
		Description:   s.Description,
		Price:         s.Price,
		EstimatedTime: s.EstimatedTime,
		Active:        s.Active,
		CreatedAt:     s.CreatedAt,
		UpdatedAt:     s.UpdatedAt,
	}
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var req createRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "INVALID_REQUEST", "malformed JSON body")
		return
	}
	if req.Price == nil {
		httpx.Error(w, http.StatusBadRequest, "INVALID_REQUEST", "price is required")
		return
	}
	created, err := h.catalog.Create(r.Context(), NewService{
		Code:          req.Code,
		Name:          req.Name,
		Description:   req.Description,
		Price:         *req.Price,
		EstimatedTime: req.EstimatedTime,
	})
	if err != nil {
		h.fail(w, "create", err)
		return
	}
	httpx.JSON(w, http.StatusCreated, toResponse(created))
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	var filter ListFilter
	if raw := r.URL.Query().Get("active"); raw != "" {
		active, err := strconv.ParseBool(raw)
		if err != nil {
			httpx.Error(w, http.StatusBadRequest, "INVALID_REQUEST", "active must be true or false")
			return
		}
		filter.Active = &active
	}
	services, err := h.catalog.List(r.Context(), filter)
	if err != nil {
		h.fail(w, "list", err)
		return
	}
	items := make([]serviceResponse, 0, len(services))
	for i := range services {
		items = append(items, toResponse(&services[i]))
	}
	httpx.JSON(w, http.StatusOK, listResponse{Items: items})
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := serviceID(w, r)
	if !ok {
		return
	}
	service, err := h.catalog.ByID(r.Context(), id)
	if err != nil {
		h.fail(w, "get", err)
		return
	}
	httpx.JSON(w, http.StatusOK, toResponse(service))
}

func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := serviceID(w, r)
	if !ok {
		return
	}
	var req updateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "INVALID_REQUEST", "malformed JSON body")
		return
	}
	updated, err := h.catalog.Update(r.Context(), id, Changes{
		Name:          req.Name,
		Description:   req.Description,
		Price:         req.Price,
		EstimatedTime: req.EstimatedTime,
	})
	if err != nil {
		h.fail(w, "update", err)
		return
	}
	httpx.JSON(w, http.StatusOK, toResponse(updated))
}

// Delete is a logical deletion: the service is deactivated, never removed (BR7).
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := serviceID(w, r)
	if !ok {
		return
	}
	if err := h.catalog.Deactivate(r.Context(), id); err != nil {
		h.fail(w, "delete", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// serviceID validates the path id before it reaches the database, so a malformed
// id answers 400 instead of surfacing a driver error as 500.
func serviceID(w http.ResponseWriter, r *http.Request) (string, bool) {
	id := r.PathValue("id")
	if !uuidPattern.MatchString(id) {
		httpx.Error(w, http.StatusBadRequest, "INVALID_REQUEST", "id must be a UUID")
		return "", false
	}
	return id, true
}

func (h *Handler) fail(w http.ResponseWriter, operation string, err error) {
	var validation ValidationError
	switch {
	case errors.As(err, &validation):
		httpx.Error(w, http.StatusBadRequest, "INVALID_REQUEST", validation.Message)
	case errors.Is(err, ErrServiceNotFound):
		httpx.Error(w, http.StatusNotFound, "SERVICE_NOT_FOUND", "service not found")
	case errors.Is(err, ErrCodeAlreadyExists):
		httpx.Error(w, http.StatusConflict, "CODE_ALREADY_EXISTS", "service code already exists")
	default:
		log.Printf("servicecatalog: %s failed: %v", operation, err)
		httpx.Error(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error")
	}
}
