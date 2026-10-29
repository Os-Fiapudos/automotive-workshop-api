package usecase

import (
	"automotive-workshop-api/internal/features/service-order/domain"
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"automotive-workshop-api/internal/shared/trackingtoken"

	"automotive-workshop-api/internal/shared/money"
)

func seedOrder(repo *fakeRepository, status domain.Status) *domain.ServiceOrder {
	order, _ := domain.NewServiceOrder(uuid.New(), uuid.New(), "", nil, nil)
	order.Status = status
	repo.addOrder(order)
	return order
}

func TestStartDiagnosisSuccess(t *testing.T) {
	repo := newFakeRepository()
	service := newTestService(repo)
	order := seedOrder(repo, domain.StatusReceived)

	updated, err := service.StartDiagnosis(context.Background(), order.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.StatusInDiagnosis, updated.Status)
}

func TestServiceStartDiagnosisRejectsNonReceived(t *testing.T) {
	repo := newFakeRepository()
	service := newTestService(repo)
	order := seedOrder(repo, domain.StatusInDiagnosis)

	_, err := service.StartDiagnosis(context.Background(), order.ID)
	require.Error(t, err)
	assert.ErrorIs(t, err, domain.ErrInvalidStatusTransition)
}

func TestStartDiagnosisUnknownOrder(t *testing.T) {
	repo := newFakeRepository()
	service := newTestService(repo)

	_, err := service.StartDiagnosis(context.Background(), uuid.New())
	require.Error(t, err)
	assert.ErrorIs(t, err, domain.ErrServiceOrderNotFound)
}

func TestComposeQuoteSuccess(t *testing.T) {
	repo := newFakeRepository()
	service := newTestService(repo)
	order := seedOrder(repo, domain.StatusInDiagnosis)

	productID := uuid.New()
	repo.addProduct(&ProductRef{ID: productID, Code: 1, Name: "Oil Filter", Description: "Engine oil filter.", UnitPrice: 3590, Active: true})
	serviceID := uuid.New()
	repo.addService(&ServiceRef{ID: serviceID, Code: 1, Name: "Oil Change", Description: "Engine oil and filter change.", Price: 8000})

	quote, err := service.ComposeQuote(context.Background(), order.ID, []QuoteItemInput{
		{Kind: domain.QuoteItemProduct, ProductID: productID.String(), Quantity: 2},
		{Kind: domain.QuoteItemService, ServiceID: serviceID.String(), Quantity: 1},
	})
	require.NoError(t, err)

	assert.Equal(t, money.Cents(15180), quote.TotalAmount) // 2*35.90 + 80.00 = 151.80, exact
	assert.Equal(t, domain.StatusInDiagnosis, order.Status, "composing a quote no longer transitions the order — only SendQuote does (specs/service-order-quote-decision/)")
	require.Len(t, quote.Items, 2)
}

