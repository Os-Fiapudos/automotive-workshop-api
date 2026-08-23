package customer

import (
	"strings"
	"time"

	"automotive-workshop-api/internal/shared/apierror"
)

// CreateRequest is the POST /api/v1/customers request body.
type CreateRequest struct {
	Name     string  `json:"name"`
	Document string  `json:"document"`
	Phone    string  `json:"phone"`
	Email    *string `json:"email,omitempty"`
}

// Validate checks that every required field is present. It only validates
// the request's shape (required fields non-empty) — deeper validation
// (CPF/CNPJ check digits, document uniqueness) happens in the domain/service
// layer, not here, since it needs more than the request alone to decide.
func (request CreateRequest) Validate() []apierror.Detail {
	var details []apierror.Detail
	if strings.TrimSpace(request.Name) == "" {
		details = append(details, apierror.Detail{Field: "name", Message: "is required"})
	}
	if strings.TrimSpace(request.Document) == "" {
		details = append(details, apierror.Detail{Field: "document", Message: "is required"})
	}
	if strings.TrimSpace(request.Phone) == "" {
		details = append(details, apierror.Detail{Field: "phone", Message: "is required"})
	}
	return details
}

// UpdateRequest is the PATCH /api/v1/customers/{id} request body. Every
// field is optional; a field not present in the JSON body is left nil and
// the corresponding customer field is left unchanged (partial update, see
// specs/customer-management/requirements.md §3.9).
type UpdateRequest struct {
	Name     *string `json:"name,omitempty"`
	Document *string `json:"document,omitempty"`
	Phone    *string `json:"phone,omitempty"`
	Email    *string `json:"email,omitempty"`
}

// Response is the JSON representation of a Customer returned by every
// endpoint in this feature.
type Response struct {
	ID           string    `json:"id"`
	Code         int64     `json:"code"`
	Name         string    `json:"name"`
	Document     string    `json:"document"`
	DocumentType string    `json:"documentType"`
	Phone        string    `json:"phone"`
	Email        *string   `json:"email,omitempty"`
	Status       string    `json:"status"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

func toResponse(customer *Customer) Response {
	return Response{
		ID:           customer.ID.String(),
		Code:         customer.Code,
		Name:         customer.Name,
		Document:     customer.Document.Value,
		DocumentType: string(customer.Document.Type),
		Phone:        customer.Phone,
		Email:        customer.Email,
		Status:       string(customer.Status),
		CreatedAt:    customer.CreatedAt,
		UpdatedAt:    customer.UpdatedAt,
	}
}

// ListResponse is the GET /api/v1/customers response body.
type ListResponse struct {
	Data       []Response `json:"data"`
	Page       int        `json:"page"`
	PageSize   int        `json:"pageSize"`
	Total      int        `json:"total"`
	TotalPages int        `json:"totalPages"`
}
