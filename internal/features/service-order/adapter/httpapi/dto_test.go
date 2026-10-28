package httpapi

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"automotive-workshop-api/internal/features/service-order/domain"
)

func validCreateRequest(products ...RequestedProductRequest) CreateRequest {
	return CreateRequest{CustomerID: uuid.NewString(), VehicleID: uuid.NewString(), RequestedProducts: products}
}

func TestCreateRequestValidateAcceptsRequestedProducts(t *testing.T) {
	request := validCreateRequest(
		RequestedProductRequest{ProductID: uuid.NewString(), Quantity: 2},
		RequestedProductRequest{ProductID: uuid.NewString(), Quantity: 1},
	)
	assert.Empty(t, request.Validate())
}

func TestCreateRequestValidateRejectsInvalidRequestedProducts(t *testing.T) {
	details := validCreateRequest(
		RequestedProductRequest{ProductID: "not-a-uuid", Quantity: 1},
		RequestedProductRequest{ProductID: uuid.NewString(), Quantity: 0},
		RequestedProductRequest{ProductID: uuid.NewString(), Quantity: -1},
	).Validate()

	fields := make([]string, 0, len(details))
	for _, detail := range details {
		fields = append(fields, detail.Field)
	}
	assert.ElementsMatch(t, []string{
		"requestedProducts[0].productId",
		"requestedProducts[1].quantity",
		"requestedProducts[2].quantity",
	}, fields)
}

func TestCreateRequestValidateRejectsDuplicateRequestedProduct(t *testing.T) {
	productID := uuid.NewString()
	details := validCreateRequest(
		RequestedProductRequest{ProductID: productID, Quantity: 1},
		RequestedProductRequest{ProductID: productID, Quantity: 4},
	).Validate()

	if assert.Len(t, details, 1) {
		assert.Equal(t, "requestedProducts[1].productId", details[0].Field)
		assert.Contains(t, details[0].Message, "duplicate product")
	}
}

// TestQuoteResponseRendersExactTotals keeps the JSON half of the FP-31 test
// TestComposeQuoteExactTotalsWithQuantities (specs/monetary-value-precision/):
// 3 × 19.90 + 7 × 0.10 = 60.40 renders as exact decimals, never float drift.
// The arithmetic half stays with the use case.
func TestQuoteResponseRendersExactTotals(t *testing.T) {
	quote := &domain.Quote{
		ID:             uuid.New(),
		ServiceOrderID: uuid.New(),
		Status:         domain.QuoteStatusPending,
		TotalAmount:    6040,
		Items: []domain.QuoteItem{
			{Kind: domain.QuoteItemProduct, ProductID: uuid.New(), Description: "A part", Quantity: 3, UnitPrice: 1990, Total: 5970},
			{Kind: domain.QuoteItemService, ServiceID: uuid.New(), Description: "A service", Quantity: 7, UnitPrice: 10, Total: 70},
		},
	}

	body, err := json.Marshal(toQuoteResponse(quote))
	require.NoError(t, err)
	assert.Contains(t, string(body), `"unitPrice":19.9,"total":59.7}`)
	assert.Contains(t, string(body), `"totalAmount":60.4,`)
}
