package usecase

import (
	"context"

	"github.com/google/uuid"

	"automotive-workshop-api/internal/features/service-order/domain"
)

// Ports are declared here, by their only consumer, and each is only as wide
// as the use cases that call it (specs/service-order-architecture-refactor/
// design.md §9.2). They replace the single 20-method serviceOrderLookups
// interface. The Postgres adapter implements all of them on one type
// (adapter/postgres/assertions.go); the e-mail adapters implement
// QuoteNotifier.

// OrderCreator persists a newly opened service order.
type OrderCreator interface {
	// Create persists order and returns the raw tracking token generated for
	// it (specs/service-order-tracking/design.md §5) — only that call ever
	// sees the raw value; the database only ever stores its hash.
	Create(ctx context.Context, order *domain.ServiceOrder) (trackingToken string, err error)
}

// OrderFinder loads a service order by its technical id
// (domain.ErrServiceOrderNotFound when it does not exist).
type OrderFinder interface {
	FindServiceOrderByID(ctx context.Context, id uuid.UUID) (*domain.ServiceOrder, error)
}

// OrderCodeFinder loads a service order by its sequential code
// (specs/service-order-query/).
type OrderCodeFinder interface {
	FindServiceOrderByCode(ctx context.Context, code int64) (*domain.ServiceOrder, error)
}

// TrackedOrderFinder resolves the order identified by its public code first
// (so an unknown code always maps to domain.ErrServiceOrderNotFound
// regardless of the token), then checks tokenHash against that specific
// order's active tracking token (domain.ErrTrackingTokenInvalid otherwise) —
// same precedence as service-order-tracking's FindByCodeAndTokenHash
// (specs/service-order-quote-decision/).
type TrackedOrderFinder interface {
	FindServiceOrderByCodeWithTrackingToken(ctx context.Context, code int64, tokenHash string) (*domain.ServiceOrder, error)
}

// StatusTransitionStore persists the status transitions that write only the
// order and its history entry. Each method re-checks the expected current
// status in the same transaction (domain.ErrInvalidStatusTransition when it
// changed concurrently). notes, when non-empty, is appended to the history
// entry's fixed description (specs/service-order-status-update/ design.md
// §2); the administrative endpoints always pass "".
type StatusTransitionStore interface {
	StartDiagnosis(ctx context.Context, order *domain.ServiceOrder, notes string) error
	FinalizeOrder(ctx context.Context, order *domain.ServiceOrder, notes string) error
	DeliverOrder(ctx context.Context, order *domain.ServiceOrder, notes string) error
}

// CustomerLookup reads the customer read model (domain.ErrCustomerNotFound
// when absent).
type CustomerLookup interface {
	FindCustomerByID(ctx context.Context, id uuid.UUID) (*CustomerRef, error)
	FindCustomerByDocument(ctx context.Context, normalizedDocument string) (*CustomerRef, error)
}

// VehicleLookup reads the vehicle read model (domain.ErrVehicleNotFound when
// absent).
type VehicleLookup interface {
	FindVehicleByID(ctx context.Context, id uuid.UUID) (*VehicleRef, error)
	FindVehicleByPlate(ctx context.Context, plate string) (*VehicleRef, error)
}

// ServiceCatalogLookup reads the service catalog read model.
type ServiceCatalogLookup interface {
	// FindServiceByID loads one service with its description and price
	// (domain.ErrRequestedServiceNotFound when absent).
	FindServiceByID(ctx context.Context, id uuid.UUID) (*ServiceRef, error)
	// FindServicesByIDs loads the display data (code, name) of ids.
	FindServicesByIDs(ctx context.Context, ids []uuid.UUID) ([]*ServiceRef, error)
	// FindMissingServiceIDs reports which of ids do not exist.
	FindMissingServiceIDs(ctx context.Context, ids []uuid.UUID) ([]uuid.UUID, error)
}

// ProductLookup reads the product catalog read model. Despite its name it
// returns inactive products too (Active false) — callers decide what an
// inactive product means; it returns domain.ErrProductNotFound when absent.
type ProductLookup interface {
	FindActiveProductByID(ctx context.Context, id uuid.UUID) (*ProductRef, error)
}

// QuoteReader loads an order's current quote and its items
// (domain.ErrQuoteNotFound when none has been composed yet).
type QuoteReader interface {
	FindQuoteByServiceOrderID(ctx context.Context, serviceOrderID uuid.UUID) (*domain.Quote, error)
}

