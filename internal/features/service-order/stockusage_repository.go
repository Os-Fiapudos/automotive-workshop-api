package serviceorder

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// RegisterStockUsage implements ServiceOrderRepository
// (specs/service-order-stock-usage/design.md §4.1): re-confirms the order is
// IN_PROGRESS, then deducts every item's quantity from its product and
// records one EXIT StockMovement per item, all in one transaction (RNF07,
// BR6/BR7) — a failure at any item, including an unknown/inactive product or
// insufficient balance, rolls back every deduction already made in this
// call, since Commit is only ever reached after every item succeeds. Each
// item's UPDATE is the same guarded, atomic pattern
// internal/features/product/repository.go's AdjustStock already established
// for this exact race (BR8): Postgres serializes concurrent UPDATEs of the
// same row, so a concurrent call can never observe a stale balance.
func (repository *PostgresServiceOrderRepository) RegisterStockUsage(ctx context.Context, orderID uuid.UUID, items []StockUsageItem) ([]*StockMovement, error) {
	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op once Commit succeeds

	var status string
	if err := tx.QueryRow(ctx, `SELECT status FROM service_orders WHERE id = $1`, orderID).Scan(&status); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrServiceOrderNotFound
		}
		return nil, err
	}
	if status != string(StatusInProgress) {
		return nil, ErrInvalidStatusTransition
	}

	movements := make([]*StockMovement, 0, len(items))
	for _, item := range items {
		productID, err := uuid.Parse(item.ProductID)
		if err != nil {
			return nil, ErrProductNotFound
		}

		movement := &StockMovement{
			ID:             uuid.New(),
			ProductID:      productID,
			ServiceOrderID: &orderID,
			Type:           StockMovementExit,
			Quantity:       item.Quantity,
		}

		err = tx.QueryRow(ctx,
			`UPDATE products
			 SET current_stock = current_stock - $2, updated_at = now()
			 WHERE id = $1 AND status = 'ACTIVE' AND current_stock >= $2
			 RETURNING current_stock + $2, current_stock`,
			productID, item.Quantity,
		).Scan(&movement.PreviousStock, &movement.NewStock)
		if err != nil {
			if !errors.Is(err, pgx.ErrNoRows) {
				return nil, err
			}
			// Zero rows affected — disambiguate why, same fallback shape
			// product/repository.go's AdjustStock already uses.
			var productStatus string
			var currentStock int
			lookupErr := tx.QueryRow(ctx,
				`SELECT status, current_stock FROM products WHERE id = $1`, productID,
			).Scan(&productStatus, &currentStock)
			if errors.Is(lookupErr, pgx.ErrNoRows) {
				return nil, ErrProductNotFound
			}
			if lookupErr != nil {
				return nil, lookupErr
			}
			if productStatus != "ACTIVE" {
				return nil, ErrProductInactive
			}
			return nil, ErrInsufficientStock
		}

		if err := tx.QueryRow(ctx,
			`INSERT INTO stock_movements (id, product_id, service_order_id, type, quantity, previous_stock, new_stock)
			 VALUES ($1, $2, $3, 'EXIT', $4, $5, $6)
			 RETURNING occurred_at`,
			movement.ID, movement.ProductID, orderID, movement.Quantity, movement.PreviousStock, movement.NewStock,
		).Scan(&movement.OccurredAt); err != nil {
			return nil, err
		}

		movements = append(movements, movement)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return movements, nil
}

// ReverseStockMovement implements ServiceOrderRepository (design.md §4.2):
// loads the original movement scoped to orderID (so a client cannot reverse
// another order's movement by guessing its id), confirms it is an
// unreversed EXIT, re-credits the product, and inserts the linked inverse
// ENTRY movement, all in one transaction (RNF07, BR9).
func (repository *PostgresServiceOrderRepository) ReverseStockMovement(ctx context.Context, orderID, movementID uuid.UUID) (*StockMovement, error) {
	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op once Commit succeeds

	var original StockMovement
	var typeStr string
	err = tx.QueryRow(ctx,
		`SELECT id, product_id, type, quantity FROM stock_movements WHERE id = $1 AND service_order_id = $2`,
		movementID, orderID,
	).Scan(&original.ID, &original.ProductID, &typeStr, &original.Quantity)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrStockMovementNotFound
		}
		return nil, err
	}
	if typeStr != string(StockMovementExit) {
		return nil, ErrStockMovementNotReversible
	}

	var alreadyReversed bool
	if err := tx.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM stock_movements WHERE reversed_movement_id = $1)`, movementID,
	).Scan(&alreadyReversed); err != nil {
		return nil, err
	}
	if alreadyReversed {
		return nil, ErrStockMovementAlreadyReversed
	}

	reversal := &StockMovement{
		ID:                 uuid.New(),
		ProductID:          original.ProductID,
		ServiceOrderID:     &orderID,
		Type:               StockMovementEntry,
		Quantity:           original.Quantity,
		ReversedMovementID: &movementID,
	}

	if err := tx.QueryRow(ctx,
		`UPDATE products
		 SET current_stock = current_stock + $2, updated_at = now()
		 WHERE id = $1
		 RETURNING current_stock - $2, current_stock`,
		reversal.ProductID, reversal.Quantity,
	).Scan(&reversal.PreviousStock, &reversal.NewStock); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrProductNotFound
		}
		return nil, err
	}

	if err := tx.QueryRow(ctx,
		`INSERT INTO stock_movements (id, product_id, service_order_id, type, quantity, previous_stock, new_stock, reversed_movement_id)
		 VALUES ($1, $2, $3, 'ENTRY', $4, $5, $6, $7)
		 RETURNING occurred_at`,
		reversal.ID, reversal.ProductID, orderID, reversal.Quantity, reversal.PreviousStock, reversal.NewStock, movementID,
	).Scan(&reversal.OccurredAt); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return reversal, nil
}

// ListStockMovements implements ServiceOrderRepository (design.md §4.3) — a
// read, no transaction needed, same reasoning findServicesByIDs already
// applies.
func (repository *PostgresServiceOrderRepository) ListStockMovements(ctx context.Context, orderID uuid.UUID) ([]*StockMovement, error) {
	rows, err := repository.pool.Query(ctx,
		`SELECT id, product_id, service_order_id, type, quantity, previous_stock, new_stock, reason, reversed_movement_id, occurred_at
		 FROM stock_movements WHERE service_order_id = $1 ORDER BY occurred_at DESC`, orderID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var movements []*StockMovement
	for rows.Next() {
		movement := &StockMovement{}
		var typeStr string
		var reason *string
		if err := rows.Scan(
			&movement.ID, &movement.ProductID, &movement.ServiceOrderID, &typeStr,
			&movement.Quantity, &movement.PreviousStock, &movement.NewStock,
			&reason, &movement.ReversedMovementID, &movement.OccurredAt,
		); err != nil {
			return nil, err
		}
		movement.Type = StockMovementType(typeStr)
		if reason != nil {
			movement.Reason = *reason
		}
		movements = append(movements, movement)
	}
	return movements, rows.Err()
}
