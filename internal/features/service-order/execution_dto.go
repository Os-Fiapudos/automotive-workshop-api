package serviceorder

import (
	"strings"
	"time"

	"automotive-workshop-api/internal/shared/apierror"
)

// StartExecutionRequest is the POST .../executions request body.
type StartExecutionRequest struct {
	ServiceID string `json:"serviceId"`
}

// Validate checks serviceId is present. Whether it identifies a real
// catalog service is a service-layer concern (design.md §2.4), same split
// as CreateRequest.Validate.
func (request StartExecutionRequest) Validate() []apierror.Detail {
	if strings.TrimSpace(request.ServiceID) == "" {
		return []apierror.Detail{{Field: "serviceId", Message: "serviceId is required"}}
	}
	return nil
}

// FinishExecutionRequest is the POST .../executions/{executionId}/finish
// request body. EndedAt is optional — a nil value means "now" (design.md
// §2.2/§2.5).
type FinishExecutionRequest struct {
	EndedAt *time.Time `json:"endedAt,omitempty"`
}

// ServiceExecutionResponse is a ServiceExecution's JSON representation.
type ServiceExecutionResponse struct {
	ID             string     `json:"id"`
	ServiceOrderID string     `json:"serviceOrderId"`
	ServiceID      string     `json:"serviceId"`
	StartedAt      time.Time  `json:"startedAt"`
	EndedAt        *time.Time `json:"endedAt,omitempty"`
}

func toServiceExecutionResponse(execution *ServiceExecution) ServiceExecutionResponse {
	return ServiceExecutionResponse{
		ID:             execution.ID.String(),
		ServiceOrderID: execution.ServiceOrderID.String(),
		ServiceID:      execution.ServiceID.String(),
		StartedAt:      execution.StartedAt,
		EndedAt:        execution.EndedAt,
	}
}
