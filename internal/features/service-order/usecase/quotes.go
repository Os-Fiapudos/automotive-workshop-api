package usecase

import (
	"context"
	"errors"
	"log"
	"strings"

	"github.com/google/uuid"

	"automotive-workshop-api/internal/features/service-order/domain"
	"automotive-workshop-api/internal/shared/trackingtoken"
)

// QuoteItemInput is a raw, unresolved line item as supplied by a compose
// request — mirrors CreateInput's "raw strings in, resolved refs out" shape
// (see Opening.Create).
type QuoteItemInput struct {
	Kind      domain.QuoteItemKind
	ProductID string
	ServiceID string
	Quantity  int
}

// QuoteDependencies lists the ports Quotes needs. It is a struct rather than
// positional parameters because Quotes has seven collaborators and the
// Postgres adapter satisfies six of them with the same value.
type QuoteDependencies struct {
	Orders        OrderFinder
	TrackedOrders TrackedOrderFinder
	Quotes        QuoteStore
	Products      ProductLookup
	Catalog       ServiceCatalogLookup
	Customers     CustomerLookup
	// Notifier is the quote-sent notification port
	// (specs/service-order-quote-decision/). Nil means "notify nobody".
	Notifier QuoteNotifier
}

// Quotes groups the quote use cases: composition
// (specs/service-order-diagnosis-quote/), sending and the customer's
// decision on any channel (specs/service-order-quote-decision/), and the
// best-effort quote-sent e-mail (specs/service-order-email-notification/).
// externalNotificationSecret authenticates the external quote-decision
// channel (specs/service-order-quote-decision/design.md §5) — set once at
// wiring time via WithExternalNotificationSecret; left empty, that channel
// rejects every caller.
type Quotes struct {
	orders                     OrderFinder
	trackedOrders              TrackedOrderFinder
	quotes                     QuoteStore
	products                   ProductLookup
	catalog                    ServiceCatalogLookup
	customers                  CustomerLookup
	notifier                   QuoteNotifier
	externalNotificationSecret string
}

// NewQuotes builds the quote use cases. A nil Notifier is replaced by one
// that does nothing, so callers (and tests) that only care about the other
// use cases don't have to wire one explicitly.
func NewQuotes(dependencies QuoteDependencies) *Quotes {
	notifier := dependencies.Notifier
	if notifier == nil {
		notifier = discardNotifier{}
	}
	return &Quotes{
		orders:        dependencies.Orders,
		trackedOrders: dependencies.TrackedOrders,
		quotes:        dependencies.Quotes,
		products:      dependencies.Products,
		catalog:       dependencies.Catalog,
		customers:     dependencies.Customers,
		notifier:      notifier,
	}
}

// discardNotifier is the QuoteNotifier NewQuotes falls back to.
type discardNotifier struct{}

func (discardNotifier) NotifyQuoteSent(context.Context, QuoteNotification) error { return nil }

// WithExternalNotificationSecret sets the shared secret the external
// quote-decision endpoint authenticates against (design.md §5.4) and returns
// the same use case, so it chains onto NewQuotes at wiring time.
func (quotes *Quotes) WithExternalNotificationSecret(secret string) *Quotes {
	quotes.externalNotificationSecret = secret
	return quotes
}

// ComposeQuote replaces the service order's quote with the given items and
// recalculates the total exclusively on the server (RF06) (requirements.md
// §3.2, §3.9). It no longer transitions the order to AWAITING_APPROVAL —
// specs/service-order-quote-decision/ moved that transition to SendQuote,
// which owns the explicit "send to customer" step; composing/recomposing
// only requires diagnosis to have started (order not RECEIVED).
func (quotes *Quotes) ComposeQuote(ctx context.Context, serviceOrderID uuid.UUID, inputs []QuoteItemInput) (*domain.Quote, error) {
	order, err := quotes.orders.FindServiceOrderByID(ctx, serviceOrderID)
	if err != nil {
		return nil, err
	}
	if order.Status == domain.StatusReceived {
		return nil, domain.ErrDiagnosisNotStarted
	}

	existing, err := quotes.quotes.FindQuoteByServiceOrderID(ctx, serviceOrderID)
	if err != nil && !errors.Is(err, domain.ErrQuoteNotFound) {
		return nil, err
	}
	if existing != nil && existing.Status != domain.QuoteStatusPending {
		return nil, domain.ErrQuoteAlreadyDecided
	}

	items, err := quotes.resolveQuoteItems(ctx, inputs)
	if err != nil {
		return nil, err
	}
	if err := domain.ValidateQuoteItems(items); err != nil {
		return nil, err
	}
	total := domain.CalculateTotal(items)

	return quotes.quotes.SaveQuote(ctx, order, items, total)
}

