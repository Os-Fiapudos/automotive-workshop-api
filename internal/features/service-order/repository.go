package serviceorder

import (
	"automotive-workshop-api/internal/features/service-order/domain"
	"automotive-workshop-api/internal/features/service-order/usecase"
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"automotive-workshop-api/internal/shared/trackingtoken"
)

// PostgresServiceOrderRepository implements ServiceOrderRepository and
// serviceOrderLookups against pgx.
type PostgresServiceOrderRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresServiceOrderRepository(pool *pgxpool.Pool) *PostgresServiceOrderRepository {
	return &PostgresServiceOrderRepository{pool: pool}
}

// Create persists order, its requested services and products, its creation
// history event,
// and its auto-generated tracking token in a single transaction (RNF07,
// design.md §3.3; specs/service-order-tracking/design.md §5): if any step
// fails, everything is rolled back — the API never leaves a service order
// without its creation history entry or its tracking token, or a requested
// service link without the order it belongs to.
func (repository *PostgresServiceOrderRepository) Create(ctx context.Context, order *domain.ServiceOrder) (string, error) {
	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op once Commit succeeds

	if err := tx.QueryRow(ctx,
		`INSERT INTO service_orders (id, customer_id, vehicle_id, notes)
		 VALUES ($1, $2, $3, $4)
		 RETURNING code, opened_at, created_at, updated_at`,
		order.ID, order.CustomerID, order.VehicleID, order.Notes,
	).Scan(&order.Code, &order.OpenedAt, &order.CreatedAt, &order.UpdatedAt); err != nil {
		return "", err
	}

	for _, serviceID := range order.RequestedServiceIDs {
		if _, err := tx.Exec(ctx,
			`INSERT INTO service_order_requested_services (service_order_id, service_id) VALUES ($1, $2)`,
			order.ID, serviceID,
		); err != nil {
			return "", err
		}
	}

	// Requested products are recorded as demand only: products.current_stock
	// is never read or written here (requirements.md §7a.1, design.md §7.3).
	for _, product := range order.RequestedProducts {
		if _, err := tx.Exec(ctx,
			`INSERT INTO service_order_requested_products (service_order_id, product_id, quantity) VALUES ($1, $2, $3)`,
			order.ID, product.ProductID, product.Quantity,
		); err != nil {
			return "", err
		}
	}

	if _, err := tx.Exec(ctx,
		`INSERT INTO service_order_history (service_order_id, event, description, previous_status, new_status)
		 VALUES ($1, 'creation', $2, $3, $3)`,
		order.ID, "Service order opened.", string(order.Status),
	); err != nil {
		return "", err
	}

	rawToken, err := trackingtoken.Generate()
	if err != nil {
		return "", err
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO service_order_tracking_tokens (service_order_id, token_hash) VALUES ($1, $2)`,
		order.ID, trackingtoken.Hash(rawToken),
	); err != nil {
		return "", err
	}

	if err := tx.Commit(ctx); err != nil {
		return "", err
	}
	return rawToken, nil
}

func (repository *PostgresServiceOrderRepository) FindCustomerByID(ctx context.Context, id uuid.UUID) (*usecase.CustomerRef, error) {
	row := repository.pool.QueryRow(ctx,
		`SELECT id, code, name, status = 'ACTIVE', document, phone, email FROM customers WHERE id = $1`, id)
	return scanCustomerRef(row)
}

func (repository *PostgresServiceOrderRepository) FindCustomerByDocument(ctx context.Context, normalizedDocument string) (*usecase.CustomerRef, error) {
	row := repository.pool.QueryRow(ctx,
		`SELECT id, code, name, status = 'ACTIVE', document, phone, email FROM customers WHERE document = $1`, normalizedDocument)
	return scanCustomerRef(row)
}

func scanCustomerRef(row pgx.Row) (*usecase.CustomerRef, error) {
	ref := &usecase.CustomerRef{}
	if err := row.Scan(&ref.ID, &ref.Code, &ref.Name, &ref.Active, &ref.Document, &ref.Phone, &ref.Email); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrCustomerNotFound
		}
		return nil, err
	}
	return ref, nil
}

func (repository *PostgresServiceOrderRepository) FindVehicleByID(ctx context.Context, id uuid.UUID) (*usecase.VehicleRef, error) {
	row := repository.pool.QueryRow(ctx,
		`SELECT id, code, license_plate, customer_id, status = 'ACTIVE', brand, model, year, color FROM vehicles WHERE id = $1`, id)
	return scanVehicleRef(row)
}

func (repository *PostgresServiceOrderRepository) FindVehicleByPlate(ctx context.Context, plate string) (*usecase.VehicleRef, error) {
	row := repository.pool.QueryRow(ctx,
		`SELECT id, code, license_plate, customer_id, status = 'ACTIVE', brand, model, year, color FROM vehicles WHERE license_plate = $1`, plate)
	return scanVehicleRef(row)
}

func scanVehicleRef(row pgx.Row) (*usecase.VehicleRef, error) {
	ref := &usecase.VehicleRef{}
	if err := row.Scan(&ref.ID, &ref.Code, &ref.LicensePlate, &ref.CustomerID, &ref.Active,
		&ref.Brand, &ref.Model, &ref.Year, &ref.Color); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrVehicleNotFound
		}
		return nil, err
	}
	return ref, nil
}

// findMissingServiceIDs reports which of ids do not exist in the services
// catalog, so the caller can reject the request naming exactly what's
// missing (requirements.md §3.7).
func (repository *PostgresServiceOrderRepository) FindMissingServiceIDs(ctx context.Context, ids []uuid.UUID) ([]uuid.UUID, error) {
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

// findServiceOrderByCodeWithTrackingToken implements serviceOrderLookups
// (specs/service-order-quote-decision/design.md): the order is resolved by
// code first, so an unknown code always maps to ErrServiceOrderNotFound
// regardless of the token; only then is tokenHash checked against that
// order's active tracking token, mirroring
// service-order-tracking.PostgresTrackingRepository.FindByCodeAndTokenHash.
func (repository *PostgresServiceOrderRepository) FindServiceOrderByCodeWithTrackingToken(ctx context.Context, code int64, tokenHash string) (*domain.ServiceOrder, error) {
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

	var matches bool
	if err := repository.pool.QueryRow(ctx,
		`SELECT EXISTS (
			SELECT 1 FROM service_order_tracking_tokens
			WHERE service_order_id = $1 AND token_hash = $2 AND revoked_at IS NULL
		)`, order.ID, tokenHash,
	).Scan(&matches); err != nil {
		return nil, err
	}
	if !matches {
		return nil, domain.ErrTrackingTokenInvalid
	}

	return order, nil
}

// findServicesByIDs loads the display data (code, name) for a set of
// service ids, used to build the response after Create commits — a read,
// not a write, so it does not need to run inside Create's transaction
// (design.md §4.2).
func (repository *PostgresServiceOrderRepository) FindServicesByIDs(ctx context.Context, ids []uuid.UUID) ([]*usecase.ServiceRef, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	rows, err := repository.pool.Query(ctx,
		`SELECT id, code, name FROM services WHERE id = ANY($1)`, ids)
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
