package httpapi

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
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
