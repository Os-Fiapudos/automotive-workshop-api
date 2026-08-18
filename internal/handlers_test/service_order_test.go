package handlers_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"automotive-workshop-api/internal/features/customer"
	serviceorder "automotive-workshop-api/internal/features/service-order"
)

// testServiceOrderServer builds the real customer + service-order HTTP
// handlers wired to a real pgxpool.Pool (customer creation is needed to set
// up fixtures for these tests, since there is no other endpoint that
// creates a customer). Skips the calling test when the database is
// unreachable, same convention as testServer in customer_test.go.
func testServiceOrderServer(t *testing.T) (*pgxpool.Pool, string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, testDatabaseURL())
	if err != nil {
		t.Skipf("skipping integration test: cannot create pool: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Skipf("skipping integration test: database unreachable (start it with `docker compose up -d db`): %v", err)
	}
	t.Cleanup(pool.Close)

	customerRepository := customer.NewPostgresCustomerRepository(pool)
	customerService := customer.NewCustomerService(customerRepository)

	serviceOrderRepository := serviceorder.NewPostgresServiceOrderRepository(pool)
	serviceOrderService := serviceorder.NewServiceOrderService(serviceOrderRepository, serviceOrderRepository)

	router := http.NewServeMux()
	customer.RegisterRoutes(router, customerService)
	serviceorder.RegisterRoutes(router, serviceOrderService)

	server := httptest.NewServer(router)
	t.Cleanup(server.Close)

	return pool, server.URL
}

// insertActiveCustomer creates a customer through the real API (exercising
// the same path production traffic uses) and registers its cleanup.
func insertActiveCustomer(t *testing.T, server string, pool *pgxpool.Pool) customer.Response {
	t.Helper()

	resp := doJSON(t, http.MethodPost, server+"/api/v1/customers", customer.CreateRequest{
		Name:     "Maria Silva",
		Document: randomValidCPF(),
		Phone:    "+55 11 91234-5678",
	})
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	var created customer.Response
	decodeBody(t, resp, &created)
	cleanupCustomer(t, pool, created.ID)
	return created
}

// insertVehicle inserts a vehicle directly via SQL, the same fixture-setup
// approach docs/seed.sql uses. specs/service-order-opening/requirements.md
// §7.2 originally justified this by "there is no vehicle management
// endpoint yet" — Vehicle Management has since been implemented
// (internal/features/vehicle/), but direct SQL insertion is kept here
// anyway: what these tests exercise is Service Order Opening's own
// validation, not vehicle creation, so going through the real
// POST /api/v1/vehicles endpoint would only add an unrelated dependency
// (and its own JWT requirement) to this fixture setup. plate must be a
// structurally valid, unique license plate — callers use randomValidPlate()
// (vehicle_test.go) so fixtures never collide with docs/seed.sql's sample
// vehicles or with each other.
func insertVehicle(t *testing.T, pool *pgxpool.Pool, customerID, plate string, active bool) string {
	t.Helper()

	status := "ACTIVE"
	if !active {
		status = "INACTIVE"
	}

	var id string
	err := pool.QueryRow(context.Background(),
		`INSERT INTO vehicles (license_plate, brand, model, year, color, customer_id, status)
		 VALUES ($1, 'Fiat', 'Uno', 2020, 'White', $2, $3)
		 RETURNING id`,
		plate, customerID, status,
	).Scan(&id)
	require.NoError(t, err)

	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM vehicles WHERE id = $1`, id)
	})
	return id
}

// insertService inserts a catalog service directly via SQL.
func insertService(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()

	var id string
	err := pool.QueryRow(context.Background(),
		`INSERT INTO services (name, description, price) VALUES ('Oil Change', 'Engine oil change.', 80.00) RETURNING id`,
	).Scan(&id)
	require.NoError(t, err)

	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM services WHERE id = $1`, id)
	})
	return id
}

