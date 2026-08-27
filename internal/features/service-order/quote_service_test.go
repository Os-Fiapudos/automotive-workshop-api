package serviceorder

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"automotive-workshop-api/internal/shared/trackingtoken"
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
	order := seedOrder(repo, StatusReceived)

	updated, err := service.StartDiagnosis(context.Background(), order.ID)
	require.NoError(t, err)
	assert.Equal(t, StatusInDiagnosis, updated.Status)
}

func TestServiceStartDiagnosisRejectsNonReceived(t *testing.T) {
	repo := newFakeRepository()
	service := newTestService(repo)
	order := seedOrder(repo, StatusInDiagnosis)

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
	order := seedOrder(repo, StatusInDiagnosis)

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
	assert.Equal(t, StatusInDiagnosis, order.Status, "composing a quote no longer transitions the order — only SendQuote does (specs/service-order-quote-decision/)")
	require.Len(t, quote.Items, 2)
}

func TestComposeQuoteRejectsBeforeDiagnosis(t *testing.T) {
	repo := newFakeRepository()
	service := newTestService(repo)
	order := seedOrder(repo, StatusReceived)

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
	order := seedOrder(repo, StatusInDiagnosis)

	_, err := service.ComposeQuote(context.Background(), order.ID, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrEmptyQuote)
}

func TestComposeQuoteRejectsInvalidQuantity(t *testing.T) {
	repo := newFakeRepository()
	service := newTestService(repo)
	order := seedOrder(repo, StatusInDiagnosis)
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
	order := seedOrder(repo, StatusInDiagnosis)

	_, err := service.ComposeQuote(context.Background(), order.ID, []QuoteItemInput{
		{Kind: QuoteItemProduct, ProductID: uuid.New().String(), Quantity: 1},
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrProductNotFound)
}

func TestComposeQuoteRejectsInactiveProduct(t *testing.T) {
	repo := newFakeRepository()
	service := newTestService(repo)
	order := seedOrder(repo, StatusInDiagnosis)
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
	order := seedOrder(repo, StatusInDiagnosis)

	_, err := service.ComposeQuote(context.Background(), order.ID, []QuoteItemInput{
		{Kind: QuoteItemService, ServiceID: uuid.New().String(), Quantity: 1},
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrRequestedServiceNotFound)
}

func TestComposeQuoteRecomposeWhilePendingReplacesItems(t *testing.T) {
	repo := newFakeRepository()
	service := newTestService(repo)
	order := seedOrder(repo, StatusInDiagnosis)
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
	order := seedOrder(repo, StatusAwaitingApproval)
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
	order := seedOrder(repo, StatusInDiagnosis)

	_, err := service.GetQuote(context.Background(), order.ID)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrQuoteNotFound)
}

func TestGetQuoteReturnsComposedQuote(t *testing.T) {
	repo := newFakeRepository()
	service := newTestService(repo)
	order := seedOrder(repo, StatusInDiagnosis)
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
	order := seedOrder(repo, StatusInDiagnosis)
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

// ---- specs/service-order-quote-decision/ ----

// composeQuoteForOrder composes a one-item quote for order through the real
// ComposeQuote use case, so SendQuote/ApproveQuote/RejectQuote tests below
// exercise a realistically-composed quote rather than a hand-built fixture.
func composeQuoteForOrder(t *testing.T, service *ServiceOrderService, repo *fakeRepository, order *ServiceOrder) *Quote {
	t.Helper()
	productID := uuid.New()
	repo.addProduct(&productRef{ID: productID, Active: true, UnitPrice: 42.50, Description: "A part"})
	quote, err := service.ComposeQuote(context.Background(), order.ID, []QuoteItemInput{
		{Kind: QuoteItemProduct, ProductID: productID.String(), Quantity: 1},
	})
	require.NoError(t, err)
	return quote
}

func TestSendQuoteSuccess(t *testing.T) {
	repo := newFakeRepository()
	service := newTestService(repo)
	order := seedOrder(repo, StatusInDiagnosis)
	composeQuoteForOrder(t, service, repo, order)

	quote, err := service.SendQuote(context.Background(), order.ID)
	require.NoError(t, err)

	assert.Equal(t, StatusAwaitingApproval, order.Status)
	require.NotNil(t, quote.SentAt)
	require.NotNil(t, quote.SentVersion)
	assert.Equal(t, quote.Version, *quote.SentVersion)
}

func TestSendQuoteRejectsIncompleteQuote(t *testing.T) {
	repo := newFakeRepository()
	service := newTestService(repo)
	order := seedOrder(repo, StatusInDiagnosis)

	_, err := service.SendQuote(context.Background(), order.ID)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrQuoteNotFound)
	assert.Equal(t, StatusInDiagnosis, order.Status)
}

func TestSendQuoteRejectsAlreadySent(t *testing.T) {
	repo := newFakeRepository()
	service := newTestService(repo)
	order := seedOrder(repo, StatusInDiagnosis)
	composeQuoteForOrder(t, service, repo, order)

	_, err := service.SendQuote(context.Background(), order.ID)
	require.NoError(t, err)

	_, err = service.SendQuote(context.Background(), order.ID)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidStatusTransition)
}

