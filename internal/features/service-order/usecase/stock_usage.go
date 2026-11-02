package usecase

import (
	"context"

	"github.com/google/uuid"

	"automotive-workshop-api/internal/features/service-order/domain"
)

// StockUsageItem is one raw line of a usage-deduction request: a product and
// the quantity taken from it (specs/service-order-stock-usage/requirements.md
// §3, BR3).
type StockUsageItem struct {
	ProductID string
	Quantity  int
}

// validateStockUsageItems enforces the business rules that don't need
// repository access: at least one item (BR7 implies a request has items to
// roll back), every quantity strictly positive (BR3). Existence, active
// status, and balance are enforced atomically by the repository (design.md
// §3) to avoid a check-then-act race with a concurrent request (BR8).
func validateStockUsageItems(items []StockUsageItem) error {
	if len(items) == 0 {
		return domain.ErrEmptyStockUsage
	}
	for _, item := range items {
		if item.Quantity <= 0 {
			return domain.ErrInvalidQuantity
		}
	}
	return nil
}

// StockUsage groups the parts/supplies usage use cases
// (specs/service-order-stock-usage/).
type StockUsage struct {
	orders OrderFinder
	ledger StockLedger
}

func NewStockUsage(orders OrderFinder, ledger StockLedger) *StockUsage {
	return &StockUsage{orders: orders, ledger: ledger}
}

// RegisterStockUsage registers one or more parts/supplies deductions against
// a service order that must already be IN_PROGRESS (BR1). Product
// existence/active/balance checks are not duplicated here — they are
// enforced atomically by the repository's guarded UPDATE (design.md §3), to
// avoid a check-then-act race with a concurrent request (BR8).
func (usage *StockUsage) RegisterStockUsage(ctx context.Context, serviceOrderID uuid.UUID, items []StockUsageItem) ([]*domain.StockMovement, error) {
	order, err := usage.orders.FindServiceOrderByID(ctx, serviceOrderID)
	if err != nil {
		return nil, err
	}
	if order.Status != domain.StatusInProgress {
		return nil, domain.ErrInvalidStatusTransition
	}

	if err := validateStockUsageItems(items); err != nil {
		return nil, err
	}

	return usage.ledger.RegisterStockUsage(ctx, order.ID, items)
}

// ReverseStockMovement reverses a previously registered usage movement
// belonging to serviceOrderID, restoring the deducted quantity (BR9).
func (usage *StockUsage) ReverseStockMovement(ctx context.Context, serviceOrderID, movementID uuid.UUID) (*domain.StockMovement, error) {
	if _, err := usage.orders.FindServiceOrderByID(ctx, serviceOrderID); err != nil {
		return nil, err
	}
	return usage.ledger.ReverseStockMovement(ctx, serviceOrderID, movementID)
}

// ListStockMovements reads every stock movement recorded against a service
// order.
func (usage *StockUsage) ListStockMovements(ctx context.Context, serviceOrderID uuid.UUID) ([]*domain.StockMovement, error) {
	if _, err := usage.orders.FindServiceOrderByID(ctx, serviceOrderID); err != nil {
		return nil, err
	}
	return usage.ledger.ListStockMovements(ctx, serviceOrderID)
}
