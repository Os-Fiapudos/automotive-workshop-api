package vehicle

import (
	"strings"
	"time"

	"automotive-workshop-api/internal/shared/apierror"
)

// CreateRequest is the POST /api/v1/vehicles request body.
type CreateRequest struct {
	LicensePlate string `json:"licensePlate"`
	Brand        string `json:"brand"`
	Model        string `json:"model"`
	Year         int    `json:"year"`
	Color        string `json:"color"`
	CustomerID   string `json:"customerId"`
}

// Validate checks that every required field is present. It only validates
// the request's shape — deeper validation (plate format, year range,
// customer existence/status) happens in the domain/service layer, not here,
// mirroring customer.CreateRequest.Validate() (specs/customer-management/design.md §5).
func (request CreateRequest) Validate() []apierror.Detail {
	var details []apierror.Detail
	if strings.TrimSpace(request.LicensePlate) == "" {
		details = append(details, apierror.Detail{Field: "licensePlate", Message: "is required"})
	}
	if strings.TrimSpace(request.Brand) == "" {
		details = append(details, apierror.Detail{Field: "brand", Message: "is required"})
	}
	if strings.TrimSpace(request.Model) == "" {
		details = append(details, apierror.Detail{Field: "model", Message: "is required"})
	}
	if request.Year == 0 {
		details = append(details, apierror.Detail{Field: "year", Message: "is required"})
	}
	if strings.TrimSpace(request.Color) == "" {
		details = append(details, apierror.Detail{Field: "color", Message: "is required"})
	}
	if strings.TrimSpace(request.CustomerID) == "" {
		details = append(details, apierror.Detail{Field: "customerId", Message: "is required"})
	}
	return details
}

// UpdateRequest is the PATCH /api/v1/vehicles/{id} request body. Every field
// is optional; a field not present in the JSON body is left nil and the
// corresponding vehicle field is left unchanged (partial update — see
// specs/vehicle-management/requirements.md §3.2). License plate and
// customerId are deliberately absent from this type: this feature has no
// endpoint capability that changes either after creation.
type UpdateRequest struct {
	Brand *string `json:"brand,omitempty"`
	Model *string `json:"model,omitempty"`
	Year  *int    `json:"year,omitempty"`
	Color *string `json:"color,omitempty"`
}

// Response is the JSON representation of a Vehicle returned by every
// endpoint in this feature.
type Response struct {
	ID           string    `json:"id"`
	Code         int64     `json:"code"`
	LicensePlate string    `json:"licensePlate"`
	Brand        string    `json:"brand"`
	Model        string    `json:"model"`
	Year         int       `json:"year"`
	Color        string    `json:"color"`
	CustomerID   string    `json:"customerId"`
	Status       string    `json:"status"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

func toResponse(vehicle *Vehicle) Response {
	return Response{
		ID:           vehicle.ID.String(),
		Code:         vehicle.Code,
		LicensePlate: vehicle.LicensePlate,
		Brand:        vehicle.Brand,
		Model:        vehicle.Model,
		Year:         vehicle.Year,
		Color:        vehicle.Color,
		CustomerID:   vehicle.CustomerID.String(),
		Status:       string(vehicle.Status),
		CreatedAt:    vehicle.CreatedAt,
		UpdatedAt:    vehicle.UpdatedAt,
	}
}

// ListResponse is the GET /api/v1/vehicles and
// GET /api/v1/customers/{customerId}/vehicles response body.
type ListResponse struct {
	Data       []Response `json:"data"`
	Page       int        `json:"page"`
	PageSize   int        `json:"pageSize"`
	Total      int        `json:"total"`
	TotalPages int        `json:"totalPages"`
}
