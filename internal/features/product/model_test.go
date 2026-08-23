package product

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewProductValidations(t *testing.T) {
	t.Run("creates valid PART product", func(t *testing.T) {
		p, err := NewProduct(nil, "Filtro de Óleo", "Filtro 1L", 35.90, 10, "PART")
		require.NoError(t, err)
		assert.Equal(t, "Filtro de Óleo", p.Name)
		assert.Equal(t, TypePart, p.Type)
		assert.Equal(t, StatusActive, p.Status)
		assert.True(t, p.IsActive())
	})

	t.Run("creates valid SUPPLY product", func(t *testing.T) {
		p, err := NewProduct(nil, "Óleo 5W30", "Óleo sintético", 45.00, 20, "SUPPLY")
		require.NoError(t, err)
		assert.Equal(t, TypeSupply, p.Type)
	})

	t.Run("rejects empty name", func(t *testing.T) {
		_, err := NewProduct(nil, "   ", "Desc", 10.0, 5, "PART")
		assert.Error(t, err)
	})

	t.Run("rejects negative unit price", func(t *testing.T) {
		_, err := NewProduct(nil, "Peça", "Desc", -5.0, 5, "PART")
		assert.ErrorIs(t, err, ErrInvalidUnitPrice)
	})

	t.Run("rejects negative stock", func(t *testing.T) {
		_, err := NewProduct(nil, "Peça", "Desc", 10.0, -1, "PART")
		assert.ErrorIs(t, err, ErrInvalidStock)
	})

	t.Run("rejects invalid product type", func(t *testing.T) {
		_, err := NewProduct(nil, "Peça", "Desc", 10.0, 5, "INVALID_TYPE")
		assert.ErrorIs(t, err, ErrInvalidType)
	})

	t.Run("deactivates product", func(t *testing.T) {
		p, err := NewProduct(nil, "Peça", "Desc", 10.0, 5, "PART")
		require.NoError(t, err)
		p.Deactivate()
		assert.Equal(t, StatusInactive, p.Status)
		assert.False(t, p.IsActive())
	})
}

func TestProductApplyStockAdjustment(t *testing.T) {
	t.Run("entry adjustment increases stock", func(t *testing.T) {
		p, err := NewProduct(nil, "Filtro de Óleo", "Desc", 35.0, 10, "PART")
		require.NoError(t, err)

		m, err := p.ApplyStockAdjustment(MovementTypeEntry, 5, "Recebimento de lote")
		require.NoError(t, err)

		assert.Equal(t, 15, p.CurrentStock)
		assert.Equal(t, 10, m.PreviousStock)
		assert.Equal(t, 15, m.NewStock)
		assert.Equal(t, MovementTypeEntry, m.Type)
		assert.Equal(t, "Recebimento de lote", m.Reason)
	})

	t.Run("exit adjustment decreases stock", func(t *testing.T) {
		p, err := NewProduct(nil, "Filtro de Óleo", "Desc", 35.0, 10, "PART")
		require.NoError(t, err)

		m, err := p.ApplyStockAdjustment(MovementTypeExit, 4, "Uso em serviço")
		require.NoError(t, err)

		assert.Equal(t, 6, p.CurrentStock)
		assert.Equal(t, 10, m.PreviousStock)
		assert.Equal(t, 6, m.NewStock)
		assert.Equal(t, MovementTypeExit, m.Type)
	})

	t.Run("rejects zero or negative quantity", func(t *testing.T) {
		p, err := NewProduct(nil, "Filtro de Óleo", "Desc", 35.0, 10, "PART")
		require.NoError(t, err)

		_, err = p.ApplyStockAdjustment(MovementTypeEntry, 0, "Ajuste")
		assert.ErrorIs(t, err, ErrInvalidQuantity)

		_, err = p.ApplyStockAdjustment(MovementTypeEntry, -5, "Ajuste")
		assert.ErrorIs(t, err, ErrInvalidQuantity)
	})

	t.Run("rejects empty reason", func(t *testing.T) {
		p, err := NewProduct(nil, "Filtro de Óleo", "Desc", 35.0, 10, "PART")
		require.NoError(t, err)

		_, err = p.ApplyStockAdjustment(MovementTypeEntry, 5, "   ")
		assert.ErrorIs(t, err, ErrEmptyReason)
	})

	t.Run("rejects exit exceeding current stock", func(t *testing.T) {
		p, err := NewProduct(nil, "Filtro de Óleo", "Desc", 35.0, 5, "PART")
		require.NoError(t, err)

		_, err = p.ApplyStockAdjustment(MovementTypeExit, 10, "Saída excessiva")
		assert.ErrorIs(t, err, ErrInsufficientStock)
		assert.Equal(t, 5, p.CurrentStock) // stock untouched
	})

	t.Run("rejects stock adjustment on inactive product", func(t *testing.T) {
		p, err := NewProduct(nil, "Filtro de Óleo", "Desc", 35.0, 10, "PART")
		require.NoError(t, err)
		p.Deactivate()

		_, err = p.ApplyStockAdjustment(MovementTypeEntry, 5, "Ajuste em inativo")
		assert.ErrorIs(t, err, ErrInactiveProduct)
	})
}
