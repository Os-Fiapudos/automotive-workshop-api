package serviceorder

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// productRef is the minimal product-catalog data needed to price and
// describe a quote item, plus its Active flag for the RF-required
// active-product check. Package-local, deliberately not
// internal/features/product's Product type — same "no direct coupling
// between features" reasoning as customerRef/vehicleRef
// (specs/service-order-opening/design.md §1.4).
type productRef struct {
	ID          uuid.UUID
	Code        int64
	Name        string
	Description string
	UnitPrice   float64
	Active      bool
}

// findServiceOrderByID loads a ServiceOrder by id, used by StartDiagnosis
// and ComposeQuote/GetQuote to resolve the order before validating a status
// transition.
func (repository *PostgresServiceOrderRepository) findServiceOrderByID(ctx context.Context, id uuid.UUID) (*ServiceOrder, error) {
	order := &ServiceOrder{}
	err := repository.pool.QueryRow(ctx,
		`SELECT id, code, customer_id, vehicle_id, opened_at, status, notes, created_at, updated_at
		 FROM service_orders WHERE id = $1`, id,
	).Scan(&order.ID, &order.Code, &order.CustomerID, &order.VehicleID, &order.OpenedAt,
		&order.Status, &order.Notes, &order.CreatedAt, &order.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrServiceOrderNotFound
		}
		return nil, err
	}
	return order, nil
}

// findActiveProductByID loads a product's catalog data for pricing a quote
// item (requirements.md §3.11: must exist and be ACTIVE).
func (repository *PostgresServiceOrderRepository) findActiveProductByID(ctx context.Context, id uuid.UUID) (*productRef, error) {
	ref := &productRef{}
	err := repository.pool.QueryRow(ctx,
		`SELECT id, code, name, description, unit_price, status = 'ACTIVE' FROM products WHERE id = $1`, id,
	).Scan(&ref.ID, &ref.Code, &ref.Name, &ref.Description, &ref.UnitPrice, &ref.Active)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrProductNotFound
		}
		return nil, err
	}
	return ref, nil
}

// findServiceByID loads a single service's catalog data for pricing a quote
// item. Services have no status column (requirements.md §3.12) — existence
// is the only check.
func (repository *PostgresServiceOrderRepository) findServiceByID(ctx context.Context, id uuid.UUID) (*serviceRef, error) {
	ref := &serviceRef{}
	err := repository.pool.QueryRow(ctx,
		`SELECT id, code, name, description, price FROM services WHERE id = $1`, id,
	).Scan(&ref.ID, &ref.Code, &ref.Name, &ref.Description, &ref.Price)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrRequestedServiceNotFound
		}
		return nil, err
	}
	return ref, nil
}