func TestSendQuoteUnknownOrder(t *testing.T) {
	repo := newFakeRepository()
	service := newTestService(repo)

	_, err := service.SendQuote(context.Background(), uuid.New())
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrServiceOrderNotFound)
}

// seedSentQuote composes and sends a quote for order, then registers rawToken
// as its tracking token — the fixture ApproveQuote/RejectQuote tests build
// on, mirroring how a real order reaches AWAITING_APPROVAL with a token
// issued at creation (specs/service-order-tracking/).
func seedSentQuote(t *testing.T, service *ServiceOrderService, repo *fakeRepository, rawToken string) *ServiceOrder {
	t.Helper()
	order := seedOrder(repo, StatusInDiagnosis)
	composeQuoteForOrder(t, service, repo, order)
	_, err := service.SendQuote(context.Background(), order.ID)
	require.NoError(t, err)
	repo.addTrackingToken(order.ID, trackingtoken.Hash(rawToken))
	return order
}

func TestApproveQuoteSuccess(t *testing.T) {
	repo := newFakeRepository()
	service := newTestService(repo)
	order := seedSentQuote(t, service, repo, "the-real-token")

	updatedOrder, quote, err := service.ApproveQuote(context.Background(), order.Code, "the-real-token")
	require.NoError(t, err)

	assert.Equal(t, StatusInProgress, updatedOrder.Status)
	assert.Equal(t, QuoteStatusApproved, quote.Status)
	assert.NotNil(t, quote.RespondedAt)
}

func TestRejectQuoteSuccess(t *testing.T) {
	repo := newFakeRepository()
	service := newTestService(repo)
	order := seedSentQuote(t, service, repo, "the-real-token")

	updatedOrder, quote, err := service.RejectQuote(context.Background(), order.Code, "the-real-token")
	require.NoError(t, err)

	assert.Equal(t, StatusCanceled, updatedOrder.Status)
	assert.Equal(t, QuoteStatusRejected, quote.Status)
	assert.NotNil(t, quote.RespondedAt)
}

func TestApproveQuoteRejectsMissingToken(t *testing.T) {
	repo := newFakeRepository()
	service := newTestService(repo)
	order := seedSentQuote(t, service, repo, "the-real-token")

	_, _, err := service.ApproveQuote(context.Background(), order.Code, "")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrTrackingTokenInvalid)
}

func TestApproveQuoteRejectsWrongToken(t *testing.T) {
	repo := newFakeRepository()
	service := newTestService(repo)
	order := seedSentQuote(t, service, repo, "the-real-token")

	_, _, err := service.ApproveQuote(context.Background(), order.Code, "not-the-real-token")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrTrackingTokenInvalid)
}

func TestApproveQuoteRejectsUnknownCode(t *testing.T) {
	repo := newFakeRepository()
	service := newTestService(repo)

	_, _, err := service.ApproveQuote(context.Background(), 987654321, "any-token")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrServiceOrderNotFound)
}

// TestApproveQuoteRejectsCrossOrderToken covers "o cliente não consegue
// responder a orçamento de outra OS": order B's token must not unlock A.
func TestApproveQuoteRejectsCrossOrderToken(t *testing.T) {
	repo := newFakeRepository()
	service := newTestService(repo)
	orderA := seedSentQuote(t, service, repo, "token-a")
	seedSentQuote(t, service, repo, "token-b")

	_, _, err := service.ApproveQuote(context.Background(), orderA.Code, "token-b")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrTrackingTokenInvalid)
}

// TestApproveThenRejectSameQuoteFails covers "não é possível aprovar e
// reprovar o mesmo orçamento".
func TestApproveThenRejectSameQuoteFails(t *testing.T) {
	repo := newFakeRepository()
	service := newTestService(repo)
	order := seedSentQuote(t, service, repo, "the-real-token")

	_, _, err := service.ApproveQuote(context.Background(), order.Code, "the-real-token")
	require.NoError(t, err)

	_, _, err = service.RejectQuote(context.Background(), order.Code, "the-real-token")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrQuoteAlreadyDecided)
	assert.Equal(t, StatusInProgress, order.Status, "a rejected second decision must not alter the order reached by the first")
}

// TestApproveQuoteTwiceIsConsistentConflict covers "repetição da mesma
// decisão é idempotente ou tratada de forma consistente" — a repeated
// identical decision is treated the same as a differing one: 409 conflict,
// no further state change.
func TestApproveQuoteTwiceIsConsistentConflict(t *testing.T) {
	repo := newFakeRepository()
	service := newTestService(repo)
	order := seedSentQuote(t, service, repo, "the-real-token")

	_, _, err := service.ApproveQuote(context.Background(), order.Code, "the-real-token")
	require.NoError(t, err)

	_, _, err = service.ApproveQuote(context.Background(), order.Code, "the-real-token")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrQuoteAlreadyDecided)
	assert.Equal(t, StatusInProgress, order.Status)
}
