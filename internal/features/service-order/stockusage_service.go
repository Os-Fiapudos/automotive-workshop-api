package serviceorder

import (
	"context"

	"github.com/google/uuid"
)

// RegisterStockUsage registers one or more parts/supplies deductions against
// a service order that must already be EM_EXECUCAO (BR1). Product
// existence/active/balance checks are not duplicated here — they are
// enforced atomically by the repository's guarded UPDATE (design.md §3), to
// avoid a check-then-act race with a concurrent request (BR8).
func (service *ServiceOrderService) RegisterStockUsage(ctx context.Context, serviceOrderID uuid.UUID, items []StockUsageItem) ([]*StockMovement, error) {
	order, err := service.lookups.findServiceOrderByID(ctx, serviceOrderID)
	if err != nil {
		return nil, err
	}
	if order.Status != StatusEmExecucao {
		return nil, ErrInvalidStatusTransition
	}

	if err := validateStockUsageItems(items); err != nil {
		return nil, err
	}

	return service.repository.RegisterStockUsage(ctx, order.ID, items)
}

// ReverseStockMovement reverses a previously registered usage movement
// belonging to serviceOrderID, restoring the deducted quantity (BR9).
func (service *ServiceOrderService) ReverseStockMovement(ctx context.Context, serviceOrderID, movementID uuid.UUID) (*StockMovement, error) {
	if _, err := service.lookups.findServiceOrderByID(ctx, serviceOrderID); err != nil {
		return nil, err
	}
	return service.repository.ReverseStockMovement(ctx, serviceOrderID, movementID)
}

// ListStockMovements reads every stock movement recorded against a service
// order.
func (service *ServiceOrderService) ListStockMovements(ctx context.Context, serviceOrderID uuid.UUID) ([]*StockMovement, error) {
	if _, err := service.lookups.findServiceOrderByID(ctx, serviceOrderID); err != nil {
		return nil, err
	}
	return service.repository.ListStockMovements(ctx, serviceOrderID)
}
