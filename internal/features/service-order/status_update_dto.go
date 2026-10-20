package serviceorder

import (
	"strings"
	"unicode/utf8"

	"automotive-workshop-api/internal/shared/apierror"
)

// ExternalStatusUpdateRequest is the body of
// POST /api/v1/service-orders/{id}/status
// (specs/service-order-status-update/requirements.md §5).
type ExternalStatusUpdateRequest struct {
	Status string `json:"status"`
	Notes  string `json:"notes"`
}

// Validate performs the structural checks that need no order lookup
// (design.md §4 step 4): status inside the contract and notes within
// maxStatusUpdateNotesLength. UpdateStatusExternal repeats both checks.
func (request ExternalStatusUpdateRequest) Validate() []apierror.Detail {
	var details []apierror.Detail
	if !isExternalStatusTarget(Status(request.Status)) {
		details = append(details, apierror.Detail{Field: "status", Message: "status must be one of IN_DIAGNOSIS, COMPLETED, DELIVERED"})
	}
	if utf8.RuneCountInString(strings.TrimSpace(request.Notes)) > maxStatusUpdateNotesLength {
		details = append(details, apierror.Detail{Field: "notes", Message: "notes must be at most 500 characters"})
	}
	return details
}

// statusUpdateResponse is the reduced success response of the external
// status update (requirements.md BR8): just enough for the tool to confirm
// the outcome.
type statusUpdateResponse struct {
	ID             string `json:"id"`
	Code           int64  `json:"code"`
	PreviousStatus string `json:"previousStatus"`
	Status         string `json:"status"`
}

func toStatusUpdateResponse(result *StatusUpdateResult) statusUpdateResponse {
	return statusUpdateResponse{
		ID:             result.Order.ID.String(),
		Code:           result.Order.Code,
		PreviousStatus: string(result.PreviousStatus),
		Status:         string(result.Order.Status),
	}
}
