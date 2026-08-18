package serviceorder

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// customerRef is the minimal customer data this feature needs to resolve
// and validate a customer, plus display it in the response. It is a
// package-local projection, deliberately not the customer feature's
// Customer type — importing internal/features/customer directly would
// violate the "no direct coupling between features" rule (CLAUDE.md §9.2).
type customerRef struct {
	ID     uuid.UUID
	Code   int64
	Name   string
	Active bool
}

// vehicleRef is the vehicle-side equivalent of customerRef.
type vehicleRef struct {
	ID           uuid.UUID
	Code         int64
	LicensePlate string
	CustomerID   uuid.UUID
	Active       bool
}

// serviceRef is the minimal service-catalog data needed to display a
// requested service in the response.
type serviceRef struct {
	ID   uuid.UUID
	Code int64
	Name string
}

// serviceOrderLookups is the read-only boundary ServiceOrderService depends
// on to resolve and validate the customer, vehicle, and requested services
// referenced by a create request. It is defined here, next to
// ServiceOrderRepository, and implemented by the same Postgres type — the
// split into two interfaces exists so service_test.go can fake exactly what
// the service layer needs (design.md §3.2).
type serviceOrderLookups interface {
	findCustomerByID(ctx context.Context, id uuid.UUID) (*customerRef, error)
	findCustomerByDocument(ctx context.Context, normalizedDocument string) (*customerRef, error)
	findVehicleByID(ctx context.Context, id uuid.UUID) (*vehicleRef, error)
	findVehicleByPlate(ctx context.Context, plate string) (*vehicleRef, error)
	findMissingServiceIDs(ctx context.Context, ids []uuid.UUID) ([]uuid.UUID, error)
	findServicesByIDs(ctx context.Context, ids []uuid.UUID) ([]*serviceRef, error)
}

// ServiceOrderRepository is the persistence boundary for the ServiceOrder
// aggregate. It only has the one write this feature performs — no
// speculative CRUD methods for reads this feature doesn't need (design.md
// §3.2).
type ServiceOrderRepository interface {
	Create(ctx context.Context, order *ServiceOrder) error
}

// PostgresServiceOrderRepository implements ServiceOrderRepository and
// serviceOrderLookups against pgx.
type PostgresServiceOrderRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresServiceOrderRepository(pool *pgxpool.Pool) *PostgresServiceOrderRepository {
	return &PostgresServiceOrderRepository{pool: pool}
}

// Create persists order, its requested services, and its creation history
// event in a single transaction (RNF07, design.md §3.3): if any step fails,
// everything is rolled back — the API never leaves a service order without
// its creation history entry, or a requested service link without the order
// it belongs to.
func (repository *PostgresServiceOrderRepository) Create(ctx context.Context, order *ServiceOrder) error {
	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op once Commit succeeds

	if err := tx.QueryRow(ctx,
		`INSERT INTO service_orders (id, customer_id, vehicle_id, notes)
		 VALUES ($1, $2, $3, $4)
		 RETURNING code, opened_at, created_at, updated_at`,
		order.ID, order.CustomerID, order.VehicleID, order.Notes,
	).Scan(&order.Code, &order.OpenedAt, &order.CreatedAt, &order.UpdatedAt); err != nil {
		return err
	}

	for _, serviceID := range order.RequestedServiceIDs {
		if _, err := tx.Exec(ctx,
			`INSERT INTO service_order_requested_services (service_order_id, service_id) VALUES ($1, $2)`,
			order.ID, serviceID,
		); err != nil {
			return err
		}
	}

	if _, err := tx.Exec(ctx,
		`INSERT INTO service_order_history (service_order_id, event, description, previous_status, new_status)
		 VALUES ($1, 'creation', $2, $3, $3)`,
		order.ID, "Service order opened.", string(order.Status),
	); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (repository *PostgresServiceOrderRepository) findCustomerByID(ctx context.Context, id uuid.UUID) (*customerRef, error) {
	row := repository.pool.QueryRow(ctx,
		`SELECT id, code, name, status = 'ACTIVE' FROM customers WHERE id = $1`, id)
	return scanCustomerRef(row)
}

func (repository *PostgresServiceOrderRepository) findCustomerByDocument(ctx context.Context, normalizedDocument string) (*customerRef, error) {
	row := repository.pool.QueryRow(ctx,
		`SELECT id, code, name, status = 'ACTIVE' FROM customers WHERE document = $1`, normalizedDocument)
	return scanCustomerRef(row)
}

func scanCustomerRef(row pgx.Row) (*customerRef, error) {
	ref := &customerRef{}
	if err := row.Scan(&ref.ID, &ref.Code, &ref.Name, &ref.Active); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrCustomerNotFound
		}
		return nil, err
	}
	return ref, nil
}

func (repository *PostgresServiceOrderRepository) findVehicleByID(ctx context.Context, id uuid.UUID) (*vehicleRef, error) {
	row := repository.pool.QueryRow(ctx,
		`SELECT id, code, license_plate, customer_id, status = 'ACTIVE' FROM vehicles WHERE id = $1`, id)
	return scanVehicleRef(row)
}

func (repository *PostgresServiceOrderRepository) findVehicleByPlate(ctx context.Context, plate string) (*vehicleRef, error) {
	row := repository.pool.QueryRow(ctx,
		`SELECT id, code, license_plate, customer_id, status = 'ACTIVE' FROM vehicles WHERE license_plate = $1`, plate)
	return scanVehicleRef(row)
}

func scanVehicleRef(row pgx.Row) (*vehicleRef, error) {
	ref := &vehicleRef{}
	if err := row.Scan(&ref.ID, &ref.Code, &ref.LicensePlate, &ref.CustomerID, &ref.Active); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrVehicleNotFound
		}
		return nil, err
	}
	return ref, nil
}

// findMissingServiceIDs reports which of ids do not exist in the services
// catalog, so the caller can reject the request naming exactly what's
// missing (requirements.md §3.7).
func (repository *PostgresServiceOrderRepository) findMissingServiceIDs(ctx context.Context, ids []uuid.UUID) ([]uuid.UUID, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	rows, err := repository.pool.Query(ctx, `SELECT id FROM services WHERE id = ANY($1)`, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	found := make(map[uuid.UUID]bool, len(ids))
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		found[id] = true
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	var missing []uuid.UUID
	for _, id := range ids {
		if !found[id] {
			missing = append(missing, id)
		}
	}
	return missing, nil
}

// findServicesByIDs loads the display data (code, name) for a set of
// service ids, used to build the response after Create commits — a read,
// not a write, so it does not need to run inside Create's transaction
// (design.md §4.2).
func (repository *PostgresServiceOrderRepository) findServicesByIDs(ctx context.Context, ids []uuid.UUID) ([]*serviceRef, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	rows, err := repository.pool.Query(ctx,
		`SELECT id, code, name FROM services WHERE id = ANY($1)`, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var refs []*serviceRef
	for rows.Next() {
		ref := &serviceRef{}
		if err := rows.Scan(&ref.ID, &ref.Code, &ref.Name); err != nil {
			return nil, err
		}
		refs = append(refs, ref)
	}
	return refs, rows.Err()
}
