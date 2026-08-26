package handlers_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"automotive-workshop-api/internal/features/auth"
	"automotive-workshop-api/internal/features/customer"
	serviceorder "automotive-workshop-api/internal/features/service-order"
	"automotive-workshop-api/internal/shared/middleware"
	"automotive-workshop-api/internal/shared/token"
)

// testServiceOrderServer builds the real auth + customer + service-order
// HTTP handlers wired to a real pgxpool.Pool (customer creation is needed to
// set up fixtures for these tests, since there is no other endpoint that
// creates a customer; auth is needed because the diagnosis/quote routes
// added by specs/service-order-diagnosis-quote/ require it). Skips the
// calling test when the database is unreachable, same convention as
// testServer in customer_test.go. Returns a valid bearer token alongside the
// pool/server URL, same pattern as testProductServer in product_test.go.
func testServiceOrderServer(t *testing.T) (*pgxpool.Pool, string, string) {
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

	tokens := token.NewManager("integration-test-secret", time.Hour)
	requireAuth := middleware.RequireAuth(tokens)

	authHandler := auth.NewHandler(auth.NewService(auth.NewRepository(pool), tokens))

	customerRepository := customer.NewPostgresCustomerRepository(pool)
	customerService := customer.NewCustomerService(customerRepository)

	serviceOrderRepository := serviceorder.NewPostgresServiceOrderRepository(pool)
	serviceOrderService := serviceorder.NewServiceOrderService(serviceOrderRepository, serviceOrderRepository, nil)

	router := http.NewServeMux()
	router.HandleFunc("POST /api/v1/auth/login", authHandler.Login)
	customer.RegisterRoutes(router, customerService, requireAuth)
	serviceorder.RegisterRoutes(router, serviceOrderService, requireAuth)

	server := httptest.NewServer(router)
	t.Cleanup(server.Close)

	loginResp := doAuthJSON(t, http.MethodPost, server.URL+"/api/v1/auth/login", map[string]string{
		"email":    "admin@workshop.local",
		"password": "admin123",
	}, "")
	require.Equal(t, http.StatusOK, loginResp.StatusCode)
	var authBody struct {
		AccessToken string `json:"access_token"`
	}
	decodeBody(t, loginResp, &authBody)
	require.NotEmpty(t, authBody.AccessToken)

	return pool, server.URL, authBody.AccessToken
}

// loginAsAdmin logs in as the seeded administrative user and returns a
// bearer token. Used internally by fixture helpers (insertActiveCustomer,
// createServiceOrder) so their signatures — and their dozens of call sites
// across this package — don't need to change now that customer creation and
// service-order creation both require auth
// (docs/owasp-vulnerability-and-coverage-report.md VULN-01/VULN-02).
func loginAsAdmin(t *testing.T, server string) string {
	t.Helper()

	resp := doAuthJSON(t, http.MethodPost, server+"/api/v1/auth/login", map[string]string{
		"email":    "admin@workshop.local",
		"password": "admin123",
	}, "")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var body struct {
		AccessToken string `json:"access_token"`
	}
	decodeBody(t, resp, &body)
	require.NotEmpty(t, body.AccessToken)
	return body.AccessToken
}

// insertActiveCustomer creates a customer through the real API (exercising
// the same path production traffic uses) and registers its cleanup.
func insertActiveCustomer(t *testing.T, server string, pool *pgxpool.Pool) customer.Response {
	t.Helper()

	resp := doAuthJSON(t, http.MethodPost, server+"/api/v1/customers", customer.CreateRequest{
		Name:     "Maria Silva",
		Document: randomValidCPF(),
		Phone:    "+55 11 91234-5678",
	}, loginAsAdmin(t, server))
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	var created customer.Response
	decodeBody(t, resp, &created)
	cleanupCustomer(t, pool, created.ID)
	return created
}

// insertVehicle inserts a vehicle directly via SQL — there is no vehicle
// management endpoint yet (specs/service-order-opening/requirements.md
// §7.2), so tests set up this fixture the same way docs/seed.sql does.
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

