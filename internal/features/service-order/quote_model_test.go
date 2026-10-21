package serviceorder

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"automotive-workshop-api/internal/shared/money"
)

func newTestOrder(t *testing.T, status Status) *ServiceOrder {
	t.Helper()
	order, err := NewServiceOrder(uuid.New(), uuid.New(), "", nil, nil)
	require.NoError(t, err)
	order.Status = status
	return order
}

func TestStartDiagnosisFromReceived(t *testing.T) {
	order := newTestOrder(t, StatusReceived)
	require.NoError(t, order.startDiagnosis())
	assert.Equal(t, StatusInDiagnosis, order.Status)
}

func TestStartDiagnosisRejectsNonReceived(t *testing.T) {
	for _, status := range []Status{StatusInDiagnosis, StatusAwaitingApproval} {
		order := newTestOrder(t, status)
		err := order.startDiagnosis()
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrInvalidStatusTransition)
		assert.Equal(t, status, order.Status, "status must not change on a rejected transition")
	}
}

func TestSendQuoteFromInDiagnosis(t *testing.T) {
	order := newTestOrder(t, StatusInDiagnosis)
	require.NoError(t, order.sendQuote())
	assert.Equal(t, StatusAwaitingApproval, order.Status)
}

func TestSendQuoteRejectsNonInDiagnosis(t *testing.T) {
	for _, status := range []Status{StatusReceived, StatusAwaitingApproval, StatusInProgress} {
		order := newTestOrder(t, status)
		err := order.sendQuote()
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrInvalidStatusTransition)
		assert.Equal(t, status, order.Status, "status must not change on a rejected transition")
	}
}

func TestApproveQuoteFromAwaitingApproval(t *testing.T) {
	order := newTestOrder(t, StatusAwaitingApproval)
	require.NoError(t, order.approveQuote())
	assert.Equal(t, StatusInProgress, order.Status)
}

func TestApproveQuoteRejectsNonAwaitingApproval(t *testing.T) {
	for _, status := range []Status{StatusReceived, StatusInDiagnosis, StatusInProgress} {
		order := newTestOrder(t, status)
		err := order.approveQuote()
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrInvalidStatusTransition)
		assert.Equal(t, status, order.Status, "status must not change on a rejected transition")
	}
}

func TestRejectQuoteFromAwaitingApproval(t *testing.T) {
	order := newTestOrder(t, StatusAwaitingApproval)
	require.NoError(t, order.rejectQuote())
	assert.Equal(t, StatusCanceled, order.Status)
}

func TestRejectQuoteRejectsNonAwaitingApproval(t *testing.T) {
	for _, status := range []Status{StatusReceived, StatusInDiagnosis, StatusInProgress} {
		order := newTestOrder(t, status)
		err := order.rejectQuote()
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrInvalidStatusTransition)
		assert.Equal(t, status, order.Status, "status must not change on a rejected transition")
	}
}

func TestValidateQuoteItemsRejectsEmpty(t *testing.T) {
	err := validateQuoteItems(nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrEmptyQuote)
}

func TestValidateQuoteItemsRejectsInvalidQuantity(t *testing.T) {
	items := []QuoteItem{{Kind: QuoteItemProduct, Quantity: 0, UnitPrice: 1000, Total: 0}}
	err := validateQuoteItems(items)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidQuantity)
}

func TestValidateQuoteItemsAcceptsValidItems(t *testing.T) {
	items := []QuoteItem{
		{Kind: QuoteItemProduct, Quantity: 2, UnitPrice: 1000, Total: 2000},
		{Kind: QuoteItemService, Quantity: 1, UnitPrice: 8000, Total: 8000},
	}
	require.NoError(t, validateQuoteItems(items))
}

func TestCalculateTotalSumsItems(t *testing.T) {
	items := []QuoteItem{
		{Total: 3590},
		{Total: 4850},
		{Total: 12990},
	}
	total, err := calculateTotal(items)
	require.NoError(t, err)
	assert.Equal(t, money.Cents(21430), total)
}

// requirements.md AC4: 1000 × 0.10 summed naively in float64 drifts
// (99.9999999999986); in integer cents it is exactly 100.00.
func TestCalculateTotalManyItemsNoDrift(t *testing.T) {
	var naive float64
	items := make([]QuoteItem, 0, 1000)
	for i := 0; i < 1000; i++ {
		items = append(items, QuoteItem{Total: 10})
		naive += 0.10
	}
	require.NotEqual(t, 100.0, naive, "precondition: the float64 sum drifts")

	total, err := calculateTotal(items)
	require.NoError(t, err)
	assert.Equal(t, money.Cents(10000), total)
}

// 0.1 + 0.2 is 0.30000000000000004 in float64; exactly 0.30 in cents.
func TestCalculateTotalPointOnePlusPointTwo(t *testing.T) {
	total, err := calculateTotal([]QuoteItem{{Total: 10}, {Total: 20}})
	require.NoError(t, err)
	assert.Equal(t, money.Cents(30), total)
}

func TestCalculateTotalEmpty(t *testing.T) {
	total, err := calculateTotal(nil)
	require.NoError(t, err)
	assert.Equal(t, money.Cents(0), total)
}

func TestCalculateTotalRejectsSumBeyondNumericLimit(t *testing.T) {
	_, err := calculateTotal([]QuoteItem{{Total: money.MaxAmount}, {Total: 1}})
	assert.ErrorIs(t, err, ErrQuoteAmountOutOfRange)

	total, err := calculateTotal([]QuoteItem{{Total: money.MaxAmount - 1}, {Total: 1}})
	require.NoError(t, err)
	assert.Equal(t, money.MaxAmount, total)
}

// Item totals are UnitPrice × Quantity in exact cents: 3 × 19.90 = 59.70
// (59.699999999999996 in float64).
func TestPriceQuoteItems(t *testing.T) {
	items := []QuoteItem{
		{Quantity: 3, UnitPrice: 1990},
		{Quantity: 7, UnitPrice: 10},
		{Quantity: 1, UnitPrice: 0},
	}
	require.NoError(t, priceQuoteItems(items))
	assert.Equal(t, money.Cents(5970), items[0].Total)
	assert.Equal(t, money.Cents(70), items[1].Total)
	assert.Equal(t, money.Cents(0), items[2].Total)
}

func TestPriceQuoteItemsRejectsProductBeyondNumericLimit(t *testing.T) {
	err := priceQuoteItems([]QuoteItem{{Quantity: 2, UnitPrice: money.MaxAmount}})
	assert.ErrorIs(t, err, ErrQuoteAmountOutOfRange)
}

func TestPriceQuoteItemsRejectsNonPositiveQuantity(t *testing.T) {
	err := priceQuoteItems([]QuoteItem{{Quantity: 0, UnitPrice: 1990}})
	assert.ErrorIs(t, err, ErrInvalidQuantity)
}
