package product

import (
	"strings"
	"time"

	"automotive-workshop-api/internal/shared/apierror"
)

// CreateRequest represents the payload for POST /api/v1/products.
type CreateRequest struct {
	Code         *int64   `json:"code,omitempty"`
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	UnitPrice    *float64 `json:"unitPrice"`
	CurrentStock *int     `json:"currentStock"`
	Type         string   `json:"type"`
}

// Validate checks fields of CreateRequest and returns any validation error details.
//
// Args:
//
//	(none)
//
// Returns:
//
//	details([]apierror.Detail): list of validation failure details
func (request CreateRequest) Validate() []apierror.Detail {
	var details []apierror.Detail

	if strings.TrimSpace(request.Name) == "" {
		details = append(details, apierror.Detail{Field: "name", Message: "is required"})
	}

	if request.UnitPrice == nil {
		details = append(details, apierror.Detail{Field: "unitPrice", Message: "is required"})
	} else if *request.UnitPrice < 0 {
		details = append(details, apierror.Detail{Field: "unitPrice", Message: "cannot be negative"})
	}

	if request.CurrentStock == nil {
		details = append(details, apierror.Detail{Field: "currentStock", Message: "is required"})
	} else if *request.CurrentStock < 0 {
		details = append(details, apierror.Detail{Field: "currentStock", Message: "cannot be negative"})
	}

	if request.Code != nil && *request.Code <= 0 {
		details = append(details, apierror.Detail{Field: "code", Message: "must be greater than zero"})
	}

	if _, err := ParseType(request.Type); err != nil {
		details = append(details, apierror.Detail{Field: "type", Message: "must be PART or SUPPLY"})
	}

	return details
}

// UpdateRequest represents the payload for PATCH /api/v1/products/{id}.
type UpdateRequest struct {
	Name         *string  `json:"name,omitempty"`
	Description  *string  `json:"description,omitempty"`
	UnitPrice    *float64 `json:"unitPrice,omitempty"`
	CurrentStock *int     `json:"currentStock,omitempty"`
	Type         *string  `json:"type,omitempty"`
}

// Validate checks fields of UpdateRequest and returns validation error details.
// RNF07: If currentStock is present in update payload, validation fails.
//
// Args:
//
//	(none)
//
// Returns:
//
//	details([]apierror.Detail): list of validation failure details
func (request UpdateRequest) Validate() []apierror.Detail {
	var details []apierror.Detail

	if request.CurrentStock != nil {
		details = append(details, apierror.Detail{Field: "currentStock", Message: "cannot be modified via cadastral update"})
	}

	if request.Name != nil && strings.TrimSpace(*request.Name) == "" {
		details = append(details, apierror.Detail{Field: "name", Message: "cannot be empty"})
	}

	if request.UnitPrice != nil && *request.UnitPrice < 0 {
		details = append(details, apierror.Detail{Field: "unitPrice", Message: "cannot be negative"})
	}

	if request.Type != nil {
		if _, err := ParseType(*request.Type); err != nil {
			details = append(details, apierror.Detail{Field: "type", Message: "must be PART or SUPPLY"})
		}
	}

	return details
}

// StockAdjustmentRequest represents the payload for POST /api/v1/products/{id}/stock/adjustments.
type StockAdjustmentRequest struct {
	Type     string `json:"type"`
	Quantity int    `json:"quantity"`
	Reason   string `json:"reason"`
}

// Validate checks fields of StockAdjustmentRequest.
//
// Args:
//
//	(none)
//
// Returns:
//
//	details([]apierror.Detail): validation details if any invalid field
func (request StockAdjustmentRequest) Validate() []apierror.Detail {
	var details []apierror.Detail

	if _, err := ParseMovementType(request.Type); err != nil {
		details = append(details, apierror.Detail{Field: "type", Message: "must be ENTRY or EXIT"})
	}

	if request.Quantity <= 0 {
		details = append(details, apierror.Detail{Field: "quantity", Message: "must be greater than zero"})
	}

	if strings.TrimSpace(request.Reason) == "" {
		details = append(details, apierror.Detail{Field: "reason", Message: "is required"})
	}

	return details
}

// Response represents the JSON structure of a product returned by the API.
type Response struct {
	ID           string    `json:"id"`
	Code         int64     `json:"code"`
	Name         string    `json:"name"`
	Description  string    `json:"description"`
	UnitPrice    float64   `json:"unitPrice"`
	CurrentStock int       `json:"currentStock"`
	Type         string    `json:"type"`
	Status       string    `json:"status"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

// StockBalanceResponse represents response for GET /api/v1/products/{id}/stock.
type StockBalanceResponse struct {
	ID           string    `json:"id"`
	Code         int64     `json:"code"`
	Name         string    `json:"name"`
	CurrentStock int       `json:"currentStock"`
	Status       string    `json:"status"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

// StockMovementResponse represents response item for GET /api/v1/products/{id}/movements and POST adjustment.
type StockMovementResponse struct {
	ID            string    `json:"id"`
	ProductID     string    `json:"productId"`
	Type          string    `json:"type"`
	Quantity      int       `json:"quantity"`
	PreviousStock int       `json:"previousStock"`
	NewStock      int       `json:"newStock"`
	Reason        string    `json:"reason"`
	CreatedAt     time.Time `json:"createdAt"`
}

// toResponse maps a Product domain aggregate to an external HTTP Response DTO.
func toResponse(product *Product) Response {
	return Response{
		ID:           product.ID.String(),
		Code:         product.Code,
		Name:         product.Name,
		Description:  product.Description,
		UnitPrice:    product.UnitPrice,
		CurrentStock: product.CurrentStock,
		Type:         string(product.Type),
		Status:       string(product.Status),
		CreatedAt:    product.CreatedAt,
		UpdatedAt:    product.UpdatedAt,
	}
}

// toStockBalanceResponse maps a Product to StockBalanceResponse DTO.
func toStockBalanceResponse(product *Product) StockBalanceResponse {
	return StockBalanceResponse{
		ID:           product.ID.String(),
		Code:         product.Code,
		Name:         product.Name,
		CurrentStock: product.CurrentStock,
		Status:       string(product.Status),
		UpdatedAt:    product.UpdatedAt,
	}
}

// toStockMovementResponse maps a StockMovement to StockMovementResponse DTO.
func toStockMovementResponse(m *StockMovement) StockMovementResponse {
	return StockMovementResponse{
		ID:            m.ID.String(),
		ProductID:     m.ProductID.String(),
		Type:          string(m.Type),
		Quantity:      m.Quantity,
		PreviousStock: m.PreviousStock,
		NewStock:      m.NewStock,
		Reason:        m.Reason,
		CreatedAt:     m.CreatedAt,
	}
}

// ListResponse represents the paginated list envelope.
type ListResponse struct {
	Data       []Response `json:"data"`
	Page       int        `json:"page"`
	PageSize   int        `json:"pageSize"`
	Total      int        `json:"total"`
	TotalPages int        `json:"totalPages"`
}

// StockMovementListResponse represents the paginated list envelope for movements.
type StockMovementListResponse struct {
	Data       []StockMovementResponse `json:"data"`
	Page       int                     `json:"page"`
	PageSize   int                     `json:"pageSize"`
	Total      int                     `json:"total"`
	TotalPages int                     `json:"totalPages"`
}
