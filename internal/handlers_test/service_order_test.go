package handlers_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
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
	customer.RegisterRoutes(router, customerService)
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
	pool, server, _ := testServiceOrderServer(t)

	createdCustomer := insertActiveCustomer(t, server, pool)
	vehicleID := insertVehicle(t, pool, createdCustomer.ID, randomLicensePlate(), true)
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
	pool, server, _ := testServiceOrderServer(t)

	plate := randomLicensePlate()
	createdCustomer := insertActiveCustomer(t, server, pool)
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
	pool, server, _ := testServiceOrderServer(t)

	createdCustomer := insertActiveCustomer(t, server, pool)
	vehicleID := insertVehicle(t, pool, createdCustomer.ID, randomLicensePlate(), true)

	deactivateResp := doJSON(t, http.MethodDelete, server+"/api/v1/customers/"+createdCustomer.ID, nil)
	require.Equal(t, http.StatusOK, deactivateResp.StatusCode)

	resp := doJSON(t, http.MethodPost, server+"/api/v1/service-orders", serviceorder.CreateRequest{
		CustomerID: createdCustomer.ID,
		VehicleID:  vehicleID,
	})
	assert.Equal(t, http.StatusConflict, resp.StatusCode)
}

func TestServiceOrderCreateRejectsInactiveVehicle(t *testing.T) {
	pool, server, _ := testServiceOrderServer(t)

	createdCustomer := insertActiveCustomer(t, server, pool)
	vehicleID := insertVehicle(t, pool, createdCustomer.ID, randomLicensePlate(), false)

	resp := doJSON(t, http.MethodPost, server+"/api/v1/service-orders", serviceorder.CreateRequest{
		CustomerID: createdCustomer.ID,
		VehicleID:  vehicleID,
	})
	assert.Equal(t, http.StatusConflict, resp.StatusCode)
}

func TestServiceOrderCreateRejectsVehicleFromAnotherCustomer(t *testing.T) {
	pool, server, _ := testServiceOrderServer(t)

	firstCustomer := insertActiveCustomer(t, server, pool)
	secondCustomer := insertActiveCustomer(t, server, pool)
	vehicleID := insertVehicle(t, pool, secondCustomer.ID, randomLicensePlate(), true)

	resp := doJSON(t, http.MethodPost, server+"/api/v1/service-orders", serviceorder.CreateRequest{
		CustomerID: firstCustomer.ID,
		VehicleID:  vehicleID,
	})
	assert.Equal(t, http.StatusConflict, resp.StatusCode)
}

func TestServiceOrderCreateRejectsUnknownRequestedService(t *testing.T) {
	pool, server, _ := testServiceOrderServer(t)

	createdCustomer := insertActiveCustomer(t, server, pool)
	vehicleID := insertVehicle(t, pool, createdCustomer.ID, "PQR1S23", true)

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
	pool, server, _ := testServiceOrderServer(t)

	createdCustomer := insertActiveCustomer(t, server, pool)
	vehicleID := insertVehicle(t, pool, createdCustomer.ID, "TUV4W56", true)
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

// createServiceOrder creates a service order through the real API and
// returns its full response, for use as a fixture by the diagnosis/quote
// tests below.
func createServiceOrder(t *testing.T, server string, pool *pgxpool.Pool) serviceorder.Response {
	t.Helper()

	createdCustomer := insertActiveCustomer(t, server, pool)
	vehicleID := insertVehicle(t, pool, createdCustomer.ID, randomLicensePlate(), true)

	resp := doJSON(t, http.MethodPost, server+"/api/v1/service-orders", serviceorder.CreateRequest{
		CustomerID: createdCustomer.ID,
		VehicleID:  vehicleID,
	})
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var created serviceorder.Response
	decodeBody(t, resp, &created)
	cleanupServiceOrder(t, pool, created.ID)
	return created
}

var licensePlateCounter int

func randomLicensePlate() string {
	licensePlateCounter++
	return "QZX" + string(rune('0'+licensePlateCounter%10)) + "Y" + string(rune('A'+licensePlateCounter%26)) + "9"
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
	resp := doJSON(t, http.MethodPost, server+"/api/v1/service-orders", serviceorder.CreateRequest{
		CustomerID: createdCustomer.ID,
		VehicleID:  vehicleID,
	})
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
