package serviceorder

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"automotive-workshop-api/internal/shared/trackingtoken"
)

// customerRef is the minimal customer data this feature needs to resolve
// and validate a customer, plus display it in the response. It is a
// package-local projection, deliberately not the customer feature's
// Customer type — importing internal/features/customer directly would
// violate the "no direct coupling between features" rule (CLAUDE.md §9.2).
type customerRef struct {
	ID       uuid.UUID
	Code     int64
	Name     string
	Active   bool
	Document string
	Phone    string
}

// vehicleRef is the vehicle-side equivalent of customerRef. Brand/Model/Year/
// Color are only needed by the detail view (specs/service-order-query/), left
// unused (but always populated — same query as everyone else, see
// scanVehicleRef) by the create flow.
type vehicleRef struct {
	ID           uuid.UUID
	Code         int64
	LicensePlate string
	CustomerID   uuid.UUID
	Active       bool
	Brand        string
	Model        string
	Year         int
	Color        string
}

// serviceRef is the minimal service-catalog data needed to display a
// requested service in the response. Description/Price are only populated
// by findServiceByID (used to price/snapshot a quote item,
// specs/service-order-diagnosis-quote/) — findServicesByIDs (used for the
// order-creation response) leaves them zero, since that response doesn't
// need them.
type serviceRef struct {
	ID          uuid.UUID
	Code        int64
	Name        string
	Description string
	Price       float64
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

	// Added by specs/service-order-diagnosis-quote/.
	findServiceOrderByID(ctx context.Context, id uuid.UUID) (*ServiceOrder, error)
	findActiveProductByID(ctx context.Context, id uuid.UUID) (*productRef, error)
	findServiceByID(ctx context.Context, id uuid.UUID) (*serviceRef, error)

	// Added by specs/service-order-query/.
	findServiceOrderByCode(ctx context.Context, code int64) (*ServiceOrder, error)
	findRequestedServices(ctx context.Context, serviceOrderID uuid.UUID) ([]*serviceRef, error)
	findHistoryByServiceOrderID(ctx context.Context, serviceOrderID uuid.UUID) ([]*ServiceOrderHistory, error)
	listServiceOrders(ctx context.Context, filter ListFilter, page, pageSize int) ([]*ServiceOrderListItem, int, error)

	// Added by specs/service-order-execution/.
	findServiceExecutionByID(ctx context.Context, serviceOrderID, executionID uuid.UUID) (*ServiceExecution, error)
	findServiceExecutionsByServiceOrderID(ctx context.Context, serviceOrderID uuid.UUID) ([]*ServiceExecution, error)

	// Added by specs/service-order-metrics/.
	findAverageExecutionTimeByService(ctx context.Context, filter MetricsFilter) ([]*ServiceMetric, error)

	// Added by specs/service-order-quote-decision/: resolves the order
	// identified by its public code first (so an unknown code always maps to
	// ErrServiceOrderNotFound regardless of the token), then checks
	// tokenHash against that specific order's active tracking token
	// (ErrTrackingTokenInvalid otherwise) — same precedence as
	// service-order-tracking's FindByCodeAndTokenHash, reading
	// service_order_tracking_tokens directly via SQL rather than importing
	// that feature's Go package (CLAUDE.md §9.2).
	findServiceOrderByCodeWithTrackingToken(ctx context.Context, code int64, tokenHash string) (*ServiceOrder, error)
}

// ServiceOrderRepository is the persistence boundary for the ServiceOrder
// aggregate. It only has the writes this feature performs — no speculative
// CRUD methods for reads this feature doesn't need (design.md §3.2).
type ServiceOrderRepository interface {
	// Create persists order and returns the raw tracking token generated for
	// it (specs/service-order-tracking/design.md §5) — only that call ever
	// sees the raw value; the database only ever stores its hash.
	Create(ctx context.Context, order *ServiceOrder) (trackingToken string, err error)

	// Added by specs/service-order-diagnosis-quote/.
	StartDiagnosis(ctx context.Context, order *ServiceOrder) error
	SaveQuote(ctx context.Context, order *ServiceOrder, items []QuoteItem, total float64) (*Quote, error)
	FindQuoteByServiceOrderID(ctx context.Context, serviceOrderID uuid.UUID) (*Quote, error)

	// Added by specs/service-order-execution/.
	StartExecution(ctx context.Context, execution *ServiceExecution) error
	FinishExecution(ctx context.Context, execution *ServiceExecution) error
	FinalizeOrder(ctx context.Context, order *ServiceOrder) error
	DeliverOrder(ctx context.Context, order *ServiceOrder) error

	// Added by specs/service-order-quote-decision/.

	// SendQuote records quote as sent (sent_at/sent_version) and transitions
	// order from EM_DIAGNOSTICO to AGUARDANDO_APROVACAO, transactionally.
	SendQuote(ctx context.Context, order *ServiceOrder, quote *Quote) (*Quote, error)

	// DecideQuote records the customer's decision on quote (APPROVED or
	// REJECTED, per decision), transitions order accordingly (EM_EXECUCAO or
	// CANCELADA), and writes the corresponding history entry, all in one
	// transaction (RNF07): a failure at any step, including the history
	// insert, rolls back every other write this call made.
	DecideQuote(ctx context.Context, order *ServiceOrder, quote *Quote, decision QuoteStatus) (*Quote, error)
}

