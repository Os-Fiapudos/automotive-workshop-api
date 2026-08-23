package serviceorder

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

// StartExecution registers the start of a service's execution within a
// service order. The order must already be EM_EXECUCAO (BR2 — see
// requirements.md §2.1 for why reaching that status is itself an external
// precondition this feature does not produce) and the service must exist in
// the catalog.
func (service *ServiceOrderService) StartExecution(ctx context.Context, serviceOrderID, serviceID uuid.UUID) (*ServiceExecution, error) {
	order, err := service.lookups.findServiceOrderByID(ctx, serviceOrderID)
	if err != nil {
		return nil, err
	}
	if order.Status != StatusEmExecucao {
		return nil, ErrInvalidStatusTransition
	}

	if _, err := service.lookups.findServiceByID(ctx, serviceID); err != nil {
		return nil, err
	}

	execution, err := NewServiceExecution(order.ID, serviceID)
	if err != nil {
		return nil, err
	}

	if err := service.repository.StartExecution(ctx, execution); err != nil {
		return nil, err
	}
	return execution, nil
}

// FinishExecution records the end date/time of a previously started
// execution (BR3/BR4). endedAt is optional — nil means "now" (design.md
// §2.2). The order must still be EM_EXECUCAO (BR6: a finalized order accepts
// no more finishes).
func (service *ServiceOrderService) FinishExecution(ctx context.Context, serviceOrderID, executionID uuid.UUID, endedAt *time.Time) (*ServiceExecution, error) {
	order, err := service.lookups.findServiceOrderByID(ctx, serviceOrderID)
	if err != nil {
		return nil, err
	}
	if order.Status != StatusEmExecucao {
		return nil, ErrInvalidStatusTransition
	}

	execution, err := service.lookups.findServiceExecutionByID(ctx, serviceOrderID, executionID)
	if err != nil {
		return nil, err
	}

	if err := execution.finish(endedAt); err != nil {
		return nil, err
	}

	if err := service.repository.FinishExecution(ctx, execution); err != nil {
		return nil, err
	}
	return execution, nil
}

// FinalizeOrder transitions the order to FINALIZADA once every required
// execution — one per distinct service line item of its approved quote — is
// complete (BR5, design.md §2.3).
func (service *ServiceOrderService) FinalizeOrder(ctx context.Context, serviceOrderID uuid.UUID) (*ServiceOrder, error) {
	order, err := service.lookups.findServiceOrderByID(ctx, serviceOrderID)
	if err != nil {
		return nil, err
	}
	if order.Status != StatusEmExecucao {
		return nil, ErrInvalidStatusTransition
	}

	quote, err := service.repository.FindQuoteByServiceOrderID(ctx, serviceOrderID)
	if err != nil && !errors.Is(err, ErrQuoteNotFound) {
		return nil, err
	}

	if quote != nil {
		if err := service.ensureRequiredExecutionsComplete(ctx, order.ID, quote); err != nil {
			return nil, err
		}
	}

	if err := order.finalize(); err != nil {
		return nil, err
	}
	if err := service.repository.FinalizeOrder(ctx, order); err != nil {
		return nil, err
	}
	return order, nil
}

// ensureRequiredExecutionsComplete implements BR5: every distinct service id
// among quote's QuoteItemService line items must have at least one execution
// with a recorded end date.
func (service *ServiceOrderService) ensureRequiredExecutionsComplete(ctx context.Context, serviceOrderID uuid.UUID, quote *Quote) error {
	required := make(map[uuid.UUID]bool)
	for _, item := range quote.Items {
		if item.Kind == QuoteItemService {
			required[item.ServiceID] = true
		}
	}
	if len(required) == 0 {
		return nil
	}

	executions, err := service.lookups.findServiceExecutionsByServiceOrderID(ctx, serviceOrderID)
	if err != nil {
		return err
	}
	completed := make(map[uuid.UUID]bool)
	for _, execution := range executions {
		if execution.EndedAt != nil {
			completed[execution.ServiceID] = true
		}
	}

	for serviceID := range required {
		if !completed[serviceID] {
			return ErrExecutionsNotCompleted
		}
	}
	return nil
}

// DeliverOrder transitions a FINALIZADA order to ENTREGUE (BR7).
func (service *ServiceOrderService) DeliverOrder(ctx context.Context, serviceOrderID uuid.UUID) (*ServiceOrder, error) {
	order, err := service.lookups.findServiceOrderByID(ctx, serviceOrderID)
	if err != nil {
		return nil, err
	}

	if err := order.deliver(); err != nil {
		return nil, err
	}
	if err := service.repository.DeliverOrder(ctx, order); err != nil {
		return nil, err
	}
	return order, nil
}