// QuoteStore is QuoteReader plus every quote write, each transactional
// (RNF07).
type QuoteStore interface {
	QuoteReader

	// SaveQuote upserts the quote header (incrementing its version on a
	// recompose) and fully replaces its items.
	SaveQuote(ctx context.Context, order *domain.ServiceOrder, items []domain.QuoteItem, total float64) (*domain.Quote, error)

	// SendQuote records quote as sent (sent_at/sent_version) and transitions
	// order from IN_DIAGNOSIS to AWAITING_APPROVAL.
	SendQuote(ctx context.Context, order *domain.ServiceOrder, quote *domain.Quote) (*domain.Quote, error)

	// DecideQuote records the customer's decision on quote (APPROVED or
	// REJECTED, per decision), transitions order accordingly (IN_PROGRESS or
	// CANCELED), and writes the corresponding history entry, all in one
	// transaction: a failure at any step, including the history insert,
	// rolls back every other write this call made.
	DecideQuote(ctx context.Context, order *domain.ServiceOrder, quote *domain.Quote, decision domain.QuoteStatus) (*domain.Quote, error)
}

// ExecutionStore persists service executions (specs/service-order-execution/).
type ExecutionStore interface {
	StartExecution(ctx context.Context, execution *domain.ServiceExecution) error
	FinishExecution(ctx context.Context, execution *domain.ServiceExecution) error
	// FindServiceExecutionByID is scoped to serviceOrderID so a client cannot
	// finish another order's execution by guessing its id.
	FindServiceExecutionByID(ctx context.Context, serviceOrderID, executionID uuid.UUID) (*domain.ServiceExecution, error)
}

// ExecutionLister loads every execution of an order, used to check the
// required-executions rule before finalizing.
type ExecutionLister interface {
	FindServiceExecutionsByServiceOrderID(ctx context.Context, serviceOrderID uuid.UUID) ([]*domain.ServiceExecution, error)
}

// ServiceOrderQueryStore holds the read-only projections of
// specs/service-order-query/.
type ServiceOrderQueryStore interface {
	ListServiceOrders(ctx context.Context, filter ListFilter, page, pageSize int) ([]*ServiceOrderListItem, int, error)
	FindRequestedServices(ctx context.Context, serviceOrderID uuid.UUID) ([]*ServiceRef, error)
	FindHistoryByServiceOrderID(ctx context.Context, serviceOrderID uuid.UUID) ([]*domain.ServiceOrderHistory, error)
}

// StockLedger writes and reads the stock movements of an order
// (specs/service-order-stock-usage/).
type StockLedger interface {
	// RegisterStockUsage deducts every item's quantity from its product's
	// stock and records one EXIT movement per item, transactionally (RNF07):
	// a failure at any item rolls back every deduction already made.
	RegisterStockUsage(ctx context.Context, orderID uuid.UUID, items []StockUsageItem) ([]*domain.StockMovement, error)

	// ReverseStockMovement restores the quantity of a previously registered
	// EXIT movement and records a linked, inverse ENTRY movement,
	// transactionally (RNF07).
	ReverseStockMovement(ctx context.Context, orderID, movementID uuid.UUID) (*domain.StockMovement, error)

	// ListStockMovements loads every movement of an order, most recent first.
	ListStockMovements(ctx context.Context, orderID uuid.UUID) ([]*domain.StockMovement, error)
}

// ExecutionMetricsReader runs the aggregate behind the average-execution-time
// metric (specs/service-order-metrics/).
type ExecutionMetricsReader interface {
	FindAverageExecutionTimeByService(ctx context.Context, filter MetricsFilter) ([]*ServiceMetric, error)
}

// QuoteNotification is the data a QuoteNotifier needs to tell a customer their
// quote is available. It is an explicit DTO rather than the ServiceOrder
// aggregate because ServiceOrder only carries CustomerID — the recipient is
// resolved by the use case (specs/service-order-email-notification/).
// It deliberately has no customer name and no tracking token: the token is
// stored only as a hash and cannot be rebuilt here.
type QuoteNotification struct {
	OrderCode      int64
	TotalAmount    float64
	RecipientEmail string
}

// QuoteNotifier is the notification port
// specs/service-order-quote-decision/requirements.md asks for. SendQuote calls
// it on a best-effort basis after the send is already durably recorded — a
// notification failure must never undo or block a legitimate send.
// The use case only calls it with a non-empty RecipientEmail.
type QuoteNotifier interface {
	NotifyQuoteSent(ctx context.Context, notification QuoteNotification) error
}
