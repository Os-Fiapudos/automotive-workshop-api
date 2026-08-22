package serviceorder

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func seedOrder(repo *fakeRepository, status Status) *ServiceOrder {
	order, _ := NewServiceOrder(uuid.New(), uuid.New(), "", nil)
	order.Status = status
	repo.addOrder(order)
	return order
}

func TestStartDiagnosisSuccess(t *testing.T) {
	repo := newFakeRepository()
	service := newTestService(repo)
	order := seedOrder(repo, StatusRecebida)

	updated, err := service.StartDiagnosis(context.Background(), order.ID)
	require.NoError(t, err)
	assert.Equal(t, StatusEmDiagnostico, updated.Status)
}

func TestServiceStartDiagnosisRejectsNonRecebida(t *testing.T) {
	repo := newFakeRepository()
	service := newTestService(repo)
	order := seedOrder(repo, StatusEmDiagnostico)

	_, err := service.StartDiagnosis(context.Background(), order.ID)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidStatusTransition)
}

func TestStartDiagnosisUnknownOrder(t *testing.T) {
	repo := newFakeRepository()
	service := newTestService(repo)

	_, err := service.StartDiagnosis(context.Background(), uuid.New())
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrServiceOrderNotFound)
}

func TestComposeQuoteSuccess(t *testing.T) {
	repo := newFakeRepository()
	service := newTestService(repo)
	order := seedOrder(repo, StatusEmDiagnostico)

	productID := uuid.New()
	repo.addProduct(&productRef{ID: productID, Code: 1, Name: "Oil Filter", Description: "Engine oil filter.", UnitPrice: 35.90, Active: true})
	serviceID := uuid.New()
	repo.addService(&serviceRef{ID: serviceID, Code: 1, Name: "Oil Change", Description: "Engine oil and filter change.", Price: 80.00})

	quote, err := service.ComposeQuote(context.Background(), order.ID, []QuoteItemInput{
		{Kind: QuoteItemProduct, ProductID: productID.String(), Quantity: 2},
		{Kind: QuoteItemService, ServiceID: serviceID.String(), Quantity: 1},
	})
	require.NoError(t, err)

	assert.InDelta(t, 151.80, quote.TotalAmount, 0.0001) // 2*35.90 + 80.00
	assert.Equal(t, StatusAguardandoAprovacao, order.Status)
	require.Len(t, quote.Items, 2)
}

func TestComposeQuoteRejectsBeforeDiagnosis(t *testing.T) {
	repo := newFakeRepository()
	service := newTestService(repo)
	order := seedOrder(repo, StatusRecebida)

	productID := uuid.New()
	repo.addProduct(&productRef{ID: productID, Active: true, UnitPrice: 10})

	_, err := service.ComposeQuote(context.Background(), order.ID, []QuoteItemInput{
		{Kind: QuoteItemProduct, ProductID: productID.String(), Quantity: 1},
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrDiagnosisNotStarted)
}

func TestComposeQuoteUnknownOrder(t *testing.T) {
	repo := newFakeRepository()
	service := newTestService(repo)

	_, err := service.ComposeQuote(context.Background(), uuid.New(), []QuoteItemInput{})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrServiceOrderNotFound)
}

func TestComposeQuoteRejectsEmptyItems(t *testing.T) {
	repo := newFakeRepository()
	service := newTestService(repo)
	order := seedOrder(repo, StatusEmDiagnostico)

	_, err := service.ComposeQuote(context.Background(), order.ID, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrEmptyQuote)
}

