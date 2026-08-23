package serviceorder

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestOrder(t *testing.T, status Status) *ServiceOrder {
	t.Helper()
	order, err := NewServiceOrder(uuid.New(), uuid.New(), "", nil)
	require.NoError(t, err)
	order.Status = status
	return order
}

func TestStartDiagnosisFromRecebida(t *testing.T) {
	order := newTestOrder(t, StatusRecebida)
	require.NoError(t, order.startDiagnosis())
	assert.Equal(t, StatusEmDiagnostico, order.Status)
}

func TestStartDiagnosisRejectsNonRecebida(t *testing.T) {
	for _, status := range []Status{StatusEmDiagnostico, StatusAguardandoAprovacao} {
		order := newTestOrder(t, status)
		err := order.startDiagnosis()
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrInvalidStatusTransition)
		assert.Equal(t, status, order.Status, "status must not change on a rejected transition")
	}
}

func TestMarkAwaitingApprovalRejectsRecebida(t *testing.T) {
	order := newTestOrder(t, StatusRecebida)
	err := order.markAwaitingApproval()
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrDiagnosisNotStarted)
}

func TestMarkAwaitingApprovalFromEmDiagnostico(t *testing.T) {
	order := newTestOrder(t, StatusEmDiagnostico)
	require.NoError(t, order.markAwaitingApproval())
	assert.Equal(t, StatusAguardandoAprovacao, order.Status)
}

func TestMarkAwaitingApprovalIsIdempotentFromItself(t *testing.T) {
	order := newTestOrder(t, StatusAguardandoAprovacao)
	require.NoError(t, order.markAwaitingApproval())
	assert.Equal(t, StatusAguardandoAprovacao, order.Status)
}

func TestValidateQuoteItemsRejectsEmpty(t *testing.T) {
	err := validateQuoteItems(nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrEmptyQuote)
}

func TestValidateQuoteItemsRejectsInvalidQuantity(t *testing.T) {
	items := []QuoteItem{{Kind: QuoteItemProduct, Quantity: 0, UnitPrice: 10, Total: 0}}
	err := validateQuoteItems(items)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidQuantity)
}

func TestValidateQuoteItemsAcceptsValidItems(t *testing.T) {
	items := []QuoteItem{
		{Kind: QuoteItemProduct, Quantity: 2, UnitPrice: 10, Total: 20},
		{Kind: QuoteItemService, Quantity: 1, UnitPrice: 80, Total: 80},
	}
	require.NoError(t, validateQuoteItems(items))
}

func TestCalculateTotalSumsItems(t *testing.T) {
	items := []QuoteItem{
		{Total: 35.90},
		{Total: 48.50},
		{Total: 129.90},
	}
	assert.InDelta(t, 214.30, calculateTotal(items), 0.0001)
}

func TestCalculateTotalManyItemsNoDrift(t *testing.T) {
	items := make([]QuoteItem, 0, 1000)
	for i := 0; i < 1000; i++ {
		items = append(items, QuoteItem{Total: 0.10})
	}
	assert.InDelta(t, 100.0, calculateTotal(items), 0.0001)
}

func TestCalculateTotalEmpty(t *testing.T) {
	assert.Equal(t, 0.0, calculateTotal(nil))
}
