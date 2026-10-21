package httpapi

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"automotive-workshop-api/internal/features/service-order/domain"
	"automotive-workshop-api/internal/features/service-order/usecase"
	"automotive-workshop-api/internal/shared/trackingtoken"
)

// fakeStore is the HTTP adapter tests' in-memory persistence. It implements
// only the ports the routes under test reach — the external quote decision,
// the customer-facing decision and the external status update — which is
// what the narrow ports of usecase/ports.go allow (design.md §9.4). Every
// other use case is built with nil ports: registering its routes never calls
// them.
type fakeStore struct {
	orders     map[uuid.UUID]*domain.ServiceOrder
	quotes     map[uuid.UUID]*domain.Quote                 // keyed by service order id
	tokens     map[uuid.UUID]string                        // keyed by service order id, value is the token hash
	history    map[uuid.UUID][]*domain.ServiceOrderHistory // keyed by service order id
	executions map[uuid.UUID][]*domain.ServiceExecution    // keyed by service order id
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		orders:     make(map[uuid.UUID]*domain.ServiceOrder),
		quotes:     make(map[uuid.UUID]*domain.Quote),
		tokens:     make(map[uuid.UUID]string),
		history:    make(map[uuid.UUID][]*domain.ServiceOrderHistory),
		executions: make(map[uuid.UUID][]*domain.ServiceExecution),
	}
}

// newTestUseCases builds the use cases RegisterRoutes needs on top of store,
// with both external secrets configured.
func newTestUseCases(store *fakeStore) UseCases {
	return UseCases{
		Opening:   usecase.NewOpening(nil, nil, nil, nil, nil),
		Lifecycle: usecase.NewLifecycle(store, store, store, store).WithExternalStatusUpdateSecret(testStatusSecret),
		Quotes: usecase.NewQuotes(usecase.QuoteDependencies{
			Orders:        store,
			TrackedOrders: store,
			Quotes:        store,
		}).WithExternalNotificationSecret(testExternalSecret),
		Executions: usecase.NewExecutions(nil, nil, nil),
		Queries:    usecase.NewQueries(nil, nil, nil, nil, nil, nil),
		StockUsage: usecase.NewStockUsage(nil, nil),
		Metrics:    usecase.NewMetrics(nil),
	}
}

// seedOrder registers an order already in status.
func seedOrder(store *fakeStore, status domain.Status) *domain.ServiceOrder {
	order := &domain.ServiceOrder{
		ID:         uuid.New(),
		Code:       int64(len(store.orders) + 1),
		CustomerID: uuid.New(),
		VehicleID:  uuid.New(),
		Status:     status,
	}
	store.orders[order.ID] = order
	return order
}

// seedSentQuote registers an AWAITING_APPROVAL order whose pending quote has
// been sent, reachable with rawToken as its tracking token.
func seedSentQuote(t *testing.T, store *fakeStore, rawToken string) *domain.ServiceOrder {
	t.Helper()
	order := seedOrder(store, domain.StatusAwaitingApproval)
	sentAt := time.Now().UTC()
	version := 1
	store.quotes[order.ID] = &domain.Quote{
		ID:             uuid.New(),
		ServiceOrderID: order.ID,
		Status:         domain.QuoteStatusPending,
		Version:        version,
		SentAt:         &sentAt,
		SentVersion:    &version,
		Items:          []domain.QuoteItem{{Kind: domain.QuoteItemService, ServiceID: uuid.New(), Quantity: 1, UnitPrice: 100, Total: 100}},
		TotalAmount:    100,
	}
	store.tokens[order.ID] = trackingtoken.Hash(rawToken)
	return order
}

func (store *fakeStore) FindServiceOrderByID(_ context.Context, id uuid.UUID) (*domain.ServiceOrder, error) {
	order, ok := store.orders[id]
	if !ok {
		return nil, domain.ErrServiceOrderNotFound
	}
	return order, nil
}

func (store *fakeStore) FindServiceOrderByCodeWithTrackingToken(_ context.Context, code int64, tokenHash string) (*domain.ServiceOrder, error) {
	for _, order := range store.orders {
		if order.Code != code {
			continue
		}
		if store.tokens[order.ID] != tokenHash {
			return nil, domain.ErrTrackingTokenInvalid
		}
		return order, nil
	}
	return nil, domain.ErrServiceOrderNotFound
}

func (store *fakeStore) FindQuoteByServiceOrderID(_ context.Context, serviceOrderID uuid.UUID) (*domain.Quote, error) {
	quote, ok := store.quotes[serviceOrderID]
	if !ok {
		return nil, domain.ErrQuoteNotFound
	}
	return quote, nil
}

func (store *fakeStore) SaveQuote(_ context.Context, order *domain.ServiceOrder, items []domain.QuoteItem, total float64) (*domain.Quote, error) {
	quote := &domain.Quote{ID: uuid.New(), ServiceOrderID: order.ID, Status: domain.QuoteStatusPending, Version: 1, Items: items, TotalAmount: total}
	store.quotes[order.ID] = quote
	return quote, nil
}

func (store *fakeStore) SendQuote(_ context.Context, _ *domain.ServiceOrder, quote *domain.Quote) (*domain.Quote, error) {
	now := time.Now().UTC()
	quote.SentAt = &now
	return quote, nil
}

func (store *fakeStore) DecideQuote(_ context.Context, _ *domain.ServiceOrder, quote *domain.Quote, decision domain.QuoteStatus) (*domain.Quote, error) {
	if quote.Status != domain.QuoteStatusPending {
		return nil, domain.ErrQuoteAlreadyDecided
	}
	now := time.Now().UTC()
	quote.Status = decision
	quote.RespondedAt = &now
	return quote, nil
}

func (store *fakeStore) StartDiagnosis(_ context.Context, order *domain.ServiceOrder, notes string) error {
	store.recordTransition(order, "diagnosis_started", domain.HistoryDescription(domain.HistoryDiagnosisStarted, notes), domain.StatusReceived)
	return nil
}

func (store *fakeStore) FinalizeOrder(_ context.Context, order *domain.ServiceOrder, notes string) error {
	store.recordTransition(order, "completion", domain.HistoryDescription(domain.HistoryOrderFinalized, notes), domain.StatusInProgress)
	return nil
}

func (store *fakeStore) DeliverOrder(_ context.Context, order *domain.ServiceOrder, notes string) error {
	store.recordTransition(order, "delivery", domain.HistoryDescription(domain.HistoryVehicleDelivered, notes), domain.StatusCompleted)
	return nil
}

func (store *fakeStore) FindServiceExecutionsByServiceOrderID(_ context.Context, serviceOrderID uuid.UUID) ([]*domain.ServiceExecution, error) {
	return store.executions[serviceOrderID], nil
}

// recordTransition appends the history entry the Postgres adapter writes in
// the same transaction as each transition.
func (store *fakeStore) recordTransition(order *domain.ServiceOrder, event, description string, previous domain.Status) {
	store.history[order.ID] = append(store.history[order.ID], &domain.ServiceOrderHistory{
		ID:             uuid.New(),
		ServiceOrderID: order.ID,
		OccurredAt:     time.Now().UTC(),
		Event:          event,
		Description:    description,
		PreviousStatus: previous,
		NewStatus:      order.Status,
	})
}