// SendQuote sends the service order's composed quote to the customer
// (specs/service-order-quote-decision/requirements.md): only a complete
// quote (at least one item) can be sent, and only while the order is
// IN_DIAGNOSIS — sending is what moves it to AWAITING_APPROVAL.
func (quotes *Quotes) SendQuote(ctx context.Context, serviceOrderID uuid.UUID) (*domain.Quote, error) {
	order, err := quotes.orders.FindServiceOrderByID(ctx, serviceOrderID)
	if err != nil {
		return nil, err
	}

	quote, err := quotes.quotes.FindQuoteByServiceOrderID(ctx, serviceOrderID)
	if err != nil {
		return nil, err
	}
	if err := domain.ValidateQuoteItems(quote.Items); err != nil {
		return nil, err
	}

	if err := order.SendQuote(); err != nil {
		return nil, err
	}

	sent, err := quotes.quotes.SendQuote(ctx, order, quote)
	if err != nil {
		return nil, err
	}

	quotes.notifyQuoteSent(ctx, order, sent)

	return sent, nil
}

// notifyQuoteSent is the best-effort notification step after a committed
// send: no failure here (customer lookup or provider) may undo or alter the
// send (specs/service-order-quote-decision/requirements.md §4 item 10). There
// is no retry or outbox — a failure is only logged. The log carries the order
// code and an outcome only, never the recipient, name, token or credentials.
func (quotes *Quotes) notifyQuoteSent(ctx context.Context, order *domain.ServiceOrder, quote *domain.Quote) {
	customer, err := quotes.customers.FindCustomerByID(ctx, order.CustomerID)
	if err != nil {
		log.Printf("quote notification: order %d: customer lookup failed", order.Code)
		return
	}

	notification := QuoteNotification{OrderCode: order.Code, TotalAmount: quote.TotalAmount}
	if customer.Email != nil {
		notification.RecipientEmail = strings.TrimSpace(*customer.Email)
	}
	if notification.RecipientEmail == "" {
		log.Printf("quote notification: order %d: skipped, customer has no e-mail", order.Code)
		return
	}

	if err := quotes.notifier.NotifyQuoteSent(ctx, notification); err != nil {
		log.Printf("quote notification: order %d: send failed", order.Code)
		return
	}
	log.Printf("quote notification: order %d: sent", order.Code)
}

// ApproveQuote records the customer's approval of the service order's quote,
// identified by the order's public code and validated via its tracking
// token (RF12) — the same secure, non-JWT mechanism
// specs/service-order-tracking/ established. Approving moves the order to
// IN_PROGRESS. Returns the updated order alongside the quote since the
// public response reports both statuses.
func (quotes *Quotes) ApproveQuote(ctx context.Context, code int64, rawTrackingToken string) (*domain.ServiceOrder, *domain.Quote, error) {
	order, err := quotes.resolveOrderByTrackingToken(ctx, code, rawTrackingToken)
	if err != nil {
		return nil, nil, err
	}
	return quotes.decideQuote(ctx, order, domain.QuoteStatusApproved)
}

// RejectQuote records the customer's rejection of the service order's
// quote, identified and authenticated the same way as ApproveQuote.
// Rejecting moves the order to CANCELED.
func (quotes *Quotes) RejectQuote(ctx context.Context, code int64, rawTrackingToken string) (*domain.ServiceOrder, *domain.Quote, error) {
	order, err := quotes.resolveOrderByTrackingToken(ctx, code, rawTrackingToken)
	if err != nil {
		return nil, nil, err
	}
	return quotes.decideQuote(ctx, order, domain.QuoteStatusRejected)
}

// DecideQuoteExternal records a decision relayed by an external system
// (specs/service-order-quote-decision/requirements.md §9, design.md §5.2).
// The caller has already been authenticated by the shared secret
// (AuthenticateExternalNotification); the order is resolved by its
// technical id, since no tracking token exists on this path, and the
// decision then goes through exactly the same decideQuote core as the
// customer-facing channel — same preconditions, same transaction.
func (quotes *Quotes) DecideQuoteExternal(ctx context.Context, serviceOrderID uuid.UUID, decision domain.QuoteStatus) (*domain.ServiceOrder, *domain.Quote, error) {
	if decision != domain.QuoteStatusApproved && decision != domain.QuoteStatusRejected {
		return nil, nil, domain.ErrInvalidQuoteDecision
	}

	order, err := quotes.orders.FindServiceOrderByID(ctx, serviceOrderID)
	if err != nil {
		return nil, nil, err
	}
	return quotes.decideQuote(ctx, order, decision)
}

