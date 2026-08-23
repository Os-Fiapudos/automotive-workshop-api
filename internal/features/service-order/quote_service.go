package serviceorder

import (
	"context"
	"errors"

	"github.com/google/uuid"
)

// StartDiagnosis transitions a service order from RECEBIDA to
// EM_DIAGNOSTICO (requirements.md §3.1).
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

// ComposeQuote replaces the service order's quote with the given items,
// recalculates the total exclusively on the server (RF06), and moves the
// order to AGUARDANDO_APROVACAO (requirements.md §3.2, §3.9).
func (service *ServiceOrderService) ComposeQuote(ctx context.Context, serviceOrderID uuid.UUID, inputs []QuoteItemInput) (*Quote, error) {
	order, err := service.lookups.findServiceOrderByID(ctx, serviceOrderID)
	if err != nil {
		return nil, err
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

	if err := order.markAwaitingApproval(); err != nil {
		return nil, err
	}

	return service.repository.SaveQuote(ctx, order, items, total)
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
