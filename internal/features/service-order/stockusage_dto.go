package serviceorder

import (
	"strings"
	"time"

	"automotive-workshop-api/internal/shared/apierror"
)

// stockUsageItemRequest is a single line of a RegisterStockUsageRequest.
type stockUsageItemRequest struct {
	ProductID string `json:"productId"`
	Quantity  int    `json:"quantity"`
}

// RegisterStockUsageRequest is the POST .../stock-movements request body.
type RegisterStockUsageRequest struct {
	Items []stockUsageItemRequest `json:"items"`
}

// Validate checks each item identifies a product and carries a shape the
// service layer can act on. Whether the list itself is non-empty, and
// whether quantities are positive, are business rules validated by the
// service layer (it needs no repository access to check) — same split
// ComposeQuoteRequest.Validate already documents for this package.
func (request RegisterStockUsageRequest) Validate() []apierror.Detail {
	var details []apierror.Detail
	for index, item := range request.Items {
		if strings.TrimSpace(item.ProductID) == "" {
			details = append(details, apierror.Detail{
				Field:   itemField(index) + ".productId",
				Message: "productId is required",
			})
		}
	}
	return details
}

func (request RegisterStockUsageRequest) toItems() []StockUsageItem {
	items := make([]StockUsageItem, 0, len(request.Items))
	for _, item := range request.Items {
		items = append(items, StockUsageItem{ProductID: item.ProductID, Quantity: item.Quantity})
	}
	return items
}

// StockMovementResponse is a StockMovement's JSON representation.
type StockMovementResponse struct {
	ID                 string    `json:"id"`
	ProductID          string    `json:"productId"`
	ServiceOrderID     *string   `json:"serviceOrderId,omitempty"`
	Type               string    `json:"type"`
	Quantity           int       `json:"quantity"`
	PreviousStock      int       `json:"previousStock"`
	NewStock           int       `json:"newStock"`
	Reason             string    `json:"reason,omitempty"`
	ReversedMovementID *string   `json:"reversedMovementId,omitempty"`
	OccurredAt         time.Time `json:"occurredAt"`
}

func toStockMovementResponse(movement *StockMovement) StockMovementResponse {
	response := StockMovementResponse{
		ID:            movement.ID.String(),
		ProductID:     movement.ProductID.String(),
		Type:          string(movement.Type),
		Quantity:      movement.Quantity,
		PreviousStock: movement.PreviousStock,
		NewStock:      movement.NewStock,
		Reason:        movement.Reason,
		OccurredAt:    movement.OccurredAt,
	}
	if movement.ServiceOrderID != nil {
		id := movement.ServiceOrderID.String()
		response.ServiceOrderID = &id
	}
	if movement.ReversedMovementID != nil {
		id := movement.ReversedMovementID.String()
		response.ReversedMovementID = &id
	}
	return response
}

// StockMovementListResponse is the GET .../stock-movements and POST
// .../stock-movements response body — the {"items": [...]} envelope
// CLAUDE.md §8 asks new list endpoints to use.
type StockMovementListResponse struct {
	Items []StockMovementResponse `json:"items"`
}

func toStockMovementListResponse(movements []*StockMovement) StockMovementListResponse {
	items := make([]StockMovementResponse, 0, len(movements))
	for _, movement := range movements {
		items = append(items, toStockMovementResponse(movement))
	}
	return StockMovementListResponse{Items: items}
}
