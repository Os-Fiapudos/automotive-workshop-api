package servicecatalog

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"

	"automotive-workshop-api/internal/shared/apierror"
	"automotive-workshop-api/internal/shared/httpx"
)

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
		apierror.Write(w, apierror.BadRequest("request body is not valid JSON"))
		return
	}
	if req.Price == nil {
		apierror.Write(w, apierror.Validation("invalid service data",
			apierror.Detail{Field: "price", Message: "price is required"}))
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
			apierror.Write(w, apierror.Validation("invalid service filter",
				apierror.Detail{Field: "active", Message: "active must be true or false"}))
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
		apierror.Write(w, apierror.BadRequest("request body is not valid JSON"))
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
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		apierror.Write(w, apierror.BadRequest("id must be a valid UUID"))
		return "", false
	}
	return id.String(), true
}

func (h *Handler) fail(w http.ResponseWriter, operation string, err error) {
	var validation ValidationError
	switch {
	case errors.As(err, &validation):
		if validation.Field == "" {
			apierror.Write(w, apierror.Validation(validation.Message))
			return
		}
		apierror.Write(w, apierror.Validation("invalid service data",
			apierror.Detail{Field: validation.Field, Message: validation.Message}))
	case errors.Is(err, ErrServiceNotFound):
		apierror.Write(w, apierror.NotFound("service not found"))
	case errors.Is(err, ErrCodeAlreadyExists):
		apierror.Write(w, apierror.Conflict("CODE_ALREADY_EXISTS", "service code already exists"))
	default:
		log.Printf("servicecatalog: %s failed: %v", operation, err)
		apierror.Write(w, apierror.Internal("unexpected error"))
	}
}