func cleanupServiceOrder(t *testing.T, pool *pgxpool.Pool, id string) {
	t.Helper()
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM service_orders WHERE id = $1`, id)
	})
}

func countServiceOrdersForVehicle(t *testing.T, pool *pgxpool.Pool, vehicleID string) int {
	t.Helper()
	var count int
	err := pool.QueryRow(context.Background(), `SELECT COUNT(*) FROM service_orders WHERE vehicle_id = $1`, vehicleID).Scan(&count)
	require.NoError(t, err)
	return count
}

func TestServiceOrderCreateSuccess(t *testing.T) {
	pool, server := testServiceOrderServer(t)

	createdCustomer := insertActiveCustomer(t, server, pool)
	vehicleID := insertVehicle(t, pool, createdCustomer.ID, randomValidPlate(), true)
	serviceID := insertService(t, pool)

	resp := doJSON(t, http.MethodPost, server+"/api/v1/service-orders", serviceorder.CreateRequest{
		CustomerID:          createdCustomer.ID,
		VehicleID:           vehicleID,
		RequestedServiceIDs: []string{serviceID},
		Notes:               "Customer reported a light engine noise.",
	})
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var created serviceorder.Response
	decodeBody(t, resp, &created)
	cleanupServiceOrder(t, pool, created.ID)

	assert.Equal(t, "RECEBIDA", created.Status)
	assert.NotZero(t, created.Code)
	assert.Equal(t, createdCustomer.ID, created.Customer.ID)
	assert.Equal(t, vehicleID, created.Vehicle.ID)
	require.Len(t, created.RequestedServices, 1)
	assert.Equal(t, serviceID, created.RequestedServices[0].ID)

	// A creation history event must exist (RNF07 / requirements.md §3.9).
	var event, previousStatus, newStatus string
	err := pool.QueryRow(context.Background(),
		`SELECT event, previous_status, new_status FROM service_order_history WHERE service_order_id = $1`,
		created.ID,
	).Scan(&event, &previousStatus, &newStatus)
	require.NoError(t, err)
	assert.Equal(t, "creation", event)
	assert.Equal(t, "RECEBIDA", previousStatus)
	assert.Equal(t, "RECEBIDA", newStatus)
}

func TestServiceOrderCreateByDocumentAndPlate(t *testing.T) {
	pool, server := testServiceOrderServer(t)

	createdCustomer := insertActiveCustomer(t, server, pool)
	plate := randomValidPlate()
	vehicleID := insertVehicle(t, pool, createdCustomer.ID, plate, true)

	resp := doJSON(t, http.MethodPost, server+"/api/v1/service-orders", serviceorder.CreateRequest{
		CustomerDocument: createdCustomer.Document,
		LicensePlate:     plate,
	})
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var created serviceorder.Response
	decodeBody(t, resp, &created)
	cleanupServiceOrder(t, pool, created.ID)
	assert.Equal(t, vehicleID, created.Vehicle.ID)
}

func TestServiceOrderCreateRejectsInactiveCustomer(t *testing.T) {
	pool, server := testServiceOrderServer(t)

	createdCustomer := insertActiveCustomer(t, server, pool)
	vehicleID := insertVehicle(t, pool, createdCustomer.ID, randomValidPlate(), true)

	deactivateResp := doJSON(t, http.MethodDelete, server+"/api/v1/customers/"+createdCustomer.ID, nil)
	require.Equal(t, http.StatusOK, deactivateResp.StatusCode)

	resp := doJSON(t, http.MethodPost, server+"/api/v1/service-orders", serviceorder.CreateRequest{
		CustomerID: createdCustomer.ID,
		VehicleID:  vehicleID,
	})
	assert.Equal(t, http.StatusConflict, resp.StatusCode)
}

func TestServiceOrderCreateRejectsInactiveVehicle(t *testing.T) {
	pool, server := testServiceOrderServer(t)

	createdCustomer := insertActiveCustomer(t, server, pool)
	vehicleID := insertVehicle(t, pool, createdCustomer.ID, randomValidPlate(), false)

	resp := doJSON(t, http.MethodPost, server+"/api/v1/service-orders", serviceorder.CreateRequest{
		CustomerID: createdCustomer.ID,
		VehicleID:  vehicleID,
	})
	assert.Equal(t, http.StatusConflict, resp.StatusCode)
}

func TestServiceOrderCreateRejectsVehicleFromAnotherCustomer(t *testing.T) {
	pool, server := testServiceOrderServer(t)

	firstCustomer := insertActiveCustomer(t, server, pool)
	secondCustomer := insertActiveCustomer(t, server, pool)
	vehicleID := insertVehicle(t, pool, secondCustomer.ID, randomValidPlate(), true)

	resp := doJSON(t, http.MethodPost, server+"/api/v1/service-orders", serviceorder.CreateRequest{
		CustomerID: firstCustomer.ID,
		VehicleID:  vehicleID,
	})
	assert.Equal(t, http.StatusConflict, resp.StatusCode)
}

func TestServiceOrderCreateRejectsUnknownRequestedService(t *testing.T) {
	pool, server := testServiceOrderServer(t)

	createdCustomer := insertActiveCustomer(t, server, pool)
	vehicleID := insertVehicle(t, pool, createdCustomer.ID, randomValidPlate(), true)

	resp := doJSON(t, http.MethodPost, server+"/api/v1/service-orders", serviceorder.CreateRequest{
		CustomerID:          createdCustomer.ID,
		VehicleID:           vehicleID,
		RequestedServiceIDs: []string{"00000000-0000-0000-0000-000000000000"},
	})
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

// TestServiceOrderCreateRollsBackOnPartialFailure exercises RNF07: a
// duplicate id in requestedServiceIds passes the pre-check (both occurrences
// exist) but the second insert into service_order_requested_services
// violates its primary key mid-transaction, after the service_orders row
// would otherwise already have been inserted — see design.md §3.4. The
// whole creation must roll back, leaving no orphan service_orders row.
func TestServiceOrderCreateRollsBackOnPartialFailure(t *testing.T) {
	pool, server := testServiceOrderServer(t)

	createdCustomer := insertActiveCustomer(t, server, pool)
	vehicleID := insertVehicle(t, pool, createdCustomer.ID, randomValidPlate(), true)
	serviceID := insertService(t, pool)

	before := countServiceOrdersForVehicle(t, pool, vehicleID)

	resp := doJSON(t, http.MethodPost, server+"/api/v1/service-orders", serviceorder.CreateRequest{
		CustomerID:          createdCustomer.ID,
		VehicleID:           vehicleID,
		RequestedServiceIDs: []string{serviceID, serviceID},
	})
	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)

	after := countServiceOrdersForVehicle(t, pool, vehicleID)
	assert.Equal(t, before, after, "a failed creation must not leave an orphan service_orders row")
}
