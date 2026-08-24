package product

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateRequestValidate(t *testing.T) {
	validPrice := 10.0
	validStock := 5

	t.Run("valid request has no details", func(t *testing.T) {
		request := CreateRequest{Name: "Filtro", UnitPrice: &validPrice, CurrentStock: &validStock, Type: "PART"}
		assert.Empty(t, request.Validate())
	})

	t.Run("blank name is rejected", func(t *testing.T) {
		request := CreateRequest{Name: "   ", UnitPrice: &validPrice, CurrentStock: &validStock, Type: "PART"}
		details := request.Validate()
		require.Len(t, details, 1)
		assert.Equal(t, "name", details[0].Field)
	})

	t.Run("missing unit price and stock are rejected", func(t *testing.T) {
		request := CreateRequest{Name: "Filtro", Type: "PART"}
		details := request.Validate()
		require.Len(t, details, 2)
		assert.Equal(t, "unitPrice", details[0].Field)
		assert.Equal(t, "currentStock", details[1].Field)
		assert.Equal(t, "is required", details[0].Message)
	})

	t.Run("negative unit price and stock are rejected", func(t *testing.T) {
		negativePrice := -1.0
		negativeStock := -1
		request := CreateRequest{Name: "Filtro", UnitPrice: &negativePrice, CurrentStock: &negativeStock, Type: "PART"}
		details := request.Validate()
		require.Len(t, details, 2)
		assert.Equal(t, "cannot be negative", details[0].Message)
		assert.Equal(t, "cannot be negative", details[1].Message)
	})

	t.Run("non-positive explicit code is rejected", func(t *testing.T) {
		code := int64(0)
		request := CreateRequest{Name: "Filtro", UnitPrice: &validPrice, CurrentStock: &validStock, Type: "PART", Code: &code}
		details := request.Validate()
		require.Len(t, details, 1)
		assert.Equal(t, "code", details[0].Field)
	})

	t.Run("unknown type is rejected", func(t *testing.T) {
		request := CreateRequest{Name: "Filtro", UnitPrice: &validPrice, CurrentStock: &validStock, Type: "GADGET"}
		details := request.Validate()
		require.Len(t, details, 1)
		assert.Equal(t, "type", details[0].Field)
	})
}

func TestUpdateRequestValidate(t *testing.T) {
	t.Run("empty request has no details", func(t *testing.T) {
		assert.Empty(t, UpdateRequest{}.Validate())
	})

	// RNF07: stock never moves through the cadastral update.
	t.Run("currentStock is rejected", func(t *testing.T) {
		stock := 10
		details := UpdateRequest{CurrentStock: &stock}.Validate()
		require.Len(t, details, 1)
		assert.Equal(t, "currentStock", details[0].Field)
		assert.Equal(t, "cannot be modified via cadastral update", details[0].Message)
	})

	t.Run("blank name is rejected", func(t *testing.T) {
		blank := "  "
		details := UpdateRequest{Name: &blank}.Validate()
		require.Len(t, details, 1)
		assert.Equal(t, "name", details[0].Field)
	})

	t.Run("negative unit price is rejected", func(t *testing.T) {
		negative := -0.01
		details := UpdateRequest{UnitPrice: &negative}.Validate()
		require.Len(t, details, 1)
		assert.Equal(t, "unitPrice", details[0].Field)
	})

	t.Run("unknown type is rejected", func(t *testing.T) {
		unknown := "GADGET"
		details := UpdateRequest{Type: &unknown}.Validate()
		require.Len(t, details, 1)
		assert.Equal(t, "type", details[0].Field)
	})

	t.Run("known type is accepted", func(t *testing.T) {
		known := "SUPPLY"
		assert.Empty(t, UpdateRequest{Type: &known}.Validate())
	})
}

func TestStockAdjustmentRequestValidate(t *testing.T) {
	t.Run("valid request has no details", func(t *testing.T) {
		request := StockAdjustmentRequest{Type: "ENTRY", Quantity: 3, Reason: "Reposição"}
		assert.Empty(t, request.Validate())
	})

	t.Run("every field can fail at once", func(t *testing.T) {
		request := StockAdjustmentRequest{Type: "TRANSFER", Quantity: 0, Reason: "  "}
		details := request.Validate()
		require.Len(t, details, 3)
		assert.Equal(t, "type", details[0].Field)
		assert.Equal(t, "quantity", details[1].Field)
		assert.Equal(t, "reason", details[2].Field)
	})

	t.Run("negative quantity is rejected", func(t *testing.T) {
		request := StockAdjustmentRequest{Type: "EXIT", Quantity: -5, Reason: "Baixa"}
		details := request.Validate()
		require.Len(t, details, 1)
		assert.Equal(t, "quantity", details[0].Field)
	})
}