// AuthenticateExternalNotification reports whether presented matches the
// configured external-notification secret (design.md §5.1). An unconfigured
// (empty) secret never authenticates anyone.
func (quotes *Quotes) AuthenticateExternalNotification(presented string) bool {
	return secretMatches(quotes.externalNotificationSecret, presented)
}

// resolveOrderByTrackingToken resolves the order by code + tracking token
// (domain.ErrServiceOrderNotFound for an unknown code,
// domain.ErrTrackingTokenInvalid for a missing/wrong/foreign/revoked token —
// so a customer can never respond to another order's quote).
func (quotes *Quotes) resolveOrderByTrackingToken(ctx context.Context, code int64, rawTrackingToken string) (*domain.ServiceOrder, error) {
	if strings.TrimSpace(rawTrackingToken) == "" {
		return nil, domain.ErrTrackingTokenInvalid
	}
	return quotes.trackedOrders.FindServiceOrderByCodeWithTrackingToken(ctx, code, trackingtoken.Hash(rawTrackingToken))
}

// decideQuote is the single decision core behind every channel
// (ApproveQuote/RejectQuote and DecideQuoteExternal — design.md §5.2): given
// an already-resolved order, require the quote to still be PENDING
// (domain.ErrQuoteAlreadyDecided otherwise — the same outcome whether this
// is a second, different decision or a repeat of the same one), then apply
// and persist the decision in DecideQuote's single transaction (RNF07).
func (quotes *Quotes) decideQuote(ctx context.Context, order *domain.ServiceOrder, decision domain.QuoteStatus) (*domain.ServiceOrder, *domain.Quote, error) {
	quote, err := quotes.quotes.FindQuoteByServiceOrderID(ctx, order.ID)
	if err != nil {
		return nil, nil, err
	}
	if quote.Status != domain.QuoteStatusPending {
		return nil, nil, domain.ErrQuoteAlreadyDecided
	}

	var transitionErr error
	if decision == domain.QuoteStatusApproved {
		transitionErr = order.ApproveQuote()
	} else {
		transitionErr = order.RejectQuote()
	}
	if transitionErr != nil {
		return nil, nil, transitionErr
	}

	decided, err := quotes.quotes.DecideQuote(ctx, order, quote, decision)
	if err != nil {
		return nil, nil, err
	}
	return order, decided, nil
}

// GetQuote reads the current quote and its items for a service order.
func (quotes *Quotes) GetQuote(ctx context.Context, serviceOrderID uuid.UUID) (*domain.Quote, error) {
	if _, err := quotes.orders.FindServiceOrderByID(ctx, serviceOrderID); err != nil {
		return nil, err
	}
	return quotes.quotes.FindQuoteByServiceOrderID(ctx, serviceOrderID)
}

// resolveQuoteItems resolves and prices every raw input against the
// catalog, building the immutable snapshot each QuoteItem persists
// (requirements.md §3.5, §3.11, §3.12).
func (quotes *Quotes) resolveQuoteItems(ctx context.Context, inputs []QuoteItemInput) ([]domain.QuoteItem, error) {
	items := make([]domain.QuoteItem, 0, len(inputs))
	for _, input := range inputs {
		switch input.Kind {
		case domain.QuoteItemProduct:
			id, err := uuid.Parse(input.ProductID)
			if err != nil {
				return nil, domain.ErrProductNotFound
			}
			product, err := quotes.products.FindActiveProductByID(ctx, id)
			if err != nil {
				return nil, err
			}
			if !product.Active {
				return nil, domain.ErrProductInactive
			}
			items = append(items, domain.QuoteItem{
				Kind:        domain.QuoteItemProduct,
				ProductID:   product.ID,
				Description: product.Description,
				Quantity:    input.Quantity,
				UnitPrice:   product.UnitPrice,
				Total:       float64(input.Quantity) * product.UnitPrice,
			})

		case domain.QuoteItemService:
			id, err := uuid.Parse(input.ServiceID)
			if err != nil {
				return nil, domain.ErrRequestedServiceNotFound
			}
			catalogService, err := quotes.catalog.FindServiceByID(ctx, id)
			if err != nil {
				return nil, err
			}
			items = append(items, domain.QuoteItem{
				Kind:        domain.QuoteItemService,
				ServiceID:   catalogService.ID,
				Description: catalogService.Description,
				Quantity:    input.Quantity,
				UnitPrice:   catalogService.Price,
				Total:       float64(input.Quantity) * catalogService.Price,
			})
		}
	}
	return items, nil
}
