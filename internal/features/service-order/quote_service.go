package serviceorder

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"

	"automotive-workshop-api/internal/shared/trackingtoken"
)

// StartDiagnosis transitions a service order from RECEIVED to
// IN_DIAGNOSIS (requirements.md §3.1).
func (service *ServiceOrderService) StartDiagnosis(ctx context.Context, serviceOrderID uuid.UUID) (*ServiceOrder, error) {
	order, err := service.lookups.findServiceOrderByID(ctx, serviceOrderID)
	if err != nil {
		return nil, err
	}

	if err := order.startDiagnosis(); err != nil {
		return nil, err
	}

	if err := service.repository.StartDiagnosis(ctx, order); err != nil {
		return nil, err
	}

	return order, nil
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

	// Best-effort: the MVP has no real e-mail integration, and a
	// notification failure must never undo or block an already-recorded
	// send (specs/service-order-quote-decision/requirements.md).
	_ = service.notifier.NotifyQuoteSent(ctx, order, sent)

	return sent, nil
}

// ApproveQuote records the customer's approval of the service order's quote,
// identified by the order's public code and validated via its tracking
// token (RF12) — the same secure, non-JWT mechanism
// specs/service-order-tracking/ established. Approving moves the order to
// IN_PROGRESS. Returns the updated order alongside the quote since the
// public response reports both statuses.
func (service *ServiceOrderService) ApproveQuote(ctx context.Context, code int64, rawTrackingToken string) (*ServiceOrder, *Quote, error) {
	return service.decideQuote(ctx, code, rawTrackingToken, QuoteStatusApproved)
}

// RejectQuote records the customer's rejection of the service order's
// quote, identified and authenticated the same way as ApproveQuote.
// Rejecting moves the order to CANCELED.
func (service *ServiceOrderService) RejectQuote(ctx context.Context, code int64, rawTrackingToken string) (*ServiceOrder, *Quote, error) {
	return service.decideQuote(ctx, code, rawTrackingToken, QuoteStatusRejected)
}

// decideQuote is the shared implementation behind ApproveQuote/RejectQuote:
// resolve the order by code + tracking token (ErrServiceOrderNotFound for an
// unknown code, ErrTrackingTokenInvalid for a wrong/foreign/revoked token —
// so a customer can never respond to another order's quote), require the
// quote to still be PENDING (ErrQuoteAlreadyDecided otherwise — the same
// outcome whether this is a second, different decision or a repeat of the
// same one), then apply and persist the decision.
func (service *ServiceOrderService) decideQuote(ctx context.Context, code int64, rawTrackingToken string, decision QuoteStatus) (*ServiceOrder, *Quote, error) {
	if strings.TrimSpace(rawTrackingToken) == "" {
		return nil, nil, ErrTrackingTokenInvalid
	}

	order, err := service.lookups.findServiceOrderByCodeWithTrackingToken(ctx, code, trackingtoken.Hash(rawTrackingToken))
	if err != nil {
		return nil, nil, err
	}

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
