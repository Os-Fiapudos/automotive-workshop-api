package serviceorder

import (
	"strconv"
	"strings"
	"time"

	"automotive-workshop-api/internal/shared/apierror"
)

// quoteItemRequest is a single line of a ComposeQuoteRequest. Exactly one of
// ProductID/ServiceID must be present — see Validate. Quantity's
// greater-than-zero rule is enforced by the service layer
// (validateQuoteItems), same split as CreateRequest.Validate vs. the
// service-layer checks in specs/service-order-opening/design.md §1.5.
type quoteItemRequest struct {
	ProductID string `json:"productId,omitempty"`
	ServiceID string `json:"serviceId,omitempty"`
	Quantity  int    `json:"quantity"`
}

// ComposeQuoteRequest is the PUT /api/v1/service-orders/{id}/quote request
// body. There is deliberately no total/totalAmount field anywhere in this
// struct — the server is the only author of a quote's total (RF06,
// requirements.md §3.7); a client-sent total has nothing to bind to even if
// present.
type ComposeQuoteRequest struct {
	Items []quoteItemRequest `json:"items"`
}

// Validate checks each item identifies exactly one catalog reference.
// Whether the list itself is non-empty, and whether quantities are
// positive, are business rules validated by the service layer (it needs no
// repository access to check, but keeping the split consistent with
// CreateRequest avoids two different validation conventions in the same
// package).
func (request ComposeQuoteRequest) Validate() []apierror.Detail {
	var details []apierror.Detail
	for index, item := range request.Items {
		hasProductID := strings.TrimSpace(item.ProductID) != ""
		hasServiceID := strings.TrimSpace(item.ServiceID) != ""
		if hasProductID == hasServiceID {
			details = append(details, apierror.Detail{
				Field:   itemField(index),
				Message: "exactly one of productId or serviceId is required",
			})
		}
	}
	return details
}

func itemField(index int) string {
	return "items[" + strconv.Itoa(index) + "]"
}

func (request ComposeQuoteRequest) toInputs() []QuoteItemInput {
	inputs := make([]QuoteItemInput, 0, len(request.Items))
	for _, item := range request.Items {
		if strings.TrimSpace(item.ProductID) != "" {
			inputs = append(inputs, QuoteItemInput{Kind: QuoteItemProduct, ProductID: item.ProductID, Quantity: item.Quantity})
		} else {
			inputs = append(inputs, QuoteItemInput{Kind: QuoteItemService, ServiceID: item.ServiceID, Quantity: item.Quantity})
		}
	}
	return inputs
}

// quoteItemResponse is a QuoteItem's JSON representation.
type quoteItemResponse struct {
	ProductID   string  `json:"productId,omitempty"`
	ServiceID   string  `json:"serviceId,omitempty"`
	Description string  `json:"description"`
	Quantity    int     `json:"quantity"`
	UnitPrice   float64 `json:"unitPrice"`
	Total       float64 `json:"total"`
}

// QuoteResponse is the JSON representation of a Quote. Version/SentAt/
// SentVersion are added by specs/service-order-quote-decision/.
type QuoteResponse struct {
	ID             string              `json:"id"`
	Code           int64               `json:"code"`
	ServiceOrderID string              `json:"serviceOrderId"`
	TotalAmount    float64             `json:"totalAmount"`
	Status         string              `json:"status"`
	Version        int                 `json:"version"`
	Items          []quoteItemResponse `json:"items"`
	GeneratedAt    time.Time           `json:"generatedAt"`
	SentAt         *time.Time          `json:"sentAt,omitempty"`
	SentVersion    *int                `json:"sentVersion,omitempty"`
	RespondedAt    *time.Time          `json:"respondedAt,omitempty"`
}

func toQuoteResponse(quote *Quote) QuoteResponse {
	items := make([]quoteItemResponse, 0, len(quote.Items))
	for _, item := range quote.Items {
		response := quoteItemResponse{
			Description: item.Description,
			Quantity:    item.Quantity,
			UnitPrice:   item.UnitPrice,
			Total:       item.Total,
		}
		if item.Kind == QuoteItemProduct {
			response.ProductID = item.ProductID.String()
		} else {
			response.ServiceID = item.ServiceID.String()
		}
		items = append(items, response)
	}

	return QuoteResponse{
		ID:             quote.ID.String(),
		Code:           quote.Code,
		ServiceOrderID: quote.ServiceOrderID.String(),
		TotalAmount:    quote.TotalAmount,
		Status:         string(quote.Status),
		Version:        quote.Version,
		Items:          items,
		GeneratedAt:    quote.GeneratedAt,
		SentAt:         quote.SentAt,
		SentVersion:    quote.SentVersion,
		RespondedAt:    quote.RespondedAt,
	}
}

// quoteDecisionResponse is the public, customer-facing response for
// POST /api/v1/acompanhamento/{codigo}/orcamento/aprovar|reprovar —
// deliberately a reduced projection distinct from the administrative
// QuoteResponse (no internal ids, no item breakdown), same "reduced read
// projection" principle service-order-tracking's trackingResponse already
// established for this same unauthenticated, customer-facing surface.
type quoteDecisionResponse struct {
	Code        int64      `json:"code"`
	QuoteStatus string     `json:"quoteStatus"`
	OrderStatus string     `json:"orderStatus"`
	RespondedAt *time.Time `json:"respondedAt"`
}

func toQuoteDecisionResponse(order *ServiceOrder, quote *Quote) quoteDecisionResponse {
	return quoteDecisionResponse{
		Code:        order.Code,
		QuoteStatus: string(quote.Status),
		OrderStatus: string(order.Status),
		RespondedAt: quote.RespondedAt,
	}
}

// serviceOrderStatusResponse is the minimal response for the diagnosis
// endpoint — just the order's id and new status, not the full
// service-order-opening Response shape (which also needs customer/vehicle
// display data this use case doesn't resolve).
type serviceOrderStatusResponse struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

func toServiceOrderStatusResponse(order *ServiceOrder) serviceOrderStatusResponse {
	return serviceOrderStatusResponse{ID: order.ID.String(), Status: string(order.Status)}
}