func TestComposeQuoteRejectsInvalidQuantity(t *testing.T) {
	repo := newFakeRepository()
	service := newTestService(repo)
	order := seedOrder(repo, StatusEmDiagnostico)
	productID := uuid.New()
	repo.addProduct(&productRef{ID: productID, Active: true, UnitPrice: 10})

	_, err := service.ComposeQuote(context.Background(), order.ID, []QuoteItemInput{
		{Kind: QuoteItemProduct, ProductID: productID.String(), Quantity: 0},
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidQuantity)
}

func TestComposeQuoteRejectsUnknownProduct(t *testing.T) {
	repo := newFakeRepository()
	service := newTestService(repo)
	order := seedOrder(repo, StatusEmDiagnostico)

	_, err := service.ComposeQuote(context.Background(), order.ID, []QuoteItemInput{
		{Kind: QuoteItemProduct, ProductID: uuid.New().String(), Quantity: 1},
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrProductNotFound)
}

func TestComposeQuoteRejectsInactiveProduct(t *testing.T) {
	repo := newFakeRepository()
	service := newTestService(repo)
	order := seedOrder(repo, StatusEmDiagnostico)
	productID := uuid.New()
	repo.addProduct(&productRef{ID: productID, Active: false, UnitPrice: 10})

	_, err := service.ComposeQuote(context.Background(), order.ID, []QuoteItemInput{
		{Kind: QuoteItemProduct, ProductID: productID.String(), Quantity: 1},
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrProductInactive)
}

func TestComposeQuoteRejectsUnknownService(t *testing.T) {
	repo := newFakeRepository()
	service := newTestService(repo)
	order := seedOrder(repo, StatusEmDiagnostico)

	_, err := service.ComposeQuote(context.Background(), order.ID, []QuoteItemInput{
		{Kind: QuoteItemService, ServiceID: uuid.New().String(), Quantity: 1},
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrRequestedServiceNotFound)
}

func TestComposeQuoteRecomposeWhilePendingReplacesItems(t *testing.T) {
	repo := newFakeRepository()
	service := newTestService(repo)
	order := seedOrder(repo, StatusEmDiagnostico)
	productID := uuid.New()
	repo.addProduct(&productRef{ID: productID, Active: true, UnitPrice: 10})

	_, err := service.ComposeQuote(context.Background(), order.ID, []QuoteItemInput{
		{Kind: QuoteItemProduct, ProductID: productID.String(), Quantity: 1},
	})
	require.NoError(t, err)

	quote, err := service.ComposeQuote(context.Background(), order.ID, []QuoteItemInput{
		{Kind: QuoteItemProduct, ProductID: productID.String(), Quantity: 3},
	})
	require.NoError(t, err)
	assert.InDelta(t, 30.0, quote.TotalAmount, 0.0001)
	require.Len(t, quote.Items, 1)
}

func TestComposeQuoteRejectsAlreadyDecided(t *testing.T) {
	repo := newFakeRepository()
	service := newTestService(repo)
	order := seedOrder(repo, StatusAguardandoAprovacao)
	repo.seedDecidedQuote(order.ID, QuoteStatusApproved)
	productID := uuid.New()
	repo.addProduct(&productRef{ID: productID, Active: true, UnitPrice: 10})

	_, err := service.ComposeQuote(context.Background(), order.ID, []QuoteItemInput{
		{Kind: QuoteItemProduct, ProductID: productID.String(), Quantity: 1},
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrQuoteAlreadyDecided)
}

func TestGetQuoteNotFoundBeforeComposition(t *testing.T) {
	repo := newFakeRepository()
	service := newTestService(repo)
	order := seedOrder(repo, StatusEmDiagnostico)

	_, err := service.GetQuote(context.Background(), order.ID)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrQuoteNotFound)
}

func TestGetQuoteReturnsComposedQuote(t *testing.T) {
	repo := newFakeRepository()
	service := newTestService(repo)
	order := seedOrder(repo, StatusEmDiagnostico)
	productID := uuid.New()
	repo.addProduct(&productRef{ID: productID, Active: true, UnitPrice: 10, Description: "A part"})

	_, err := service.ComposeQuote(context.Background(), order.ID, []QuoteItemInput{
		{Kind: QuoteItemProduct, ProductID: productID.String(), Quantity: 2},
	})
	require.NoError(t, err)

	quote, err := service.GetQuote(context.Background(), order.ID)
	require.NoError(t, err)
	assert.InDelta(t, 20.0, quote.TotalAmount, 0.0001)
}

// TestComposeQuoteCatalogChangeDoesNotAffectPersistedItem proves the
// snapshot is taken by value: mutating the catalog fixture after composing
// must not change the already-built QuoteItem (requirements.md §3.5).
func TestComposeQuoteCatalogChangeDoesNotAffectPersistedItem(t *testing.T) {
	repo := newFakeRepository()
	service := newTestService(repo)
	order := seedOrder(repo, StatusEmDiagnostico)
	productID := uuid.New()
	repo.addProduct(&productRef{ID: productID, Active: true, UnitPrice: 10, Description: "Original description"})

	quote, err := service.ComposeQuote(context.Background(), order.ID, []QuoteItemInput{
		{Kind: QuoteItemProduct, ProductID: productID.String(), Quantity: 1},
	})
	require.NoError(t, err)

	// Simulate a later catalog change.
	repo.addProduct(&productRef{ID: productID, Active: true, UnitPrice: 999, Description: "Changed description"})

	assert.Equal(t, "Original description", quote.Items[0].Description)
	assert.InDelta(t, 10.0, quote.Items[0].UnitPrice, 0.0001)
}
