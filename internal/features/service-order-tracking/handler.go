package servicetracking

import (
	"net/http"
	"strconv"

	"automotive-workshop-api/internal/shared/apierror"
)

// trackingTokenHeader is the custom header the tracking token is sent in —
// deliberately not Authorization: Bearer, to keep it unambiguous from the
// administrative JWT (requirements.md §0 item 2).
const trackingTokenHeader = "X-Tracking-Token"

// RegisterRoutes registers the tracking endpoint on mux. This route is never
// wrapped in middleware.RequireAuth (requirements.md §4/AC10) — it validates
// its own tracking token instead.
func RegisterRoutes(mux *http.ServeMux, service *TrackingService) {
	handler := &trackingHandler{service: service}
	mux.HandleFunc("GET /api/v1/acompanhamento/{codigo}", handler.get)
}

type trackingHandler struct {
	service *TrackingService
}

func (handler *trackingHandler) get(w http.ResponseWriter, r *http.Request) {
	code, err := strconv.ParseInt(r.PathValue("codigo"), 10, 64)
	if err != nil {
		// A non-numeric {codigo} can never match a real service_orders.code,
		// same observable outcome as an unknown numeric one (design.md §8).
		apierror.Write(w, apierror.NotFound("service order not found"))
		return
	}

	rawToken := r.Header.Get(trackingTokenHeader)

	read, err := handler.service.Get(r.Context(), code, rawToken)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, toTrackingResponse(read))
}
