package httpapi

import (
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"automotive-workshop-api/internal/features/service-order/usecase"
	"automotive-workshop-api/internal/shared/apierror"
)

// averageExecutionTime handles
// GET /api/v1/service-orders/metrics/average-execution-time
// (specs/service-order-metrics/, requirements.md BR1-BR7).
func (handler *serviceOrderHandler) averageExecutionTime(w http.ResponseWriter, r *http.Request) {
	filter, details := parseMetricsFilter(r)
	if len(details) > 0 {
		apierror.Write(w, apierror.Validation("invalid filter", details...))
		return
	}

	metrics, err := handler.useCases.Metrics.AverageExecutionTime(r.Context(), filter)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, toAverageExecutionTimeResponse(metrics))
}

// parseMetricsFilter reads/validates the
// GET .../metrics/average-execution-time query filters
// (specs/service-order-metrics/design.md §1.3), mirroring
// parseListFilter's per-field apierror.Detail accumulation pattern.
func parseMetricsFilter(r *http.Request) (usecase.MetricsFilter, []apierror.Detail) {
	var filter usecase.MetricsFilter
	var details []apierror.Detail
	query := r.URL.Query()

	if raw := strings.TrimSpace(query.Get("serviceId")); raw != "" {
		serviceID, err := uuid.Parse(raw)
		if err != nil {
			details = append(details, apierror.Detail{Field: "serviceId", Message: "must be a valid UUID"})
		} else {
			filter.ServiceID = &serviceID
		}
	}

	if raw := strings.TrimSpace(query.Get("startDate")); raw != "" {
		parsed, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			details = append(details, apierror.Detail{Field: "startDate", Message: "must be an RFC3339 date-time"})
		} else {
			filter.StartDate = &parsed
		}
	}

	if raw := strings.TrimSpace(query.Get("endDate")); raw != "" {
		parsed, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			details = append(details, apierror.Detail{Field: "endDate", Message: "must be an RFC3339 date-time"})
		} else {
			filter.EndDate = &parsed
		}
	}

	return filter, details
}
