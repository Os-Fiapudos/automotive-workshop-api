package vehicle

import (
	"net/http"

	"automotive-workshop-api/internal/shared/apierror"
)

// RegisterRoutes registers every Vehicle Management endpoint on mux, using
// Go 1.22's method-aware ServeMux patterns (mirrors customer.RegisterRoutes,
// specs/customer-management/design.md §1.5). Every route is wrapped in
// requireAuth (RNF02): unlike Customer Management, Vehicle Management's own
// requirements explicitly demand JWT on every route — the caller
// (cmd/api/main.go) passes the same middleware.RequireAuth(tokens) value
// already used for GET /api/v1/auth/me.
//
// The customer-scoped listing is GET /api/v1/vehicles/customer/{customerId},
// not the originally-specified GET /api/v1/customers/{customerId}/vehicles —
// the latter panics http.ServeMux at startup (ambiguous against customer's
// own GET /api/v1/customers/document/{document}); see
// specs/vehicle-management/design.md §1.5 for the full account.
func RegisterRoutes(mux *http.ServeMux, service *VehicleService, requireAuth func(http.Handler) http.Handler) {
	handler := &vehicleHandler{service: service}

	mux.Handle("POST /api/v1/vehicles", requireAuth(http.HandlerFunc(handler.create)))
	mux.Handle("GET /api/v1/vehicles", requireAuth(http.HandlerFunc(handler.list)))
	mux.Handle("GET /api/v1/vehicles/plate/{plate}", requireAuth(http.HandlerFunc(handler.getByPlate)))
	mux.Handle("GET /api/v1/vehicles/customer/{customerId}", requireAuth(http.HandlerFunc(handler.listByCustomer)))
	mux.Handle("GET /api/v1/vehicles/{id}", requireAuth(http.HandlerFunc(handler.getByID)))
	mux.Handle("PATCH /api/v1/vehicles/{id}", requireAuth(http.HandlerFunc(handler.update)))
	mux.Handle("DELETE /api/v1/vehicles/{id}", requireAuth(http.HandlerFunc(handler.deactivate)))
}

// vehicleHandler holds only what every endpoint needs (the service).
// Request decoding/parsing and error → HTTP status mapping live in
// httpsupport.go, not here, so each method below reads as: parse input,
// call the service, write the response.
type vehicleHandler struct {
	service *VehicleService
}

func (handler *vehicleHandler) create(w http.ResponseWriter, r *http.Request) {
	request, apiError := decodeJSON[CreateRequest](r)
	if apiError != nil {
		apierror.Write(w, apiError)
		return
	}

	if details := request.Validate(); len(details) > 0 {
		apierror.Write(w, apierror.Validation("invalid vehicle data", details...))
		return
	}

	customerID, detail := parseUUIDField("customerId", request.CustomerID)
	if detail != nil {
		apierror.Write(w, apierror.Validation("invalid vehicle data", *detail))
		return
	}

	vehicle, err := handler.service.Create(r.Context(), request.LicensePlate, request.Brand, request.Model, request.Year, request.Color, customerID)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	w.Header().Set("Location", "/api/v1/vehicles/"+vehicle.ID.String())
	writeJSON(w, http.StatusCreated, toResponse(vehicle))
}

func (handler *vehicleHandler) getByID(w http.ResponseWriter, r *http.Request) {
	id, apiError := parseUUIDPathValue(r, "id")
	if apiError != nil {
		apierror.Write(w, apiError)
		return
	}

	vehicle, err := handler.service.Get(r.Context(), id)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, toResponse(vehicle))
}

func (handler *vehicleHandler) getByPlate(w http.ResponseWriter, r *http.Request) {
	vehicle, err := handler.service.GetByPlate(r.Context(), r.PathValue("plate"))
	if err != nil {
		writeServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, toResponse(vehicle))
}

func (handler *vehicleHandler) list(w http.ResponseWriter, r *http.Request) {
	page, pageSize := parsePagination(r)

	vehicles, total, err := handler.service.List(r.Context(), page, pageSize)
	if err != nil {
		apierror.Write(w, apierror.Internal("failed to list vehicles"))
		return
	}

	writeJSON(w, http.StatusOK, toListResponse(vehicles, page, pageSize, total))
}

func (handler *vehicleHandler) listByCustomer(w http.ResponseWriter, r *http.Request) {
	customerID, apiError := parseUUIDPathValue(r, "customerId")
	if apiError != nil {
		apierror.Write(w, apiError)
		return
	}

	page, pageSize := parsePagination(r)

	vehicles, total, err := handler.service.ListByCustomer(r.Context(), customerID, page, pageSize)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, toListResponse(vehicles, page, pageSize, total))
}

func (handler *vehicleHandler) update(w http.ResponseWriter, r *http.Request) {
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

	vehicle, err := handler.service.Update(r.Context(), id, UpdateInput{
		Brand: request.Brand,
		Model: request.Model,
		Year:  request.Year,
		Color: request.Color,
	})
	if err != nil {
		writeServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, toResponse(vehicle))
}

func (handler *vehicleHandler) deactivate(w http.ResponseWriter, r *http.Request) {
	id, apiError := parseUUIDPathValue(r, "id")
	if apiError != nil {
		apierror.Write(w, apiError)
		return
	}

	vehicle, err := handler.service.Deactivate(r.Context(), id)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, toResponse(vehicle))
}
