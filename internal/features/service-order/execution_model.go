package serviceorder

import (
	"time"

	"github.com/google/uuid"
)

// ServiceExecution is the Go name for the AuditServices entity
// (docs/entities.md) — one row per execution of a service within a service
// order, tracking its own start and end. Introduced by
// specs/service-order-execution/ (requirements.md §6, design.md §2.2), the
// first feature to implement this previously schema-only entity.
type ServiceExecution struct {
	ID             uuid.UUID
	ServiceOrderID uuid.UUID
	ServiceID      uuid.UUID
	StartedAt      time.Time
	EndedAt        *time.Time
}

// NewServiceExecution builds a new execution, always unfinished (EndedAt
// nil) — StartedAt is left zero here and filled in by the repository from
// the database's own now() (design.md §2.2), the same server-authored-date
// convention as ServiceOrder.OpenedAt/Quote.GeneratedAt.
func NewServiceExecution(serviceOrderID, serviceID uuid.UUID) (*ServiceExecution, error) {
	if serviceOrderID == uuid.Nil || serviceID == uuid.Nil {
		return nil, ErrInvalidAggregate
	}
	return &ServiceExecution{
		ID:             uuid.New(),
		ServiceOrderID: serviceOrderID,
		ServiceID:      serviceID,
	}, nil
}

// finish records the execution's end date/time (BR3), rejecting a second
// finish (BR "an execution's end cannot be recorded twice") and an end date
// before the start date (BR4). endedAt is nil when the caller wants the
// server-default "now" — that case is intentionally not compared against
// StartedAt here: StartedAt was authored by the database's own clock
// (execution_repository.go's StartExecution), and the default end time is
// likewise left to the database's now() (FinishExecution), so both come
// from the same clock and BR4 can never spuriously fire from app/DB clock
// drift. Only an explicit, caller-supplied endedAt is checked against
// StartedAt.
func (execution *ServiceExecution) finish(endedAt *time.Time) error {
	if execution.EndedAt != nil {
		return ErrServiceExecutionAlreadyFinished
	}
	if endedAt != nil && endedAt.Before(execution.StartedAt) {
		return ErrServiceExecutionEndBeforeStart
	}
	execution.EndedAt = endedAt
	return nil
}
