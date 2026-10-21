package serviceorder

import (
	"automotive-workshop-api/internal/features/service-order/domain"
	"automotive-workshop-api/internal/features/service-order/usecase"
	"context"
	"errors"
	"strconv"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// findServiceOrderByCode loads a ServiceOrder by its sequential code,
// findServiceOrderByID's counterpart for the code branch of
// GET /api/v1/service-orders/{id} (design.md §1.2).
func (repository *PostgresServiceOrderRepository) FindServiceOrderByCode(ctx context.Context, code int64) (*domain.ServiceOrder, error) {
	order := &domain.ServiceOrder{}
	err := repository.pool.QueryRow(ctx,
		`SELECT id, code, customer_id, vehicle_id, opened_at, status, notes, created_at, updated_at
		 FROM service_orders WHERE code = $1`, code,
	).Scan(&order.ID, &order.Code, &order.CustomerID, &order.VehicleID, &order.OpenedAt,
		&order.Status, &order.Notes, &order.CreatedAt, &order.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrServiceOrderNotFound
		}
		return nil, err
	}
	return order, nil
}

// findRequestedServices loads the display data for every service requested
// when serviceOrderID was opened, joining service_order_requested_services
// against services directly — unlike findServicesByIDs (used by the create
// flow, which already has the ids in memory), a detail view loaded back from
// the database only has the order's id (design.md §1.7).
func (repository *PostgresServiceOrderRepository) FindRequestedServices(ctx context.Context, serviceOrderID uuid.UUID) ([]*usecase.ServiceRef, error) {
	rows, err := repository.pool.Query(ctx,
		`SELECT s.id, s.code, s.name
		 FROM service_order_requested_services r
		 JOIN services s ON s.id = r.service_id
		 WHERE r.service_order_id = $1`, serviceOrderID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var refs []*usecase.ServiceRef
	for rows.Next() {
		ref := &usecase.ServiceRef{}
		if err := rows.Scan(&ref.ID, &ref.Code, &ref.Name); err != nil {
			return nil, err
		}
		refs = append(refs, ref)
	}
	return refs, rows.Err()
}

// findHistoryByServiceOrderID loads a service order's full status-change
// trail, oldest first (requirements.md BR4).
func (repository *PostgresServiceOrderRepository) FindHistoryByServiceOrderID(ctx context.Context, serviceOrderID uuid.UUID) ([]*domain.ServiceOrderHistory, error) {
	rows, err := repository.pool.Query(ctx,
		`SELECT id, service_order_id, occurred_at, event, description, previous_status, new_status
		 FROM service_order_history
		 WHERE service_order_id = $1
		 ORDER BY occurred_at ASC`, serviceOrderID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var history []*domain.ServiceOrderHistory
	for rows.Next() {
		entry := &domain.ServiceOrderHistory{}
		if err := rows.Scan(&entry.ID, &entry.ServiceOrderID, &entry.OccurredAt, &entry.Event,
			&entry.Description, &entry.PreviousStatus, &entry.NewStatus); err != nil {
			return nil, err
		}
		history = append(history, entry)
	}
	return history, rows.Err()
}

// listServiceOrders retrieves a filtered, paginated page of service orders
// joined with their customer/vehicle (design.md §1.3/§1.4). Every filter is
// optional and combined with AND. When filter.Status is nil, the default,
// no-filter view excludes COMPLETED/DELIVERED/CANCELED and orders by status
// priority (IN_PROGRESS > AWAITING_APPROVAL > IN_DIAGNOSIS > RECEIVED), then
// oldest first; when filter.Status is set, every row already shares one
// status, so both the exclusion and the priority CASE are skipped in favor
// of plain oldest-first (requirements.md BR3, AC12/AC13).
func (repository *PostgresServiceOrderRepository) ListServiceOrders(ctx context.Context, filter usecase.ListFilter, page, pageSize int) ([]*usecase.ServiceOrderListItem, int, error) {
	offset := (page - 1) * pageSize

	query := `
		SELECT so.id, so.code, so.customer_id, so.vehicle_id, so.opened_at, so.status, so.notes, so.created_at, so.updated_at,
		       c.id, c.code, c.name, c.status = 'ACTIVE', c.document, c.phone,
		       v.id, v.code, v.license_plate, v.customer_id, v.status = 'ACTIVE', v.brand, v.model, v.year, v.color,
		       COUNT(*) OVER() AS total
		FROM service_orders so
		JOIN customers c ON c.id = so.customer_id
		JOIN vehicles v ON v.id = so.vehicle_id
		WHERE 1=1`
	args := []any{}
	argIdx := 1

	if filter.Code != nil {
		query += ` AND so.code = $` + strconv.Itoa(argIdx)
		args = append(args, *filter.Code)
		argIdx++
	}
	if filter.Status != nil {
		query += ` AND so.status = $` + strconv.Itoa(argIdx)
		args = append(args, *filter.Status)
		argIdx++
	}
	if filter.CustomerDocument != "" {
		query += ` AND c.document = $` + strconv.Itoa(argIdx)
		args = append(args, filter.CustomerDocument)
		argIdx++
	}
	if filter.LicensePlate != "" {
		query += ` AND v.license_plate = $` + strconv.Itoa(argIdx)
		args = append(args, filter.LicensePlate)
		argIdx++
	}
	if filter.CreatedFrom != nil {
		query += ` AND so.created_at >= $` + strconv.Itoa(argIdx)
		args = append(args, *filter.CreatedFrom)
		argIdx++
	}
	if filter.CreatedTo != nil {
		query += ` AND so.created_at <= $` + strconv.Itoa(argIdx)
		args = append(args, *filter.CreatedTo)
		argIdx++
	}

	if filter.Status == nil {
		query += ` AND so.status NOT IN ('COMPLETED', 'DELIVERED', 'CANCELED')
			ORDER BY
			  CASE so.status
			    WHEN 'IN_PROGRESS'       THEN 1
			    WHEN 'AWAITING_APPROVAL' THEN 2
			    WHEN 'IN_DIAGNOSIS'      THEN 3
			    WHEN 'RECEIVED'          THEN 4
			  END,
			  so.created_at ASC,
			  so.code ASC`
	} else {
		query += ` ORDER BY so.created_at ASC, so.code ASC`
	}

	query += ` LIMIT $` + strconv.Itoa(argIdx) + ` OFFSET $` + strconv.Itoa(argIdx+1)
	args = append(args, pageSize, offset)

	rows, err := repository.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var items []*usecase.ServiceOrderListItem
	total := 0
	for rows.Next() {
		order := &domain.ServiceOrder{}
		customer := &usecase.CustomerRef{}
		vehicle := &usecase.VehicleRef{}
		if err := rows.Scan(
			&order.ID, &order.Code, &order.CustomerID, &order.VehicleID, &order.OpenedAt, &order.Status, &order.Notes, &order.CreatedAt, &order.UpdatedAt,
			&customer.ID, &customer.Code, &customer.Name, &customer.Active, &customer.Document, &customer.Phone,
			&vehicle.ID, &vehicle.Code, &vehicle.LicensePlate, &vehicle.CustomerID, &vehicle.Active, &vehicle.Brand, &vehicle.Model, &vehicle.Year, &vehicle.Color,
			&total,
		); err != nil {
			return nil, 0, err
		}
		items = append(items, &usecase.ServiceOrderListItem{Order: order, Customer: customer, Vehicle: vehicle})
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	return items, total, nil
}
