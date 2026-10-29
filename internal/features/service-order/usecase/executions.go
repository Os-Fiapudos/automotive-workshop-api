package usecase

import (
	"context"
	"time"

	"github.com/google/uuid"

	"automotive-workshop-api/internal/features/service-order/domain"
)

// Executions groups the service-execution use cases
// (specs/service-order-execution/). Finalizing and delivering the order live
// in Lifecycle, since the external status update shares their cores.
type Executions struct {
	orders     OrderFinder
	catalog    ServiceCatalogLookup
	executions ExecutionStore
}

func NewExecutions(orders OrderFinder, catalog ServiceCatalogLookup, executions ExecutionStore) *Executions {
	return &Executions{orders: orders, catalog: catalog, executions: executions}
}

// StartExecution registers the start of a service's execution within a
// service order. The order must already be IN_PROGRESS (BR2 — see
// requirements.md §2.1 for why reaching that status is itself an external
// precondition this feature does not produce) and the service must exist in
// the catalog.
func (executions *Executions) StartExecution(ctx context.Context, serviceOrderID, serviceID uuid.UUID) (*domain.ServiceExecution, error) {
	order, err := executions.orders.FindServiceOrderByID(ctx, serviceOrderID)
	if err != nil {
		return nil, err
	}
	if order.Status != domain.StatusInProgress {
		return nil, domain.ErrInvalidStatusTransition
	}

	if _, err := executions.catalog.FindServiceByID(ctx, serviceID); err != nil {
		return nil, err
	}

	execution, err := domain.NewServiceExecution(order.ID, serviceID)
	if err != nil {
		return nil, err
	}

	if err := executions.executions.StartExecution(ctx, execution); err != nil {
		return nil, err
	}
	return execution, nil
}

// FinishExecution records the end date/time of a previously started
// execution (BR3/BR4). endedAt is optional — nil means "now" (design.md
// §2.2). The order must still be IN_PROGRESS (BR6: a finalized order accepts
// no more finishes).
func (executions *Executions) FinishExecution(ctx context.Context, serviceOrderID, executionID uuid.UUID, endedAt *time.Time) (*domain.ServiceExecution, error) {
	order, err := executions.orders.FindServiceOrderByID(ctx, serviceOrderID)
	if err != nil {
		return nil, err
	}
	if order.Status != domain.StatusInProgress {
		return nil, domain.ErrInvalidStatusTransition
	}

	execution, err := executions.executions.FindServiceExecutionByID(ctx, serviceOrderID, executionID)
	if err != nil {
		return nil, err
	}

	if err := execution.Finish(endedAt); err != nil {
		return nil, err
	}

	if err := executions.executions.FinishExecution(ctx, execution); err != nil {
		return nil, err
	}
	return execution, nil
}
