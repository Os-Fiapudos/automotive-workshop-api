package serviceorder

import (
	"context"
	"errors"
	"log"
	"strings"

	"github.com/google/uuid"

	"automotive-workshop-api/internal/shared/trackingtoken"
)

// StartDiagnosis transitions a service order from RECEIVED to
// IN_DIAGNOSIS (requirements.md §3.1).
func (service *ServiceOrderService) StartDiagnosis(ctx context.Context, serviceOrderID uuid.UUID) (*ServiceOrder, error) {
	order, _, err := service.startDiagnosis(ctx, serviceOrderID, "")
	return order, err
}

// startDiagnosis is the single core behind StartDiagnosis and the external
// status update (specs/service-order-status-update/design.md §1): notes, if
// non-empty, is appended to the history entry. It also returns the status
// the order had before the transition.
func (service *ServiceOrderService) startDiagnosis(ctx context.Context, serviceOrderID uuid.UUID, notes string) (*ServiceOrder, Status, error) {
	order, err := service.lookups.findServiceOrderByID(ctx, serviceOrderID)
	if err != nil {
		return nil, "", err
	}
	previousStatus := order.Status

	if err := order.startDiagnosis(); err != nil {
		return nil, "", err
	}

	if err := service.repository.StartDiagnosis(ctx, order, notes); err != nil {
		return nil, "", err
	}

	return order, previousStatus, nil
}

// ComposeQuote replaces the service order's quote with the given items and
// recalculates the total exclusively on the server (RF06) (requirements.md
// §3.2, §3.9). It no longer transitions the order to AWAITING_APPROVAL —
// specs/service-order-quote-decision/ moved that transition to SendQuote,
// which owns the explicit "send to customer" step; composing/recomposing
// only requires diagnosis to have started (order not RECEIVED).
func (service *ServiceOrderService) ComposeQuote(ctx context.Context, serviceOrderID uuid.UUID, inputs []QuoteItemInput) (*Quote, error) {
	order, err := service.lookups.findServiceOrderByID(ctx, serviceOrderID)
	if err != nil {
		return nil, err
	}
	if order.Status == StatusReceived {
		return nil, ErrDiagnosisNotStarted
	}

	existing, err := service.repository.FindQuoteByServiceOrderID(ctx, serviceOrderID)
	if err != nil && !errors.Is(err, ErrQuoteNotFound) {
		return nil, err
	}
	if existing != nil && existing.Status != QuoteStatusPending {
		return nil, ErrQuoteAlreadyDecided
	}

	items, err := service.resolveQuoteItems(ctx, inputs)
	if err != nil {
		return nil, err
	}
	if err := validateQuoteItems(items); err != nil {
		return nil, err
	}
	total := calculateTotal(items)

	return service.repository.SaveQuote(ctx, order, items, total)
}

// SendQuote sends the service order's composed quote to the customer
// (specs/service-order-quote-decision/requirements.md): only a complete
// quote (at least one item) can be sent, and only while the order is
// IN_DIAGNOSIS — sending is what moves it to AWAITING_APPROVAL.
func (service *ServiceOrderService) SendQuote(ctx context.Context, serviceOrderID uuid.UUID) (*Quote, error) {
	order, err := service.lookups.findServiceOrderByID(ctx, serviceOrderID)
	if err != nil {
		return nil, err
	}

	quote, err := service.repository.FindQuoteByServiceOrderID(ctx, serviceOrderID)
	if err != nil {
		return nil, err
	}
	if err := validateQuoteItems(quote.Items); err != nil {
		return nil, err
	}

	if err := order.sendQuote(); err != nil {
		return nil, err
	}

	sent, err := service.repository.SendQuote(ctx, order, quote)
	if err != nil {
		return nil, err
	}

	service.notifyQuoteSent(ctx, order, sent)

	return sent, nil
}