// insertProduct inserts a catalog product directly via SQL, used to test
// quote composition (specs/service-order-diagnosis-quote/).
func insertProduct(t *testing.T, pool *pgxpool.Pool, unitPrice float64, active bool) string {
	t.Helper()

	status := "ACTIVE"
	if !active {
		status = "INACTIVE"
	}

	var id string
	err := pool.QueryRow(context.Background(),
		`INSERT INTO products (name, description, unit_price, current_stock, type, status)
		 VALUES ('Oil Filter', 'Engine oil filter, general use.', $1, 10, 'PART', $2)
		 RETURNING id`,
		unitPrice, status,
	).Scan(&id)
	require.NoError(t, err)

	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM products WHERE id = $1`, id)
	})
	return id
}

func productStock(t *testing.T, pool *pgxpool.Pool, id string) int {
	t.Helper()
	var stock int
	err := pool.QueryRow(context.Background(), `SELECT current_stock FROM products WHERE id = $1`, id).Scan(&stock)
	require.NoError(t, err)
	return stock
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
	pool, server, authToken := testServiceOrderServer(t)

	createdCustomer := insertActiveCustomer(t, server, pool)
	vehicleID := insertVehicle(t, pool, createdCustomer.ID, randomLicensePlate(), true)
	serviceID := insertService(t, pool)

	resp := doAuthJSON(t, http.MethodPost, server+"/api/v1/service-orders", serviceorder.CreateRequest{
		CustomerID:          createdCustomer.ID,
		VehicleID:           vehicleID,
		RequestedServiceIDs: []string{serviceID},
		Notes:               "Customer reported a light engine noise.",
	}, authToken)
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

// TestServiceOrderCreateRequiresAuth guards against VULN-02
// (docs/owasp-vulnerability-and-coverage-report.md): POST
// /api/v1/service-orders used to accept a customer document/plate as an
// identifier with no credential at all, letting an unauthenticated caller
// confirm a CPF/CNPJ belonged to a registered customer and open orders in
// their name. It must now reject a request with no bearer token, same as
// every other route in this feature.
func TestServiceOrderCreateRequiresAuth(t *testing.T) {
	pool, server, _ := testServiceOrderServer(t)

	createdCustomer := insertActiveCustomer(t, server, pool)
	vehicleID := insertVehicle(t, pool, createdCustomer.ID, randomLicensePlate(), true)

	resp := doAuthJSON(t, http.MethodPost, server+"/api/v1/service-orders", serviceorder.CreateRequest{
		CustomerID: createdCustomer.ID,
		VehicleID:  vehicleID,
	}, "")
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestServiceOrderCreateByDocumentAndPlate(t *testing.T) {
	pool, server, authToken := testServiceOrderServer(t)

	plate := randomLicensePlate()
	createdCustomer := insertActiveCustomer(t, server, pool)
	vehicleID := insertVehicle(t, pool, createdCustomer.ID, plate, true)

	resp := doAuthJSON(t, http.MethodPost, server+"/api/v1/service-orders", serviceorder.CreateRequest{
		CustomerDocument: createdCustomer.Document,
		LicensePlate:     plate,
	}, authToken)
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var created serviceorder.Response
	decodeBody(t, resp, &created)
	cleanupServiceOrder(t, pool, created.ID)
	assert.Equal(t, vehicleID, created.Vehicle.ID)
}

func TestServiceOrderCreateRejectsInactiveCustomer(t *testing.T) {
	pool, server, authToken := testServiceOrderServer(t)

	createdCustomer := insertActiveCustomer(t, server, pool)
	vehicleID := insertVehicle(t, pool, createdCustomer.ID, randomLicensePlate(), true)

	deactivateResp := doAuthJSON(t, http.MethodDelete, server+"/api/v1/customers/"+createdCustomer.ID, nil, authToken)
	require.Equal(t, http.StatusOK, deactivateResp.StatusCode)

	resp := doAuthJSON(t, http.MethodPost, server+"/api/v1/service-orders", serviceorder.CreateRequest{
		CustomerID: createdCustomer.ID,
		VehicleID:  vehicleID,
	}, authToken)
	assert.Equal(t, http.StatusConflict, resp.StatusCode)
}

func TestServiceOrderCreateRejectsInactiveVehicle(t *testing.T) {
	pool, server, authToken := testServiceOrderServer(t)

	createdCustomer := insertActiveCustomer(t, server, pool)
	vehicleID := insertVehicle(t, pool, createdCustomer.ID, randomLicensePlate(), false)

	resp := doAuthJSON(t, http.MethodPost, server+"/api/v1/service-orders", serviceorder.CreateRequest{
		CustomerID: createdCustomer.ID,
		VehicleID:  vehicleID,
	}, authToken)
	assert.Equal(t, http.StatusConflict, resp.StatusCode)
}

func TestServiceOrderCreateRejectsVehicleFromAnotherCustomer(t *testing.T) {
	pool, server, authToken := testServiceOrderServer(t)

	firstCustomer := insertActiveCustomer(t, server, pool)
	secondCustomer := insertActiveCustomer(t, server, pool)
	vehicleID := insertVehicle(t, pool, secondCustomer.ID, randomLicensePlate(), true)

	resp := doAuthJSON(t, http.MethodPost, server+"/api/v1/service-orders", serviceorder.CreateRequest{
		CustomerID: firstCustomer.ID,
		VehicleID:  vehicleID,
	}, authToken)
	assert.Equal(t, http.StatusConflict, resp.StatusCode)
}

func TestServiceOrderCreateRejectsUnknownRequestedService(t *testing.T) {
	pool, server, authToken := testServiceOrderServer(t)

	createdCustomer := insertActiveCustomer(t, server, pool)
	vehicleID := insertVehicle(t, pool, createdCustomer.ID, "PQR1S23", true)

	resp := doAuthJSON(t, http.MethodPost, server+"/api/v1/service-orders", serviceorder.CreateRequest{
		CustomerID:          createdCustomer.ID,
		VehicleID:           vehicleID,
		RequestedServiceIDs: []string{"00000000-0000-0000-0000-000000000000"},
	}, authToken)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

// TestServiceOrderCreateRollsBackOnPartialFailure exercises RNF07: a
// duplicate id in requestedServiceIds passes the pre-check (both occurrences
// exist) but the second insert into service_order_requested_services
// violates its primary key mid-transaction, after the service_orders row
// would otherwise already have been inserted — see design.md §3.4. The
// whole creation must roll back, leaving no orphan service_orders row.
func TestServiceOrderCreateRollsBackOnPartialFailure(t *testing.T) {
	pool, server, authToken := testServiceOrderServer(t)

	createdCustomer := insertActiveCustomer(t, server, pool)
	vehicleID := insertVehicle(t, pool, createdCustomer.ID, "TUV4W56", true)
	serviceID := insertService(t, pool)

	before := countServiceOrdersForVehicle(t, pool, vehicleID)

	resp := doAuthJSON(t, http.MethodPost, server+"/api/v1/service-orders", serviceorder.CreateRequest{
		CustomerID:          createdCustomer.ID,
		VehicleID:           vehicleID,
		RequestedServiceIDs: []string{serviceID, serviceID},
	}, authToken)
	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)

	after := countServiceOrdersForVehicle(t, pool, vehicleID)
	assert.Equal(t, before, after, "a failed creation must not leave an orphan service_orders row")
}

// createServiceOrder creates a service order through the real API and
// returns its full response, for use as a fixture by the diagnosis/quote
// tests below.
func createServiceOrder(t *testing.T, server string, pool *pgxpool.Pool) serviceorder.Response {
	t.Helper()

	createdCustomer := insertActiveCustomer(t, server, pool)
	vehicleID := insertVehicle(t, pool, createdCustomer.ID, randomLicensePlate(), true)

	resp := doAuthJSON(t, http.MethodPost, server+"/api/v1/service-orders", serviceorder.CreateRequest{
		CustomerID: createdCustomer.ID,
		VehicleID:  vehicleID,
	}, loginAsAdmin(t, server))
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var created serviceorder.Response
	decodeBody(t, resp, &created)
	cleanupServiceOrder(t, pool, created.ID)
	return created
}

var licensePlateCounter int

// randomLicensePlate returns a plate unique for this test binary run.
// licensePlateCounter is a plain package-level counter (tests in this
// package never run with t.Parallel), formatted with enough digits
// (10000 distinct values) that the growing number of tests calling this
// helper — this file alone now has 40+ — never wraps around and collides
// with an earlier, still-cleaned-up-later vehicle within the same run (an
// earlier 10*26=260-combination format did collide once the call count grew
// past that, specs/service-order-stock-usage/'s new tests included).
func randomLicensePlate() string {
	licensePlateCounter++
	return fmt.Sprintf("QZX%04dY", licensePlateCounter%10000)
}

func TestServiceOrderStartDiagnosisSuccess(t *testing.T) {
	pool, server, authToken := testServiceOrderServer(t)
	order := createServiceOrder(t, server, pool)

	resp := doAuthJSON(t, http.MethodPost, server+"/api/v1/service-orders/"+order.ID+"/diagnosis", nil, authToken)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var status string
	err := pool.QueryRow(context.Background(), `SELECT status FROM service_orders WHERE id = $1`, order.ID).Scan(&status)
	require.NoError(t, err)
	assert.Equal(t, "EM_DIAGNOSTICO", status)

	var event, previousStatus, newStatus string
	err = pool.QueryRow(context.Background(),
		`SELECT event, previous_status, new_status FROM service_order_history WHERE service_order_id = $1 AND event = 'diagnosis_started'`,
		order.ID,
	).Scan(&event, &previousStatus, &newStatus)
	require.NoError(t, err)
	assert.Equal(t, "RECEBIDA", previousStatus)
	assert.Equal(t, "EM_DIAGNOSTICO", newStatus)
}

func TestServiceOrderStartDiagnosisRejectsNonRecebida(t *testing.T) {
	pool, server, authToken := testServiceOrderServer(t)
	order := createServiceOrder(t, server, pool)

	resp := doAuthJSON(t, http.MethodPost, server+"/api/v1/service-orders/"+order.ID+"/diagnosis", nil, authToken)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	resp = doAuthJSON(t, http.MethodPost, server+"/api/v1/service-orders/"+order.ID+"/diagnosis", nil, authToken)
	assert.Equal(t, http.StatusConflict, resp.StatusCode)
}

func TestServiceOrderDiagnosisRequiresAuth(t *testing.T) {
	pool, server, _ := testServiceOrderServer(t)
	order := createServiceOrder(t, server, pool)

	resp := doAuthJSON(t, http.MethodPost, server+"/api/v1/service-orders/"+order.ID+"/diagnosis", nil, "")
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestServiceOrderComposeQuoteFullFlow(t *testing.T) {
	pool, server, authToken := testServiceOrderServer(t)
	order := createServiceOrder(t, server, pool)
	productID := insertProduct(t, pool, 35.90, true)
	serviceID := insertService(t, pool)

	stockBefore := productStock(t, pool, productID)

	resp := doAuthJSON(t, http.MethodPost, server+"/api/v1/service-orders/"+order.ID+"/diagnosis", nil, authToken)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	body := map[string]any{
		"items": []map[string]any{
			{"productId": productID, "quantity": 2},
			{"serviceId": serviceID, "quantity": 1, "totalAmount": 999999}, // extraneous field must be ignored (RF06)
		},
	}
	resp = doAuthJSON(t, http.MethodPut, server+"/api/v1/service-orders/"+order.ID+"/quote", body, authToken)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var quote serviceorder.QuoteResponse
	decodeBody(t, resp, &quote)
	assert.InDelta(t, 2*35.90+80.00, quote.TotalAmount, 0.0001)
	require.Len(t, quote.Items, 2)

	// Composing a quote no longer transitions the order by itself — only
	// sending it does (specs/service-order-quote-decision/).
	var status string
	err := pool.QueryRow(context.Background(), `SELECT status FROM service_orders WHERE id = $1`, order.ID).Scan(&status)
	require.NoError(t, err)
	assert.Equal(t, "EM_DIAGNOSTICO", status)

	// Stock must be untouched by composing a quote (requirements.md §3.8).
	assert.Equal(t, stockBefore, productStock(t, pool, productID))

	// GET must return the same quote.
	getResp := doAuthJSON(t, http.MethodGet, server+"/api/v1/service-orders/"+order.ID+"/quote", nil, authToken)
	require.Equal(t, http.StatusOK, getResp.StatusCode)
	var fetched serviceorder.QuoteResponse
	decodeBody(t, getResp, &fetched)
	assert.InDelta(t, quote.TotalAmount, fetched.TotalAmount, 0.0001)

	// A quote_composed history event must exist.
	var event string
	err = pool.QueryRow(context.Background(),
		`SELECT event FROM service_order_history WHERE service_order_id = $1 AND event = 'quote_composed'`,
		order.ID,
	).Scan(&event)
	require.NoError(t, err)
}

func TestServiceOrderComposeQuoteRejectsBeforeDiagnosis(t *testing.T) {
	pool, server, authToken := testServiceOrderServer(t)
	order := createServiceOrder(t, server, pool)
	productID := insertProduct(t, pool, 10, true)

	body := map[string]any{"items": []map[string]any{{"productId": productID, "quantity": 1}}}
	resp := doAuthJSON(t, http.MethodPut, server+"/api/v1/service-orders/"+order.ID+"/quote", body, authToken)
	assert.Equal(t, http.StatusConflict, resp.StatusCode)
}

func TestServiceOrderComposeQuoteRejectsInactiveProduct(t *testing.T) {
	pool, server, authToken := testServiceOrderServer(t)
	order := createServiceOrder(t, server, pool)
	productID := insertProduct(t, pool, 10, false)

	doAuthJSON(t, http.MethodPost, server+"/api/v1/service-orders/"+order.ID+"/diagnosis", nil, authToken)

	body := map[string]any{"items": []map[string]any{{"productId": productID, "quantity": 1}}}
	resp := doAuthJSON(t, http.MethodPut, server+"/api/v1/service-orders/"+order.ID+"/quote", body, authToken)
	assert.Equal(t, http.StatusConflict, resp.StatusCode)
}

func TestServiceOrderComposeQuoteRejectsInvalidQuantity(t *testing.T) {
	pool, server, authToken := testServiceOrderServer(t)
	order := createServiceOrder(t, server, pool)
	productID := insertProduct(t, pool, 10, true)

	doAuthJSON(t, http.MethodPost, server+"/api/v1/service-orders/"+order.ID+"/diagnosis", nil, authToken)

	body := map[string]any{"items": []map[string]any{{"productId": productID, "quantity": 0}}}
	resp := doAuthJSON(t, http.MethodPut, server+"/api/v1/service-orders/"+order.ID+"/quote", body, authToken)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestServiceOrderComposeQuoteRejectsEmptyItems(t *testing.T) {
	pool, server, authToken := testServiceOrderServer(t)
	order := createServiceOrder(t, server, pool)

	doAuthJSON(t, http.MethodPost, server+"/api/v1/service-orders/"+order.ID+"/diagnosis", nil, authToken)

	body := map[string]any{"items": []map[string]any{}}
	resp := doAuthJSON(t, http.MethodPut, server+"/api/v1/service-orders/"+order.ID+"/quote", body, authToken)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

// TestServiceOrderComposeQuoteSnapshotSurvivesCatalogChange exercises
// requirements.md §3.5: changing a product's price/description after
// composing a quote must not alter the already-persisted item.
func TestServiceOrderComposeQuoteSnapshotSurvivesCatalogChange(t *testing.T) {
	pool, server, authToken := testServiceOrderServer(t)
	order := createServiceOrder(t, server, pool)
	productID := insertProduct(t, pool, 35.90, true)

	doAuthJSON(t, http.MethodPost, server+"/api/v1/service-orders/"+order.ID+"/diagnosis", nil, authToken)

	body := map[string]any{"items": []map[string]any{{"productId": productID, "quantity": 1}}}
	resp := doAuthJSON(t, http.MethodPut, server+"/api/v1/service-orders/"+order.ID+"/quote", body, authToken)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	_, err := pool.Exec(context.Background(), `UPDATE products SET unit_price = 999.99, name = 'Changed Name', description = 'Changed description' WHERE id = $1`, productID)
	require.NoError(t, err)

	getResp := doAuthJSON(t, http.MethodGet, server+"/api/v1/service-orders/"+order.ID+"/quote", nil, authToken)
	require.Equal(t, http.StatusOK, getResp.StatusCode)
	var fetched serviceorder.QuoteResponse
	decodeBody(t, getResp, &fetched)

	require.Len(t, fetched.Items, 1)
	assert.InDelta(t, 35.90, fetched.Items[0].UnitPrice, 0.0001)
	assert.NotEqual(t, "Changed description", fetched.Items[0].Description)
}

func TestServiceOrderGetQuoteNotFoundBeforeComposition(t *testing.T) {
	pool, server, authToken := testServiceOrderServer(t)
	order := createServiceOrder(t, server, pool)

	resp := doAuthJSON(t, http.MethodGet, server+"/api/v1/service-orders/"+order.ID+"/quote", nil, authToken)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

// ---- specs/service-order-query/ ----

func TestServiceOrderListRequiresAuth(t *testing.T) {
	_, server, _ := testServiceOrderServer(t)

	resp := doAuthJSON(t, http.MethodGet, server+"/api/v1/service-orders", nil, "")
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestServiceOrderDetailRequiresAuth(t *testing.T) {
	pool, server, _ := testServiceOrderServer(t)
	order := createServiceOrder(t, server, pool)

	resp := doAuthJSON(t, http.MethodGet, server+"/api/v1/service-orders/"+order.ID, nil, "")
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestServiceOrderListNoFilterReturnsMostRecentFirst(t *testing.T) {
	pool, server, authToken := testServiceOrderServer(t)
	older := createServiceOrder(t, server, pool)
	time.Sleep(10 * time.Millisecond)
	newer := createServiceOrder(t, server, pool)

	resp := doAuthJSON(t, http.MethodGet, server+"/api/v1/service-orders?pageSize=100", nil, authToken)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var list serviceorder.ListResponse
	decodeBody(t, resp, &list)

	require.GreaterOrEqual(t, len(list.Data), 2)
	positions := make(map[string]int, len(list.Data))
	for i, item := range list.Data {
		positions[item.ID] = i
	}
	assert.Less(t, positions[newer.ID], positions[older.ID])
}

func TestServiceOrderListFiltersByCode(t *testing.T) {
	pool, server, authToken := testServiceOrderServer(t)
	createServiceOrder(t, server, pool)
	target := createServiceOrder(t, server, pool)

	resp := doAuthJSON(t, http.MethodGet, server+fmt.Sprintf("/api/v1/service-orders?code=%d", target.Code), nil, authToken)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var list serviceorder.ListResponse
	decodeBody(t, resp, &list)

	require.Len(t, list.Data, 1)
	assert.Equal(t, target.ID, list.Data[0].ID)
}

func TestServiceOrderListFiltersByStatus(t *testing.T) {
	pool, server, authToken := testServiceOrderServer(t)
	received := createServiceOrder(t, server, pool)
	diagnosing := createServiceOrder(t, server, pool)
	resp := doAuthJSON(t, http.MethodPost, server+"/api/v1/service-orders/"+diagnosing.ID+"/diagnosis", nil, authToken)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	resp = doAuthJSON(t, http.MethodGet, server+"/api/v1/service-orders?status=EM_DIAGNOSTICO&pageSize=100", nil, authToken)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var list serviceorder.ListResponse
	decodeBody(t, resp, &list)

	var ids []string
	for _, item := range list.Data {
		ids = append(ids, item.ID)
	}
	assert.Contains(t, ids, diagnosing.ID)
	assert.NotContains(t, ids, received.ID)
}

func TestServiceOrderListFiltersByCustomerDocument(t *testing.T) {
	pool, server, authToken := testServiceOrderServer(t)
	createdCustomer := insertActiveCustomer(t, server, pool)
	vehicleID := insertVehicle(t, pool, createdCustomer.ID, randomLicensePlate(), true)
	resp := doAuthJSON(t, http.MethodPost, server+"/api/v1/service-orders", serviceorder.CreateRequest{
		CustomerID: createdCustomer.ID,
		VehicleID:  vehicleID,
	}, authToken)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	var target serviceorder.Response
	decodeBody(t, resp, &target)
	cleanupServiceOrder(t, pool, target.ID)

	createServiceOrder(t, server, pool) // a different customer/vehicle pair

	resp = doAuthJSON(t, http.MethodGet, server+"/api/v1/service-orders?document="+createdCustomer.Document+"&pageSize=100", nil, authToken)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var list serviceorder.ListResponse
	decodeBody(t, resp, &list)

	require.Len(t, list.Data, 1)
	assert.Equal(t, target.ID, list.Data[0].ID)
}

func TestServiceOrderListFiltersByLicensePlate(t *testing.T) {
	pool, server, authToken := testServiceOrderServer(t)
	target := createServiceOrder(t, server, pool)
	createServiceOrder(t, server, pool)

	resp := doAuthJSON(t, http.MethodGet, server+"/api/v1/service-orders?licensePlate="+target.Vehicle.LicensePlate, nil, authToken)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var list serviceorder.ListResponse
	decodeBody(t, resp, &list)

	require.Len(t, list.Data, 1)
	assert.Equal(t, target.ID, list.Data[0].ID)
}

func TestServiceOrderListFiltersByCreatedRange(t *testing.T) {
	pool, server, authToken := testServiceOrderServer(t)
	target := createServiceOrder(t, server, pool)

	from := target.CreatedAt.Add(-1 * time.Minute).UTC().Format(time.RFC3339)
	to := target.CreatedAt.Add(1 * time.Minute).UTC().Format(time.RFC3339)
	resp := doAuthJSON(t, http.MethodGet,
		server+"/api/v1/service-orders?createdFrom="+url.QueryEscape(from)+"&createdTo="+url.QueryEscape(to), nil, authToken)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var list serviceorder.ListResponse
	decodeBody(t, resp, &list)

	var ids []string
	for _, item := range list.Data {
		ids = append(ids, item.ID)
	}
	assert.Contains(t, ids, target.ID)

	farFuture := target.CreatedAt.Add(24 * time.Hour).UTC().Format(time.RFC3339)
	resp = doAuthJSON(t, http.MethodGet, server+"/api/v1/service-orders?createdFrom="+url.QueryEscape(farFuture), nil, authToken)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	decodeBody(t, resp, &list)
	ids = nil
	for _, item := range list.Data {
		ids = append(ids, item.ID)
	}
	assert.NotContains(t, ids, target.ID)
}

func TestServiceOrderListInvalidStatusFilterIsRejected(t *testing.T) {
	_, server, authToken := testServiceOrderServer(t)

	resp := doAuthJSON(t, http.MethodGet, server+"/api/v1/service-orders?status=NOT_A_STATUS", nil, authToken)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestServiceOrderListPagination(t *testing.T) {
	pool, server, authToken := testServiceOrderServer(t)
	for i := 0; i < 3; i++ {
		createServiceOrder(t, server, pool)
	}

	resp := doAuthJSON(t, http.MethodGet, server+"/api/v1/service-orders?page=1&pageSize=2", nil, authToken)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var firstPage serviceorder.ListResponse
	decodeBody(t, resp, &firstPage)
	assert.Len(t, firstPage.Data, 2)
	assert.Equal(t, 1, firstPage.Page)
	assert.Equal(t, 2, firstPage.PageSize)
	assert.GreaterOrEqual(t, firstPage.Total, 3)
}

func TestServiceOrderDetailByIDFullLifecycle(t *testing.T) {
	pool, server, authToken := testServiceOrderServer(t)
	order := createServiceOrder(t, server, pool)
	productID := insertProduct(t, pool, 35.90, true)

	resp := doAuthJSON(t, http.MethodPost, server+"/api/v1/service-orders/"+order.ID+"/diagnosis", nil, authToken)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	body := map[string]any{"items": []map[string]any{{"productId": productID, "quantity": 2}}}
	resp = doAuthJSON(t, http.MethodPut, server+"/api/v1/service-orders/"+order.ID+"/quote", body, authToken)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	resp = doAuthJSON(t, http.MethodGet, server+"/api/v1/service-orders/"+order.ID, nil, authToken)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var detail serviceorder.DetailResponse
	decodeBody(t, resp, &detail)

	assert.Equal(t, order.ID, detail.ID)
	assert.Equal(t, "EM_DIAGNOSTICO", detail.Status, "composing a quote no longer transitions the order — only sending it does (specs/service-order-quote-decision/)")
	require.NotNil(t, detail.Quote)
	assert.InDelta(t, 2*35.90, detail.Quote.TotalAmount, 0.0001)
	require.Len(t, detail.Quote.Items, 1)
	assert.GreaterOrEqual(t, len(detail.History), 2) // creation + quote_composed (diagnosis_started too)
}

func TestServiceOrderDetailByIDBeforeDiagnosis(t *testing.T) {
	pool, server, authToken := testServiceOrderServer(t)
	order := createServiceOrder(t, server, pool)

	resp := doAuthJSON(t, http.MethodGet, server+"/api/v1/service-orders/"+order.ID, nil, authToken)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var detail serviceorder.DetailResponse
	decodeBody(t, resp, &detail)

	assert.Equal(t, "RECEBIDA", detail.Status)
	assert.Nil(t, detail.Quote)
	require.Len(t, detail.History, 1)
	assert.Equal(t, "creation", detail.History[0].Event)
}

func TestServiceOrderDetailByCodeMatchesDetailByID(t *testing.T) {
	pool, server, authToken := testServiceOrderServer(t)
	order := createServiceOrder(t, server, pool)

	resp := doAuthJSON(t, http.MethodGet, server+fmt.Sprintf("/api/v1/service-orders/%d", order.Code), nil, authToken)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var byCode serviceorder.DetailResponse
	decodeBody(t, resp, &byCode)

	assert.Equal(t, order.ID, byCode.ID)
	assert.Equal(t, order.Customer.ID, byCode.Customer.ID)
	assert.Equal(t, order.Vehicle.ID, byCode.Vehicle.ID)
}

func TestServiceOrderDetailUnknownIDReturns404(t *testing.T) {
	_, server, authToken := testServiceOrderServer(t)

	resp := doAuthJSON(t, http.MethodGet, server+"/api/v1/service-orders/"+uuid.NewString(), nil, authToken)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestServiceOrderDetailUnknownCodeReturns404(t *testing.T) {
	_, server, authToken := testServiceOrderServer(t)

	resp := doAuthJSON(t, http.MethodGet, server+"/api/v1/service-orders/999999999", nil, authToken)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestServiceOrderDetailMalformedIdentifierReturns404(t *testing.T) {
	_, server, authToken := testServiceOrderServer(t)

	resp := doAuthJSON(t, http.MethodGet, server+"/api/v1/service-orders/not-a-valid-identifier", nil, authToken)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

// ---- specs/service-order-execution/ ----
//
// No endpoint implements AGUARDANDO_APROVACAO -> EM_EXECUCAO yet
// (requirements.md §2.1) — moveServiceOrderToEmExecucao and approveQuote
// below reach that precondition directly via SQL, the same "insert the
// missing precondition directly" pattern insertVehicle/insertProduct/
// insertService already use for gaps the public API can't fill.

func moveServiceOrderToEmExecucao(t *testing.T, pool *pgxpool.Pool, orderID string) {
	t.Helper()
	_, err := pool.Exec(context.Background(), `UPDATE service_orders SET status = 'EM_EXECUCAO' WHERE id = $1`, orderID)
	require.NoError(t, err)
}

func approveQuote(t *testing.T, pool *pgxpool.Pool, orderID string) {
	t.Helper()
	_, err := pool.Exec(context.Background(), `UPDATE quotes SET status = 'APPROVED' WHERE service_order_id = $1`, orderID)
	require.NoError(t, err)
}

// createOrderInExecutionWithRequiredService builds a full order through
// diagnosis + quote composition (one service line item), then jumps it to
// EM_EXECUCAO with its quote APPROVED, returning the order and the service
// id its quote requires an execution for (BR5).
func createOrderInExecutionWithRequiredService(t *testing.T, server string, pool *pgxpool.Pool, authToken string) (serviceorder.Response, string) {
	t.Helper()

	order := createServiceOrder(t, server, pool)
	serviceID := insertService(t, pool)

	resp := doAuthJSON(t, http.MethodPost, server+"/api/v1/service-orders/"+order.ID+"/diagnosis", nil, authToken)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	body := map[string]any{"items": []map[string]any{{"serviceId": serviceID, "quantity": 1}}}
	resp = doAuthJSON(t, http.MethodPut, server+"/api/v1/service-orders/"+order.ID+"/quote", body, authToken)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	approveQuote(t, pool, order.ID)
	moveServiceOrderToEmExecucao(t, pool, order.ID)

	return order, serviceID
}

func TestServiceExecutionStartSuccess(t *testing.T) {
	pool, server, authToken := testServiceOrderServer(t)
	order, serviceID := createOrderInExecutionWithRequiredService(t, server, pool, authToken)

	resp := doAuthJSON(t, http.MethodPost, server+"/api/v1/service-orders/"+order.ID+"/executions",
		map[string]any{"serviceId": serviceID}, authToken)
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var execution serviceorder.ServiceExecutionResponse
	decodeBody(t, resp, &execution)
	assert.Equal(t, order.ID, execution.ServiceOrderID)
	assert.Equal(t, serviceID, execution.ServiceID)
	assert.False(t, execution.StartedAt.IsZero())
	assert.Nil(t, execution.EndedAt)
}

// TestServiceExecutionStartRejectsBeforeApproval covers the acceptance
// checklist's "Execução não pode ser iniciada sem orçamento aprovado" (BR2),
// via the EM_EXECUCAO precondition (requirements.md §2.1).
func TestServiceExecutionStartRejectsBeforeApproval(t *testing.T) {
	pool, server, authToken := testServiceOrderServer(t)
	order := createServiceOrder(t, server, pool) // still RECEBIDA
	serviceID := insertService(t, pool)

	resp := doAuthJSON(t, http.MethodPost, server+"/api/v1/service-orders/"+order.ID+"/executions",
		map[string]any{"serviceId": serviceID}, authToken)
	assert.Equal(t, http.StatusConflict, resp.StatusCode)
}

func TestServiceExecutionStartRequiresAuth(t *testing.T) {
	pool, server, authToken := testServiceOrderServer(t)
	order, serviceID := createOrderInExecutionWithRequiredService(t, server, pool, authToken)

	resp := doAuthJSON(t, http.MethodPost, server+"/api/v1/service-orders/"+order.ID+"/executions",
		map[string]any{"serviceId": serviceID}, "")
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestServiceExecutionFinishSuccess(t *testing.T) {
	pool, server, authToken := testServiceOrderServer(t)
	order, serviceID := createOrderInExecutionWithRequiredService(t, server, pool, authToken)

	startResp := doAuthJSON(t, http.MethodPost, server+"/api/v1/service-orders/"+order.ID+"/executions",
		map[string]any{"serviceId": serviceID}, authToken)
	require.Equal(t, http.StatusCreated, startResp.StatusCode)
	var started serviceorder.ServiceExecutionResponse
	decodeBody(t, startResp, &started)

	// No body: the server must default endedAt to now (design.md §2.5).
	finishResp := doAuthJSON(t, http.MethodPost, server+"/api/v1/service-orders/"+order.ID+"/executions/"+started.ID+"/finish", nil, authToken)
	require.Equal(t, http.StatusOK, finishResp.StatusCode)

	var finished serviceorder.ServiceExecutionResponse
	decodeBody(t, finishResp, &finished)
	require.NotNil(t, finished.EndedAt)
	assert.False(t, finished.EndedAt.Before(finished.StartedAt))
}

// TestServiceExecutionFinishRejectsEndBeforeStart covers BR4.
func TestServiceExecutionFinishRejectsEndBeforeStart(t *testing.T) {
	pool, server, authToken := testServiceOrderServer(t)
	order, serviceID := createOrderInExecutionWithRequiredService(t, server, pool, authToken)

	startResp := doAuthJSON(t, http.MethodPost, server+"/api/v1/service-orders/"+order.ID+"/executions",
		map[string]any{"serviceId": serviceID}, authToken)
	require.Equal(t, http.StatusCreated, startResp.StatusCode)
	var started serviceorder.ServiceExecutionResponse
	decodeBody(t, startResp, &started)

	before := started.StartedAt.Add(-1 * time.Hour)
	finishResp := doAuthJSON(t, http.MethodPost, server+"/api/v1/service-orders/"+order.ID+"/executions/"+started.ID+"/finish",
		map[string]any{"endedAt": before.Format(time.RFC3339)}, authToken)
	assert.Equal(t, http.StatusBadRequest, finishResp.StatusCode)
}

func TestServiceExecutionFinishRejectsUnknownExecution(t *testing.T) {
	pool, server, authToken := testServiceOrderServer(t)
	order, _ := createOrderInExecutionWithRequiredService(t, server, pool, authToken)

	resp := doAuthJSON(t, http.MethodPost, server+"/api/v1/service-orders/"+order.ID+"/executions/"+uuid.NewString()+"/finish", nil, authToken)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

// TestServiceOrderFinalizeRequiresCompletedExecutions covers the acceptance
// checklist's "Finalização da OS exige execuções concluídas" (BR5).
func TestServiceOrderFinalizeRequiresCompletedExecutions(t *testing.T) {
	pool, server, authToken := testServiceOrderServer(t)
	order, serviceID := createOrderInExecutionWithRequiredService(t, server, pool, authToken)

	finalizeResp := doAuthJSON(t, http.MethodPost, server+"/api/v1/service-orders/"+order.ID+"/finalize", nil, authToken)
	assert.Equal(t, http.StatusConflict, finalizeResp.StatusCode, "the required execution has not even started yet")

	startResp := doAuthJSON(t, http.MethodPost, server+"/api/v1/service-orders/"+order.ID+"/executions",
		map[string]any{"serviceId": serviceID}, authToken)
	require.Equal(t, http.StatusCreated, startResp.StatusCode)
	var started serviceorder.ServiceExecutionResponse
	decodeBody(t, startResp, &started)

	finalizeResp = doAuthJSON(t, http.MethodPost, server+"/api/v1/service-orders/"+order.ID+"/finalize", nil, authToken)
	assert.Equal(t, http.StatusConflict, finalizeResp.StatusCode, "the required execution has started but not finished")

	finishResp := doAuthJSON(t, http.MethodPost, server+"/api/v1/service-orders/"+order.ID+"/executions/"+started.ID+"/finish", nil, authToken)
	require.Equal(t, http.StatusOK, finishResp.StatusCode)

	finalizeResp = doAuthJSON(t, http.MethodPost, server+"/api/v1/service-orders/"+order.ID+"/finalize", nil, authToken)
	require.Equal(t, http.StatusOK, finalizeResp.StatusCode)

	var finalized serviceorder.Response
	decodeBody(t, finalizeResp, &finalized)
	assert.Equal(t, "FINALIZADA", finalized.Status)

	var event, previousStatus, newStatus string
	err := pool.QueryRow(context.Background(),
		`SELECT event, previous_status, new_status FROM service_order_history WHERE service_order_id = $1 AND event = 'completion'`,
		order.ID,
	).Scan(&event, &previousStatus, &newStatus)
	require.NoError(t, err, "finalizing must generate a history entry (BR8)")
	assert.Equal(t, "EM_EXECUCAO", previousStatus)
	assert.Equal(t, "FINALIZADA", newStatus)
}

func TestServiceOrderFinalizeRejectsNonEmExecucao(t *testing.T) {
	pool, server, authToken := testServiceOrderServer(t)
	order := createServiceOrder(t, server, pool) // still RECEBIDA

	resp := doAuthJSON(t, http.MethodPost, server+"/api/v1/service-orders/"+order.ID+"/finalize", nil, authToken)
	assert.Equal(t, http.StatusConflict, resp.StatusCode)
}

func TestServiceOrderDeliverSuccess(t *testing.T) {
	pool, server, authToken := testServiceOrderServer(t)
	order := createServiceOrder(t, server, pool)
	moveServiceOrderToEmExecucao(t, pool, order.ID)

	finalizeResp := doAuthJSON(t, http.MethodPost, server+"/api/v1/service-orders/"+order.ID+"/finalize", nil, authToken)
	require.Equal(t, http.StatusOK, finalizeResp.StatusCode)

	deliverResp := doAuthJSON(t, http.MethodPost, server+"/api/v1/service-orders/"+order.ID+"/deliver", nil, authToken)
	require.Equal(t, http.StatusOK, deliverResp.StatusCode)

	var delivered serviceorder.Response
	decodeBody(t, deliverResp, &delivered)
	assert.Equal(t, "ENTREGUE", delivered.Status)

	var event, previousStatus, newStatus string
	err := pool.QueryRow(context.Background(),
		`SELECT event, previous_status, new_status FROM service_order_history WHERE service_order_id = $1 AND event = 'delivery'`,
		order.ID,
	).Scan(&event, &previousStatus, &newStatus)
	require.NoError(t, err, "delivering must generate a history entry (BR8)")
	assert.Equal(t, "FINALIZADA", previousStatus)
	assert.Equal(t, "ENTREGUE", newStatus)
}

func TestServiceOrderDeliverRejectsNonFinalizada(t *testing.T) {
	pool, server, authToken := testServiceOrderServer(t)
	order := createServiceOrder(t, server, pool)
	moveServiceOrderToEmExecucao(t, pool, order.ID)

	resp := doAuthJSON(t, http.MethodPost, server+"/api/v1/service-orders/"+order.ID+"/deliver", nil, authToken)
	assert.Equal(t, http.StatusConflict, resp.StatusCode)
}

// TestServiceOrderFinalizedRejectsNewExecutions covers the acceptance
// checklist's "OS entregue/finalizada não aceita novas alterações
// operacionais" (BR6).
func TestServiceOrderFinalizedRejectsNewExecutions(t *testing.T) {
	pool, server, authToken := testServiceOrderServer(t)
	order := createServiceOrder(t, server, pool)
	moveServiceOrderToEmExecucao(t, pool, order.ID)
	serviceID := insertService(t, pool)

	finalizeResp := doAuthJSON(t, http.MethodPost, server+"/api/v1/service-orders/"+order.ID+"/finalize", nil, authToken)
	require.Equal(t, http.StatusOK, finalizeResp.StatusCode)

	resp := doAuthJSON(t, http.MethodPost, server+"/api/v1/service-orders/"+order.ID+"/executions",
		map[string]any{"serviceId": serviceID}, authToken)
	assert.Equal(t, http.StatusConflict, resp.StatusCode)
}

// ---- specs/service-order-stock-usage/ ----

func TestRegisterStockUsageSuccess(t *testing.T) {
	pool, server, authToken := testServiceOrderServer(t)
	order := createServiceOrder(t, server, pool)
	moveServiceOrderToEmExecucao(t, pool, order.ID)
	productID := insertProduct(t, pool, 35.90, true) // seeded with current_stock = 10

	resp := doAuthJSON(t, http.MethodPost, server+"/api/v1/service-orders/"+order.ID+"/stock-movements",
		map[string]any{"items": []map[string]any{{"productId": productID, "quantity": 3}}}, authToken)
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var body serviceorder.StockMovementListResponse
	decodeBody(t, resp, &body)
	require.Len(t, body.Items, 1)
	assert.Equal(t, productID, body.Items[0].ProductID)
	require.NotNil(t, body.Items[0].ServiceOrderID)
	assert.Equal(t, order.ID, *body.Items[0].ServiceOrderID)
	assert.Equal(t, "EXIT", body.Items[0].Type)
	assert.Equal(t, 10, body.Items[0].PreviousStock)
	assert.Equal(t, 7, body.Items[0].NewStock)
	assert.Equal(t, 7, productStock(t, pool, productID))
}

func TestRegisterStockUsageRejectsBeforeEmExecucao(t *testing.T) {
	pool, server, authToken := testServiceOrderServer(t)
	order := createServiceOrder(t, server, pool) // still RECEBIDA
	productID := insertProduct(t, pool, 10, true)

	resp := doAuthJSON(t, http.MethodPost, server+"/api/v1/service-orders/"+order.ID+"/stock-movements",
		map[string]any{"items": []map[string]any{{"productId": productID, "quantity": 1}}}, authToken)
	assert.Equal(t, http.StatusConflict, resp.StatusCode)
	assert.Equal(t, 10, productStock(t, pool, productID))
}

// TestRegisterStockUsageRejectsInsufficientStock covers BR4/the acceptance
// checklist's "saldo insuficiente impede a operação" and "o estoque nunca
// fica negativo".
func TestRegisterStockUsageRejectsInsufficientStock(t *testing.T) {
	pool, server, authToken := testServiceOrderServer(t)
	order := createServiceOrder(t, server, pool)
	moveServiceOrderToEmExecucao(t, pool, order.ID)
	productID := insertProduct(t, pool, 10, true) // current_stock = 10

	resp := doAuthJSON(t, http.MethodPost, server+"/api/v1/service-orders/"+order.ID+"/stock-movements",
		map[string]any{"items": []map[string]any{{"productId": productID, "quantity": 50}}}, authToken)
	assert.Equal(t, http.StatusConflict, resp.StatusCode)
	assert.Equal(t, 10, productStock(t, pool, productID), "no deduction on a rejected request")
}

func TestRegisterStockUsageRejectsInactiveProduct(t *testing.T) {
	pool, server, authToken := testServiceOrderServer(t)
	order := createServiceOrder(t, server, pool)
	moveServiceOrderToEmExecucao(t, pool, order.ID)
	productID := insertProduct(t, pool, 10, false)

	resp := doAuthJSON(t, http.MethodPost, server+"/api/v1/service-orders/"+order.ID+"/stock-movements",
		map[string]any{"items": []map[string]any{{"productId": productID, "quantity": 1}}}, authToken)
	assert.Equal(t, http.StatusConflict, resp.StatusCode)
}

func TestRegisterStockUsageRejectsUnknownProduct(t *testing.T) {
	pool, server, authToken := testServiceOrderServer(t)
	order := createServiceOrder(t, server, pool)
	moveServiceOrderToEmExecucao(t, pool, order.ID)

	resp := doAuthJSON(t, http.MethodPost, server+"/api/v1/service-orders/"+order.ID+"/stock-movements",
		map[string]any{"items": []map[string]any{{"productId": uuid.NewString(), "quantity": 1}}}, authToken)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestRegisterStockUsageRejectsInvalidQuantity(t *testing.T) {
	pool, server, authToken := testServiceOrderServer(t)
	order := createServiceOrder(t, server, pool)
	moveServiceOrderToEmExecucao(t, pool, order.ID)
	productID := insertProduct(t, pool, 10, true)

	resp := doAuthJSON(t, http.MethodPost, server+"/api/v1/service-orders/"+order.ID+"/stock-movements",
		map[string]any{"items": []map[string]any{{"productId": productID, "quantity": 0}}}, authToken)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestRegisterStockUsageRejectsEmptyItems(t *testing.T) {
	pool, server, authToken := testServiceOrderServer(t)
	order := createServiceOrder(t, server, pool)
	moveServiceOrderToEmExecucao(t, pool, order.ID)

	resp := doAuthJSON(t, http.MethodPost, server+"/api/v1/service-orders/"+order.ID+"/stock-movements",
		map[string]any{"items": []map[string]any{}}, authToken)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

// TestRegisterStockUsageRollsBackPartialFailure covers BR7/the acceptance
// checklist's "falha parcial desfaz todas as baixas da requisição": a
// multi-item request where the second item fails must leave the first
// item's product balance untouched.
func TestRegisterStockUsageRollsBackPartialFailure(t *testing.T) {
	pool, server, authToken := testServiceOrderServer(t)
	order := createServiceOrder(t, server, pool)
	moveServiceOrderToEmExecucao(t, pool, order.ID)
	okProductID := insertProduct(t, pool, 10, true)    // current_stock = 10, would succeed alone
	shortProductID := insertProduct(t, pool, 10, true) // current_stock = 10, insufficient for the requested quantity

	resp := doAuthJSON(t, http.MethodPost, server+"/api/v1/service-orders/"+order.ID+"/stock-movements",
		map[string]any{"items": []map[string]any{
			{"productId": okProductID, "quantity": 2},
			{"productId": shortProductID, "quantity": 50},
		}}, authToken)
	assert.Equal(t, http.StatusConflict, resp.StatusCode)

	assert.Equal(t, 10, productStock(t, pool, okProductID), "the first item's deduction must be rolled back")
	assert.Equal(t, 10, productStock(t, pool, shortProductID))

	var count int
	err := pool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM stock_movements WHERE service_order_id = $1`, order.ID,
	).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 0, count, "no movement row must survive a rolled-back request")
}