func TestComposeQuoteRejectsBeforeDiagnosis(t *testing.T) {
	repo := newFakeRepository()
	service := newTestService(repo)
	order := seedOrder(repo, domain.StatusReceived)

	productID := uuid.New()
	repo.addProduct(&ProductRef{ID: productID, Active: true, UnitPrice: 1000})

	_, err := service.ComposeQuote(context.Background(), order.ID, []QuoteItemInput{
		{Kind: domain.QuoteItemProduct, ProductID: productID.String(), Quantity: 1},
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, domain.ErrDiagnosisNotStarted)
}

func TestComposeQuoteUnknownOrder(t *testing.T) {
	repo := newFakeRepository()
	service := newTestService(repo)

	_, err := service.ComposeQuote(context.Background(), uuid.New(), []QuoteItemInput{})
	require.Error(t, err)
	assert.ErrorIs(t, err, domain.ErrServiceOrderNotFound)
}

func TestComposeQuoteRejectsEmptyItems(t *testing.T) {
	repo := newFakeRepository()
	service := newTestService(repo)
	order := seedOrder(repo, domain.StatusInDiagnosis)

	_, err := service.ComposeQuote(context.Background(), order.ID, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, domain.ErrEmptyQuote)
}

func TestComposeQuoteRejectsInvalidQuantity(t *testing.T) {
	repo := newFakeRepository()
	service := newTestService(repo)
	order := seedOrder(repo, domain.StatusInDiagnosis)
	productID := uuid.New()
	repo.addProduct(&ProductRef{ID: productID, Active: true, UnitPrice: 1000})

	_, err := service.ComposeQuote(context.Background(), order.ID, []QuoteItemInput{
		{Kind: domain.QuoteItemProduct, ProductID: productID.String(), Quantity: 0},
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, domain.ErrInvalidQuantity)
}

func TestComposeQuoteRejectsUnknownProduct(t *testing.T) {
	repo := newFakeRepository()
	service := newTestService(repo)
	order := seedOrder(repo, domain.StatusInDiagnosis)

	_, err := service.ComposeQuote(context.Background(), order.ID, []QuoteItemInput{
		{Kind: domain.QuoteItemProduct, ProductID: uuid.New().String(), Quantity: 1},
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, domain.ErrProductNotFound)
}

func TestComposeQuoteRejectsInactiveProduct(t *testing.T) {
	repo := newFakeRepository()
	service := newTestService(repo)
	order := seedOrder(repo, domain.StatusInDiagnosis)
	productID := uuid.New()
	repo.addProduct(&ProductRef{ID: productID, Active: false, UnitPrice: 1000})

	_, err := service.ComposeQuote(context.Background(), order.ID, []QuoteItemInput{
		{Kind: domain.QuoteItemProduct, ProductID: productID.String(), Quantity: 1},
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, domain.ErrProductInactive)
}

func TestComposeQuoteRejectsUnknownService(t *testing.T) {
	repo := newFakeRepository()
	service := newTestService(repo)
	order := seedOrder(repo, domain.StatusInDiagnosis)

	_, err := service.ComposeQuote(context.Background(), order.ID, []QuoteItemInput{
		{Kind: domain.QuoteItemService, ServiceID: uuid.New().String(), Quantity: 1},
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, domain.ErrRequestedServiceNotFound)
}

func TestComposeQuoteRecomposeWhilePendingReplacesItems(t *testing.T) {
	repo := newFakeRepository()
	service := newTestService(repo)
	order := seedOrder(repo, domain.StatusInDiagnosis)
	productID := uuid.New()
	repo.addProduct(&ProductRef{ID: productID, Active: true, UnitPrice: 1000})

	_, err := service.ComposeQuote(context.Background(), order.ID, []QuoteItemInput{
		{Kind: domain.QuoteItemProduct, ProductID: productID.String(), Quantity: 1},
	})
	require.NoError(t, err)

	quote, err := service.ComposeQuote(context.Background(), order.ID, []QuoteItemInput{
		{Kind: domain.QuoteItemProduct, ProductID: productID.String(), Quantity: 3},
	})
	require.NoError(t, err)
	assert.Equal(t, money.Cents(3000), quote.TotalAmount)
	require.Len(t, quote.Items, 1)
}

func TestComposeQuoteRejectsAlreadyDecided(t *testing.T) {
	repo := newFakeRepository()
	service := newTestService(repo)
	order := seedOrder(repo, domain.StatusAwaitingApproval)
	repo.seedDecidedQuote(order.ID, domain.QuoteStatusApproved)
	productID := uuid.New()
	repo.addProduct(&ProductRef{ID: productID, Active: true, UnitPrice: 1000})

	_, err := service.ComposeQuote(context.Background(), order.ID, []QuoteItemInput{
		{Kind: domain.QuoteItemProduct, ProductID: productID.String(), Quantity: 1},
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, domain.ErrQuoteAlreadyDecided)
}

func TestGetQuoteNotFoundBeforeComposition(t *testing.T) {
	repo := newFakeRepository()
	service := newTestService(repo)
	order := seedOrder(repo, domain.StatusInDiagnosis)

	_, err := service.GetQuote(context.Background(), order.ID)
	require.Error(t, err)
	assert.ErrorIs(t, err, domain.ErrQuoteNotFound)
}

func TestGetQuoteReturnsComposedQuote(t *testing.T) {
	repo := newFakeRepository()
	service := newTestService(repo)
	order := seedOrder(repo, domain.StatusInDiagnosis)
	productID := uuid.New()
	repo.addProduct(&ProductRef{ID: productID, Active: true, UnitPrice: 1000, Description: "A part"})

	_, err := service.ComposeQuote(context.Background(), order.ID, []QuoteItemInput{
		{Kind: domain.QuoteItemProduct, ProductID: productID.String(), Quantity: 2},
	})
	require.NoError(t, err)

	quote, err := service.GetQuote(context.Background(), order.ID)
	require.NoError(t, err)
	assert.Equal(t, money.Cents(2000), quote.TotalAmount)
}

// TestComposeQuoteCatalogChangeDoesNotAffectPersistedItem proves the
// snapshot is taken by value: mutating the catalog fixture after composing
// must not change the already-built QuoteItem (requirements.md §3.5).
func TestComposeQuoteCatalogChangeDoesNotAffectPersistedItem(t *testing.T) {
	repo := newFakeRepository()
	service := newTestService(repo)
	order := seedOrder(repo, domain.StatusInDiagnosis)
	productID := uuid.New()
	repo.addProduct(&ProductRef{ID: productID, Active: true, UnitPrice: 1000, Description: "Original description"})

	quote, err := service.ComposeQuote(context.Background(), order.ID, []QuoteItemInput{
		{Kind: domain.QuoteItemProduct, ProductID: productID.String(), Quantity: 1},
	})
	require.NoError(t, err)

	// Simulate a later catalog change.
	repo.addProduct(&ProductRef{ID: productID, Active: true, UnitPrice: 99900, Description: "Changed description"})

	assert.Equal(t, "Original description", quote.Items[0].Description)
	assert.Equal(t, money.Cents(1000), quote.Items[0].UnitPrice)
}

// ---- specs/service-order-quote-decision/ ----

// composeQuoteForOrder composes a one-item quote for order through the real
// ComposeQuote use case, so SendQuote/ApproveQuote/RejectQuote tests below
// exercise a realistically-composed quote rather than a hand-built fixture.
func composeQuoteForOrder(t *testing.T, service *testServices, repo *fakeRepository, order *domain.ServiceOrder) *domain.Quote {
	t.Helper()
	productID := uuid.New()
	repo.addProduct(&ProductRef{ID: productID, Active: true, UnitPrice: 4250, Description: "A part"})
	quote, err := service.ComposeQuote(context.Background(), order.ID, []QuoteItemInput{
		{Kind: domain.QuoteItemProduct, ProductID: productID.String(), Quantity: 1},
	})
	require.NoError(t, err)
	return quote
}

func TestSendQuoteSuccess(t *testing.T) {
	repo := newFakeRepository()
	service := newTestService(repo)
	order := seedOrder(repo, domain.StatusInDiagnosis)
	composeQuoteForOrder(t, service, repo, order)

	quote, err := service.SendQuote(context.Background(), order.ID)
	require.NoError(t, err)

	assert.Equal(t, domain.StatusAwaitingApproval, order.Status)
	require.NotNil(t, quote.SentAt)
	require.NotNil(t, quote.SentVersion)
	assert.Equal(t, quote.Version, *quote.SentVersion)
}

func TestSendQuoteRejectsIncompleteQuote(t *testing.T) {
	repo := newFakeRepository()
	service := newTestService(repo)
	order := seedOrder(repo, domain.StatusInDiagnosis)

	_, err := service.SendQuote(context.Background(), order.ID)
	require.Error(t, err)
	assert.ErrorIs(t, err, domain.ErrQuoteNotFound)
	assert.Equal(t, domain.StatusInDiagnosis, order.Status)
}

func TestSendQuoteRejectsAlreadySent(t *testing.T) {
	repo := newFakeRepository()
	service := newTestService(repo)
	order := seedOrder(repo, domain.StatusInDiagnosis)
	composeQuoteForOrder(t, service, repo, order)

	_, err := service.SendQuote(context.Background(), order.ID)
	require.NoError(t, err)

	_, err = service.SendQuote(context.Background(), order.ID)
	require.Error(t, err)
	assert.ErrorIs(t, err, domain.ErrInvalidStatusTransition)
}

func TestSendQuoteUnknownOrder(t *testing.T) {
	repo := newFakeRepository()
	service := newTestService(repo)

	_, err := service.SendQuote(context.Background(), uuid.New())
	require.Error(t, err)
	assert.ErrorIs(t, err, domain.ErrServiceOrderNotFound)
}

// seedSentQuote composes and sends a quote for order, then registers rawToken
// as its tracking token — the fixture ApproveQuote/RejectQuote tests build
// on, mirroring how a real order reaches AWAITING_APPROVAL with a token
// issued at creation (specs/service-order-tracking/).
func seedSentQuote(t *testing.T, service *testServices, repo *fakeRepository, rawToken string) *domain.ServiceOrder {
	t.Helper()
	order := seedOrder(repo, domain.StatusInDiagnosis)
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

	assert.Equal(t, domain.StatusInProgress, updatedOrder.Status)
	assert.Equal(t, domain.QuoteStatusApproved, quote.Status)
	assert.NotNil(t, quote.RespondedAt)
}

func TestRejectQuoteSuccess(t *testing.T) {
	repo := newFakeRepository()
	service := newTestService(repo)
	order := seedSentQuote(t, service, repo, "the-real-token")

	updatedOrder, quote, err := service.RejectQuote(context.Background(), order.Code, "the-real-token")
	require.NoError(t, err)

	assert.Equal(t, domain.StatusCanceled, updatedOrder.Status)
	assert.Equal(t, domain.QuoteStatusRejected, quote.Status)
	assert.NotNil(t, quote.RespondedAt)
}

func TestApproveQuoteRejectsMissingToken(t *testing.T) {
	repo := newFakeRepository()
	service := newTestService(repo)
	order := seedSentQuote(t, service, repo, "the-real-token")

	_, _, err := service.ApproveQuote(context.Background(), order.Code, "")
	require.Error(t, err)
	assert.ErrorIs(t, err, domain.ErrTrackingTokenInvalid)
}

func TestApproveQuoteRejectsWrongToken(t *testing.T) {
	repo := newFakeRepository()
	service := newTestService(repo)
	order := seedSentQuote(t, service, repo, "the-real-token")

	_, _, err := service.ApproveQuote(context.Background(), order.Code, "not-the-real-token")
	require.Error(t, err)
	assert.ErrorIs(t, err, domain.ErrTrackingTokenInvalid)
}

func TestApproveQuoteRejectsUnknownCode(t *testing.T) {
	repo := newFakeRepository()
	service := newTestService(repo)

	_, _, err := service.ApproveQuote(context.Background(), 987654321, "any-token")
	require.Error(t, err)
	assert.ErrorIs(t, err, domain.ErrServiceOrderNotFound)
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
	assert.ErrorIs(t, err, domain.ErrTrackingTokenInvalid)
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
	assert.ErrorIs(t, err, domain.ErrQuoteAlreadyDecided)
	assert.Equal(t, domain.StatusInProgress, order.Status, "a rejected second decision must not alter the order reached by the first")
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
	assert.ErrorIs(t, err, domain.ErrQuoteAlreadyDecided)
	assert.Equal(t, domain.StatusInProgress, order.Status)
}

// ---- External notification channel (requirements.md §9) ----

func TestDecideQuoteExternalApproveSuccess(t *testing.T) {
	repo := newFakeRepository()
	service := newTestService(repo)
	order := seedSentQuote(t, service, repo, "the-real-token")

	updatedOrder, quote, err := service.DecideQuoteExternal(context.Background(), order.ID, domain.QuoteStatusApproved)
	require.NoError(t, err)

	assert.Equal(t, domain.StatusInProgress, updatedOrder.Status)
	assert.Equal(t, domain.QuoteStatusApproved, quote.Status)
	assert.NotNil(t, quote.RespondedAt)
}

func TestDecideQuoteExternalRejectSuccess(t *testing.T) {
	repo := newFakeRepository()
	service := newTestService(repo)
	order := seedSentQuote(t, service, repo, "the-real-token")

	updatedOrder, quote, err := service.DecideQuoteExternal(context.Background(), order.ID, domain.QuoteStatusRejected)
	require.NoError(t, err)

	assert.Equal(t, domain.StatusCanceled, updatedOrder.Status)
	assert.Equal(t, domain.QuoteStatusRejected, quote.Status)
	assert.NotNil(t, quote.RespondedAt)
}

func TestDecideQuoteExternalRejectsUnknownOrder(t *testing.T) {
	repo := newFakeRepository()
	service := newTestService(repo)

	_, _, err := service.DecideQuoteExternal(context.Background(), uuid.New(), domain.QuoteStatusApproved)
	require.Error(t, err)
	assert.ErrorIs(t, err, domain.ErrServiceOrderNotFound)
}

func TestDecideQuoteExternalRejectsInvalidDecision(t *testing.T) {
	repo := newFakeRepository()
	service := newTestService(repo)
	order := seedSentQuote(t, service, repo, "the-real-token")

	_, _, err := service.DecideQuoteExternal(context.Background(), order.ID, domain.QuoteStatusPending)
	require.Error(t, err)
	assert.ErrorIs(t, err, domain.ErrInvalidQuoteDecision)
	assert.Equal(t, domain.StatusAwaitingApproval, order.Status)
}

// TestDecideQuoteExternalRequiresSentQuote: a quote composed but never sent
// leaves the order IN_DIAGNOSIS, so the shared core's status precondition
// rejects the decision.
func TestDecideQuoteExternalRequiresSentQuote(t *testing.T) {
	repo := newFakeRepository()
	service := newTestService(repo)
	order := seedOrder(repo, domain.StatusInDiagnosis)
	composeQuoteForOrder(t, service, repo, order)

	_, _, err := service.DecideQuoteExternal(context.Background(), order.ID, domain.QuoteStatusApproved)
	require.Error(t, err)
	assert.ErrorIs(t, err, domain.ErrInvalidStatusTransition)
}

// TestDecideQuoteExternalRepeatIsConflict covers AC19 across both channels:
// a decision already taken via the customer channel, then relayed again
// externally (same or different), is a consistent 409 with no state change.
func TestDecideQuoteExternalRepeatIsConflict(t *testing.T) {
	repo := newFakeRepository()
	service := newTestService(repo)
	order := seedSentQuote(t, service, repo, "the-real-token")

	_, _, err := service.ApproveQuote(context.Background(), order.Code, "the-real-token")
	require.NoError(t, err)

	for _, decision := range []domain.QuoteStatus{domain.QuoteStatusApproved, domain.QuoteStatusRejected} {
		_, _, err = service.DecideQuoteExternal(context.Background(), order.ID, decision)
		require.Error(t, err)
		assert.ErrorIs(t, err, domain.ErrQuoteAlreadyDecided)
		assert.Equal(t, domain.StatusInProgress, order.Status)
	}
}

func TestAuthenticateExternalNotification(t *testing.T) {
	service := newTestService(newFakeRepository()).WithExternalNotificationSecret("s3cret")

	assert.True(t, service.AuthenticateExternalNotification("s3cret"))
	assert.False(t, service.AuthenticateExternalNotification(""))
	assert.False(t, service.AuthenticateExternalNotification("s3cre"))
	assert.False(t, service.AuthenticateExternalNotification("s3cret-longer"))
}

func TestAuthenticateExternalNotificationRejectsWhenUnconfigured(t *testing.T) {
	service := newTestService(newFakeRepository())

	assert.False(t, service.AuthenticateExternalNotification(""))
	assert.False(t, service.AuthenticateExternalNotification("anything"))
}

// specs/monetary-value-precision/: an item total or quote total beyond the
// NUMERIC(12,2) columns is rejected before anything is persisted, instead of
// failing in the database.
func TestComposeQuoteRejectsAmountBeyondNumericLimit(t *testing.T) {
	repo := newFakeRepository()
	service := newTestService(repo)
	order := seedOrder(repo, domain.StatusInDiagnosis)
	productID := uuid.New()
	repo.addProduct(&ProductRef{ID: productID, Active: true, UnitPrice: money.MaxAmount, Description: "A part"})

	_, err := service.ComposeQuote(context.Background(), order.ID, []QuoteItemInput{
		{Kind: domain.QuoteItemProduct, ProductID: productID.String(), Quantity: 2},
	})
	assert.ErrorIs(t, err, domain.ErrQuoteAmountOutOfRange)
	assert.Nil(t, repo.quotes[order.ID])
}

// Totals with quantities are exact: 3 × 19.90 + 7 × 0.10 = 60.40.
func TestComposeQuoteExactTotalsWithQuantities(t *testing.T) {
	repo := newFakeRepository()
	service := newTestService(repo)
	order := seedOrder(repo, domain.StatusInDiagnosis)
	productID := uuid.New()
	repo.addProduct(&ProductRef{ID: productID, Active: true, UnitPrice: 1990, Description: "A part"})
	serviceID := uuid.New()
	repo.addService(&ServiceRef{ID: serviceID, Description: "A service", Price: 10})

	quote, err := service.ComposeQuote(context.Background(), order.ID, []QuoteItemInput{
		{Kind: domain.QuoteItemProduct, ProductID: productID.String(), Quantity: 3},
		{Kind: domain.QuoteItemService, ServiceID: serviceID.String(), Quantity: 7},
	})
	require.NoError(t, err)
	assert.Equal(t, money.Cents(5970), quote.Items[0].Total)
	assert.Equal(t, money.Cents(70), quote.Items[1].Total)
	assert.Equal(t, money.Cents(6040), quote.TotalAmount)
	// The JSON rendering of these exact totals is asserted next to the DTO, in
	// adapter/httpapi (TestQuoteResponseRendersExactTotals).
}