// StartDiagnosis moves order to EM_DIAGNOSTICO and records the transition in
// service_order_history, transactionally (RNF07). The
// "AND status = 'RECEBIDA'" guard closes a race with a concurrent
// transition: zero rows affected is treated the same as the pre-checked
// ErrInvalidStatusTransition.
func (repository *PostgresServiceOrderRepository) StartDiagnosis(ctx context.Context, order *ServiceOrder) error {
	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op once Commit succeeds

	tag, err := tx.Exec(ctx,
		`UPDATE service_orders SET status = $2 WHERE id = $1 AND status = 'RECEBIDA'`,
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
		 VALUES ($1, 'diagnosis_started', $2, 'RECEBIDA', $3)`,
		order.ID, "Diagnosis started.", string(order.Status),
	); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// SaveQuote upserts the quote header, fully replaces its item rows, and
// transitions the order to AGUARDANDO_APROVACAO, all in one transaction
// (RNF07, design.md §1.6). A PUT is a full replace, not a diff: existing
// items are deleted and the new set is inserted.
func (repository *PostgresServiceOrderRepository) SaveQuote(ctx context.Context, order *ServiceOrder, items []QuoteItem, total float64) (*Quote, error) {
	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op once Commit succeeds

	quote := &Quote{ServiceOrderID: order.ID, TotalAmount: total, Status: QuoteStatusPending}
	err = tx.QueryRow(ctx,
		`INSERT INTO quotes (service_order_id, total_amount)
		 VALUES ($1, $2)
		 ON CONFLICT (service_order_id) DO UPDATE SET total_amount = EXCLUDED.total_amount
		 RETURNING id, code, status, generated_at, responded_at, created_at, updated_at`,
		order.ID, total,
	).Scan(&quote.ID, &quote.Code, &quote.Status, &quote.GeneratedAt, &quote.RespondedAt, &quote.CreatedAt, &quote.UpdatedAt)
	if err != nil {
		return nil, err
	}

	if _, err := tx.Exec(ctx, `DELETE FROM quote_products WHERE quote_id = $1`, quote.ID); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM quote_services WHERE quote_id = $1`, quote.ID); err != nil {
		return nil, err
	}

	for _, item := range items {
		switch item.Kind {
		case QuoteItemProduct:
			if _, err := tx.Exec(ctx,
				`INSERT INTO quote_products (quote_id, product_id, quantity, applied_description, applied_unit_price, applied_total_price)
				 VALUES ($1, $2, $3, $4, $5, $6)`,
				quote.ID, item.ProductID, item.Quantity, item.Description, item.UnitPrice, item.Total,
			); err != nil {
				return nil, err
			}
		case QuoteItemService:
			if _, err := tx.Exec(ctx,
				`INSERT INTO quote_services (quote_id, service_id, quantity, applied_description, applied_price, applied_total_price)
				 VALUES ($1, $2, $3, $4, $5, $6)`,
				quote.ID, item.ServiceID, item.Quantity, item.Description, item.UnitPrice, item.Total,
			); err != nil {
				return nil, err
			}
		}
	}

	if _, err := tx.Exec(ctx,
		`UPDATE service_orders SET status = $2 WHERE id = $1`,
		order.ID, string(order.Status),
	); err != nil {
		return nil, err
	}

	if _, err := tx.Exec(ctx,
		`INSERT INTO service_order_history (service_order_id, event, description, previous_status, new_status)
		 VALUES ($1, 'quote_composed', $2, $3, $3)`,
		order.ID, "Quote composed.", string(order.Status),
	); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	quote.Items = items
	return quote, nil
}

// FindQuoteByServiceOrderID loads the current quote and its items for an
// order. Returns ErrQuoteNotFound if no quote has been composed yet (a
// real, expected state — see docs/seed.sql order 4).
func (repository *PostgresServiceOrderRepository) FindQuoteByServiceOrderID(ctx context.Context, serviceOrderID uuid.UUID) (*Quote, error) {
	quote := &Quote{ServiceOrderID: serviceOrderID}
	err := repository.pool.QueryRow(ctx,
		`SELECT id, code, total_amount, status, generated_at, responded_at, created_at, updated_at
		 FROM quotes WHERE service_order_id = $1`, serviceOrderID,
	).Scan(&quote.ID, &quote.Code, &quote.TotalAmount, &quote.Status, &quote.GeneratedAt, &quote.RespondedAt, &quote.CreatedAt, &quote.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrQuoteNotFound
		}
		return nil, err
	}

	productRows, err := repository.pool.Query(ctx,
		`SELECT product_id, quantity, applied_description, applied_unit_price, applied_total_price
		 FROM quote_products WHERE quote_id = $1`, quote.ID)
	if err != nil {
		return nil, err
	}
	for productRows.Next() {
		item := QuoteItem{Kind: QuoteItemProduct}
		if err := productRows.Scan(&item.ProductID, &item.Quantity, &item.Description, &item.UnitPrice, &item.Total); err != nil {
			productRows.Close()
			return nil, err
		}
		quote.Items = append(quote.Items, item)
	}
	productRows.Close()
	if err := productRows.Err(); err != nil {
		return nil, err
	}

	serviceRows, err := repository.pool.Query(ctx,
		`SELECT service_id, quantity, applied_description, applied_price, applied_total_price
		 FROM quote_services WHERE quote_id = $1`, quote.ID)
	if err != nil {
		return nil, err
	}
	for serviceRows.Next() {
		item := QuoteItem{Kind: QuoteItemService}
		if err := serviceRows.Scan(&item.ServiceID, &item.Quantity, &item.Description, &item.UnitPrice, &item.Total); err != nil {
			serviceRows.Close()
			return nil, err
		}
		quote.Items = append(quote.Items, item)
	}
	serviceRows.Close()
	if err := serviceRows.Err(); err != nil {
		return nil, err
	}

	return quote, nil
}