func TestRegisterStockUsageRequiresAuth(t *testing.T) {
	pool, server, _ := testServiceOrderServer(t)
	order := createServiceOrder(t, server, pool)
	moveServiceOrderToEmExecucao(t, pool, order.ID)
	productID := insertProduct(t, pool, 10, true)

	resp := doAuthJSON(t, http.MethodPost, server+"/api/v1/service-orders/"+order.ID+"/stock-movements",
		map[string]any{"items": []map[string]any{{"productId": productID, "quantity": 1}}}, "")
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

// TestRegisterStockUsageConcurrencyNeverOversells covers BR8/the acceptance
// checklist's "operações concorrentes não consomem o mesmo saldo": several
// requests racing against a product with limited stock must never let the
// balance go negative or oversell past what stock actually allows.
func TestRegisterStockUsageConcurrencyNeverOversells(t *testing.T) {
	pool, server, authToken := testServiceOrderServer(t)
	order := createServiceOrder(t, server, pool)
	moveServiceOrderToEmExecucao(t, pool, order.ID)
	productID := insertProduct(t, pool, 10, true) // current_stock = 10

	const attempts = 5
	const quantityPerAttempt = 3 // 5 * 3 = 15 requested against a balance of 10: at most 3 can succeed

	var wg sync.WaitGroup
	statusCodes := make([]int, attempts)
	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			resp := doAuthJSON(t, http.MethodPost, server+"/api/v1/service-orders/"+order.ID+"/stock-movements",
				map[string]any{"items": []map[string]any{{"productId": productID, "quantity": quantityPerAttempt}}}, authToken)
			statusCodes[index] = resp.StatusCode
		}(i)
	}
	wg.Wait()

	succeeded := 0
	for _, code := range statusCodes {
		if code == http.StatusCreated {
			succeeded++
		} else {
			assert.Equal(t, http.StatusConflict, code)
		}
	}
	assert.Equal(t, 3, succeeded, "exactly 3 attempts of 3 units each fit in a balance of 10")
	assert.Equal(t, 1, productStock(t, pool, productID), "10 - 3*3 = 1, never negative")
}