// PostgresServiceOrderRepository implements ServiceOrderRepository and
// serviceOrderLookups against pgx.
type PostgresServiceOrderRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresServiceOrderRepository(pool *pgxpool.Pool) *PostgresServiceOrderRepository {
	return &PostgresServiceOrderRepository{pool: pool}
}

// Create persists order, its requested services, its creation history event,
// and its auto-generated tracking token in a single transaction (RNF07,
// design.md §3.3; specs/service-order-tracking/design.md §5): if any step
// fails, everything is rolled back — the API never leaves a service order
// without its creation history entry or its tracking token, or a requested
// service link without the order it belongs to.
func (repository *PostgresServiceOrderRepository) Create(ctx context.Context, order *ServiceOrder) (string, error) {
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

func (repository *PostgresServiceOrderRepository) findCustomerByID(ctx context.Context, id uuid.UUID) (*customerRef, error) {
	row := repository.pool.QueryRow(ctx,
		`SELECT id, code, name, status = 'ACTIVE', document, phone FROM customers WHERE id = $1`, id)
	return scanCustomerRef(row)
}

func (repository *PostgresServiceOrderRepository) findCustomerByDocument(ctx context.Context, normalizedDocument string) (*customerRef, error) {
	row := repository.pool.QueryRow(ctx,
		`SELECT id, code, name, status = 'ACTIVE', document, phone FROM customers WHERE document = $1`, normalizedDocument)
	return scanCustomerRef(row)
}

func scanCustomerRef(row pgx.Row) (*customerRef, error) {
	ref := &customerRef{}
	if err := row.Scan(&ref.ID, &ref.Code, &ref.Name, &ref.Active, &ref.Document, &ref.Phone); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrCustomerNotFound
		}
		return nil, err
	}
	return ref, nil
}

func (repository *PostgresServiceOrderRepository) findVehicleByID(ctx context.Context, id uuid.UUID) (*vehicleRef, error) {
	row := repository.pool.QueryRow(ctx,
		`SELECT id, code, license_plate, customer_id, status = 'ACTIVE', brand, model, year, color FROM vehicles WHERE id = $1`, id)
	return scanVehicleRef(row)
}

func (repository *PostgresServiceOrderRepository) findVehicleByPlate(ctx context.Context, plate string) (*vehicleRef, error) {
	row := repository.pool.QueryRow(ctx,
		`SELECT id, code, license_plate, customer_id, status = 'ACTIVE', brand, model, year, color FROM vehicles WHERE license_plate = $1`, plate)
	return scanVehicleRef(row)
}

func scanVehicleRef(row pgx.Row) (*vehicleRef, error) {
	ref := &vehicleRef{}
	if err := row.Scan(&ref.ID, &ref.Code, &ref.LicensePlate, &ref.CustomerID, &ref.Active,
		&ref.Brand, &ref.Model, &ref.Year, &ref.Color); err != nil {
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

// findServiceOrderByCodeWithTrackingToken implements serviceOrderLookups
// (specs/service-order-quote-decision/design.md): the order is resolved by
// code first, so an unknown code always maps to ErrServiceOrderNotFound
// regardless of the token; only then is tokenHash checked against that
// order's active tracking token, mirroring
// service-order-tracking.PostgresTrackingRepository.FindByCodeAndTokenHash.
func (repository *PostgresServiceOrderRepository) findServiceOrderByCodeWithTrackingToken(ctx context.Context, code int64, tokenHash string) (*ServiceOrder, error) {
	order := &ServiceOrder{}
	err := repository.pool.QueryRow(ctx,
		`SELECT id, code, customer_id, vehicle_id, opened_at, status, notes, created_at, updated_at
		 FROM service_orders WHERE code = $1`, code,
	).Scan(&order.ID, &order.Code, &order.CustomerID, &order.VehicleID, &order.OpenedAt,
		&order.Status, &order.Notes, &order.CreatedAt, &order.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrServiceOrderNotFound
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
		return nil, ErrTrackingTokenInvalid
	}

	return order, nil
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
