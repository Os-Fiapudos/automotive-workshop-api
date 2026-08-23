package serviceorder

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// StartExecution inserts a new, unfinished ServiceExecution row and fills in
// its database-assigned StartedAt (design.md §2.6). A single INSERT needs no
// transaction — nothing else is written alongside it (starting an execution
// does not change the order's status, requirements.md BR8).
func (repository *PostgresServiceOrderRepository) StartExecution(ctx context.Context, execution *ServiceExecution) error {
	return repository.pool.QueryRow(ctx,
		`INSERT INTO audit_services (id, service_order_id, service_id)
		 VALUES ($1, $2, $3)
		 RETURNING started_at`,
		execution.ID, execution.ServiceOrderID, execution.ServiceID,
	).Scan(&execution.StartedAt)
}

// FinishExecution persists execution.EndedAt. A nil EndedAt (the "use now"
// case, design.md §2.2) is resolved by the database's own now() via
// COALESCE, then read back — the same clock that authored StartedAt at
// INSERT time, so the two can never appear inverted purely from app/DB
// clock drift. The "AND ended_at IS NULL" guard closes a race with a
// concurrent finish of the same execution, same pattern as StartDiagnosis's
// status guard — zero rows affected is treated as
// ErrServiceExecutionAlreadyFinished, since existence was already confirmed
// by findServiceExecutionByID earlier in the same request.
func (repository *PostgresServiceOrderRepository) FinishExecution(ctx context.Context, execution *ServiceExecution) error {
	err := repository.pool.QueryRow(ctx,
		`UPDATE audit_services SET ended_at = COALESCE($2, now())
		 WHERE id = $1 AND ended_at IS NULL
		 RETURNING ended_at`,
		execution.ID, execution.EndedAt,
	).Scan(&execution.EndedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrServiceExecutionAlreadyFinished
		}
		return err
	}
	return nil
}

// FinalizeOrder moves order to FINALIZADA and records the transition in
// service_order_history, transactionally (RNF07) — same shape as
// StartDiagnosis.
func (repository *PostgresServiceOrderRepository) FinalizeOrder(ctx context.Context, order *ServiceOrder) error {
	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op once Commit succeeds

	tag, err := tx.Exec(ctx,
		`UPDATE service_orders SET status = $2 WHERE id = $1 AND status = 'EM_EXECUCAO'`,
		order.ID, string(order.Status),
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrInvalidStatusTransition
	}

	if _, err := tx.Exec(ctx,
		`INSERT INTO service_order_history (service_order_id, event, description, previous_status, new_status)
		 VALUES ($1, 'completion', $2, 'EM_EXECUCAO', $3)`,
		order.ID, "Service order finalized.", string(order.Status),
	); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// DeliverOrder moves order to ENTREGUE and records the transition in
// service_order_history, transactionally (RNF07) — same shape as
// FinalizeOrder.
func (repository *PostgresServiceOrderRepository) DeliverOrder(ctx context.Context, order *ServiceOrder) error {
	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op once Commit succeeds

	tag, err := tx.Exec(ctx,
		`UPDATE service_orders SET status = $2 WHERE id = $1 AND status = 'FINALIZADA'`,
		order.ID, string(order.Status),
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrInvalidStatusTransition
	}

	if _, err := tx.Exec(ctx,
		`INSERT INTO service_order_history (service_order_id, event, description, previous_status, new_status)
		 VALUES ($1, 'delivery', $2, 'FINALIZADA', $3)`,
		order.ID, "Vehicle delivered to customer.", string(order.Status),
	); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// findServiceExecutionByID loads a single execution, scoped to
// serviceOrderID so a client cannot finish another order's execution by
// guessing its id.
func (repository *PostgresServiceOrderRepository) findServiceExecutionByID(ctx context.Context, serviceOrderID, executionID uuid.UUID) (*ServiceExecution, error) {
	execution := &ServiceExecution{}
	err := repository.pool.QueryRow(ctx,
		`SELECT id, service_order_id, service_id, started_at, ended_at
		 FROM audit_services WHERE id = $1 AND service_order_id = $2`,
		executionID, serviceOrderID,
	).Scan(&execution.ID, &execution.ServiceOrderID, &execution.ServiceID, &execution.StartedAt, &execution.EndedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrServiceExecutionNotFound
		}
		return nil, err
	}
	return execution, nil
}

// findServiceExecutionsByServiceOrderID loads every execution recorded for
// an order, used by FinalizeOrder to check the required-executions rule
// (design.md §2.3).
func (repository *PostgresServiceOrderRepository) findServiceExecutionsByServiceOrderID(ctx context.Context, serviceOrderID uuid.UUID) ([]*ServiceExecution, error) {
	rows, err := repository.pool.Query(ctx,
		`SELECT id, service_order_id, service_id, started_at, ended_at
		 FROM audit_services WHERE service_order_id = $1`, serviceOrderID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var executions []*ServiceExecution
	for rows.Next() {
		execution := &ServiceExecution{}
		if err := rows.Scan(&execution.ID, &execution.ServiceOrderID, &execution.ServiceID, &execution.StartedAt, &execution.EndedAt); err != nil {
			return nil, err
		}
		executions = append(executions, execution)
	}
	return executions, rows.Err()
}