func TestListStockMovementsReturnsRegisteredUsage(t *testing.T) {
	pool, server, authToken := testServiceOrderServer(t)
	order := createServiceOrder(t, server, pool)
	moveServiceOrderToEmExecucao(t, pool, order.ID)
	productID := insertProduct(t, pool, 10, true)

	registerResp := doAuthJSON(t, http.MethodPost, server+"/api/v1/service-orders/"+order.ID+"/stock-movements",
		map[string]any{"items": []map[string]any{{"productId": productID, "quantity": 2}}}, authToken)
	require.Equal(t, http.StatusCreated, registerResp.StatusCode)

	listResp := doAuthJSON(t, http.MethodGet, server+"/api/v1/service-orders/"+order.ID+"/stock-movements", nil, authToken)
	require.Equal(t, http.StatusOK, listResp.StatusCode)

	var body serviceorder.StockMovementListResponse
	decodeBody(t, listResp, &body)
	require.Len(t, body.Items, 1)
	assert.Equal(t, 2, body.Items[0].Quantity)
}

// TestReverseStockMovementSuccess covers BR9/the acceptance checklist's
// "estorno, se incluído no MVP, preserva a movimentação original".
func TestReverseStockMovementSuccess(t *testing.T) {
	pool, server, authToken := testServiceOrderServer(t)
	order := createServiceOrder(t, server, pool)
	moveServiceOrderToEmExecucao(t, pool, order.ID)
	productID := insertProduct(t, pool, 10, true) // current_stock = 10

	registerResp := doAuthJSON(t, http.MethodPost, server+"/api/v1/service-orders/"+order.ID+"/stock-movements",
		map[string]any{"items": []map[string]any{{"productId": productID, "quantity": 4}}}, authToken)
	require.Equal(t, http.StatusCreated, registerResp.StatusCode)
	var registered serviceorder.StockMovementListResponse
	decodeBody(t, registerResp, &registered)
	require.Len(t, registered.Items, 1)
	require.Equal(t, 6, productStock(t, pool, productID))

	reverseResp := doAuthJSON(t, http.MethodPost,
		server+"/api/v1/service-orders/"+order.ID+"/stock-movements/"+registered.Items[0].ID+"/reversal", nil, authToken)
	require.Equal(t, http.StatusCreated, reverseResp.StatusCode)

	var reversal serviceorder.StockMovementResponse
	decodeBody(t, reverseResp, &reversal)
	assert.Equal(t, "ENTRY", reversal.Type)
	require.NotNil(t, reversal.ReversedMovementID)
	assert.Equal(t, registered.Items[0].ID, *reversal.ReversedMovementID)
	assert.Equal(t, 10, productStock(t, pool, productID), "reversal must restore the deducted quantity")

	// The original movement itself must be preserved, unmodified.
	var originalStillExists bool
	err := pool.QueryRow(context.Background(),
		`SELECT EXISTS(SELECT 1 FROM stock_movements WHERE id = $1 AND type = 'EXIT')`, registered.Items[0].ID,
	).Scan(&originalStillExists)
	require.NoError(t, err)
	assert.True(t, originalStillExists)
}