// notifyQuoteSent is the best-effort notification step after a committed
// send: no failure here (customer lookup or provider) may undo or alter the
// send (specs/service-order-quote-decision/requirements.md §4 item 10). There
// is no retry or outbox — a failure is only logged. The log carries the order
// code and an outcome only, never the recipient, name, token or credentials.
func (service *ServiceOrderService) notifyQuoteSent(ctx context.Context, order *ServiceOrder, quote *Quote) {
	customer, err := service.lookups.findCustomerByID(ctx, order.CustomerID)
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

	if err := service.notifier.NotifyQuoteSent(ctx, notification); err != nil {
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
func (service *ServiceOrderService) ApproveQuote(ctx context.Context, code int64, rawTrackingToken string) (*ServiceOrder, *Quote, error) {
	order, err := service.resolveOrderByTrackingToken(ctx, code, rawTrackingToken)
	if err != nil {
		return nil, nil, err
	}
	return service.decideQuote(ctx, order, QuoteStatusApproved)
}

// RejectQuote records the customer's rejection of the service order's
// quote, identified and authenticated the same way as ApproveQuote.
// Rejecting moves the order to CANCELED.
func (service *ServiceOrderService) RejectQuote(ctx context.Context, code int64, rawTrackingToken string) (*ServiceOrder, *Quote, error) {
	order, err := service.resolveOrderByTrackingToken(ctx, code, rawTrackingToken)
	if err != nil {
		return nil, nil, err
	}
	return service.decideQuote(ctx, order, QuoteStatusRejected)
}

// DecideQuoteExternal records a decision relayed by an external system
// (specs/service-order-quote-decision/requirements.md §9, design.md §5.2).
// The caller has already been authenticated by the shared secret
// (AuthenticateExternalNotification); the order is resolved by its
// technical id, since no tracking token exists on this path, and the
// decision then goes through exactly the same decideQuote core as the
// customer-facing channel — same preconditions, same transaction.
func (service *ServiceOrderService) DecideQuoteExternal(ctx context.Context, serviceOrderID uuid.UUID, decision QuoteStatus) (*ServiceOrder, *Quote, error) {
	if decision != QuoteStatusApproved && decision != QuoteStatusRejected {
		return nil, nil, ErrInvalidQuoteDecision
	}

	order, err := service.lookups.findServiceOrderByID(ctx, serviceOrderID)
	if err != nil {
		return nil, nil, err
	}
	return service.decideQuote(ctx, order, decision)
}

// AuthenticateExternalNotification reports whether presented matches the
// configured external-notification secret (design.md §5.1). Both values are
// hashed before the constant-time comparison so neither the content nor the
// length of the secret leaks through timing. An unconfigured (empty) secret
// never authenticates anyone.
func (service *ServiceOrderService) AuthenticateExternalNotification(presented string) bool {
	return secretMatches(service.externalNotificationSecret, presented)
}

// resolveOrderByTrackingToken resolves the order by code + tracking token
// (ErrServiceOrderNotFound for an unknown code, ErrTrackingTokenInvalid for
// a missing/wrong/foreign/revoked token — so a customer can never respond to
// another order's quote).
func (service *ServiceOrderService) resolveOrderByTrackingToken(ctx context.Context, code int64, rawTrackingToken string) (*ServiceOrder, error) {
	if strings.TrimSpace(rawTrackingToken) == "" {
		return nil, ErrTrackingTokenInvalid
	}
	return service.lookups.findServiceOrderByCodeWithTrackingToken(ctx, code, trackingtoken.Hash(rawTrackingToken))
}

// decideQuote is the single decision core behind every channel
// (ApproveQuote/RejectQuote and DecideQuoteExternal — design.md §5.2): given
// an already-resolved order, require the quote to still be PENDING
// (ErrQuoteAlreadyDecided otherwise — the same outcome whether this is a
// second, different decision or a repeat of the same one), then apply and
// persist the decision in DecideQuote's single transaction (RNF07).
func (service *ServiceOrderService) decideQuote(ctx context.Context, order *ServiceOrder, decision QuoteStatus) (*ServiceOrder, *Quote, error) {
	quote, err := service.repository.FindQuoteByServiceOrderID(ctx, order.ID)
	if err != nil {
		return nil, nil, err
	}
	if quote.Status != QuoteStatusPending {
		return nil, nil, ErrQuoteAlreadyDecided
	}

	var transitionErr error
	if decision == QuoteStatusApproved {
		transitionErr = order.approveQuote()
	} else {
		transitionErr = order.rejectQuote()
	}
	if transitionErr != nil {
		return nil, nil, transitionErr
	}

	decided, err := service.repository.DecideQuote(ctx, order, quote, decision)
	if err != nil {
		return nil, nil, err
	}
	return order, decided, nil
}

// GetQuote reads the current quote and its items for a service order.
func (service *ServiceOrderService) GetQuote(ctx context.Context, serviceOrderID uuid.UUID) (*Quote, error) {
	if _, err := service.lookups.findServiceOrderByID(ctx, serviceOrderID); err != nil {
		return nil, err
	}
	return service.repository.FindQuoteByServiceOrderID(ctx, serviceOrderID)
}

// resolveQuoteItems resolves and prices every raw input against the
// catalog, building the immutable snapshot each QuoteItem persists
// (requirements.md §3.5, §3.11, §3.12).
func (service *ServiceOrderService) resolveQuoteItems(ctx context.Context, inputs []QuoteItemInput) ([]QuoteItem, error) {
	items := make([]QuoteItem, 0, len(inputs))
	for _, input := range inputs {
		switch input.Kind {
		case QuoteItemProduct:
			id, err := uuid.Parse(input.ProductID)
			if err != nil {
				return nil, ErrProductNotFound
			}
			product, err := service.lookups.findActiveProductByID(ctx, id)
			if err != nil {
				return nil, err
			}
			if !product.Active {
				return nil, ErrProductInactive
			}
			items = append(items, QuoteItem{
				Kind:        QuoteItemProduct,
				ProductID:   product.ID,
				Description: product.Description,
				Quantity:    input.Quantity,
				UnitPrice:   product.UnitPrice,
				Total:       float64(input.Quantity) * product.UnitPrice,
			})

		case QuoteItemService:
			id, err := uuid.Parse(input.ServiceID)
			if err != nil {
				return nil, ErrRequestedServiceNotFound
			}
			catalogService, err := service.lookups.findServiceByID(ctx, id)
			if err != nil {
				return nil, err
			}
			items = append(items, QuoteItem{
				Kind:        QuoteItemService,
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