func TestReverseStockMovementRejectsSecondReversal(t *testing.T) {
	pool, server, authToken := testServiceOrderServer(t)
	order := createServiceOrder(t, server, pool)
	moveServiceOrderToEmExecucao(t, pool, order.ID)
	productID := insertProduct(t, pool, 10, true)

	registerResp := doAuthJSON(t, http.MethodPost, server+"/api/v1/service-orders/"+order.ID+"/stock-movements",
		map[string]any{"items": []map[string]any{{"productId": productID, "quantity": 4}}}, authToken)
	require.Equal(t, http.StatusCreated, registerResp.StatusCode)
	var registered serviceorder.StockMovementListResponse
	decodeBody(t, registerResp, &registered)

	firstReversal := doAuthJSON(t, http.MethodPost,
		server+"/api/v1/service-orders/"+order.ID+"/stock-movements/"+registered.Items[0].ID+"/reversal", nil, authToken)
	require.Equal(t, http.StatusCreated, firstReversal.StatusCode)

	secondReversal := doAuthJSON(t, http.MethodPost,
		server+"/api/v1/service-orders/"+order.ID+"/stock-movements/"+registered.Items[0].ID+"/reversal", nil, authToken)
	assert.Equal(t, http.StatusConflict, secondReversal.StatusCode)
}

func TestReverseStockMovementRejectsUnknownMovement(t *testing.T) {
	pool, server, authToken := testServiceOrderServer(t)
	order := createServiceOrder(t, server, pool)
	moveServiceOrderToEmExecucao(t, pool, order.ID)

	resp := doAuthJSON(t, http.MethodPost,
		server+"/api/v1/service-orders/"+order.ID+"/stock-movements/"+uuid.NewString()+"/reversal", nil, authToken)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}
