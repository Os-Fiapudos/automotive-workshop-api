package handlers_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"automotive-workshop-api/internal/features/auth"
	"automotive-workshop-api/internal/features/customer"
	serviceorder "automotive-workshop-api/internal/features/service-order"
	servicetracking "automotive-workshop-api/internal/features/service-order-tracking"
	"automotive-workshop-api/internal/shared/token"
)

// readBodyString reads and returns response's full body, then closes it.
func readBodyString(t *testing.T, response *http.Response) string {
	t.Helper()
	defer response.Body.Close()
	raw, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	return string(raw)
}

// decodeJSONString decodes body (already read via readBodyString) into out.
func decodeJSONString(t *testing.T, body string, out any) {
	t.Helper()
	require.NoError(t, json.Unmarshal([]byte(body), out))
}

// testTrackingServer builds the real customer + service-order + tracking HTTP
// handlers wired to a real pgxpool.Pool. Service Order Tracking's own route
// is deliberately never wrapped in requireAuth (requirements.md §4/AC10), so
// no auth handler/token manager is needed here at all, unlike
// testServiceOrderServer.
func testTrackingServer(t *testing.T) (*pgxpool.Pool, string) {
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
	serviceOrderService := serviceorder.NewServiceOrderService(serviceOrderRepository, serviceOrderRepository, nil)

	trackingRepository := servicetracking.NewPostgresTrackingRepository(pool)
	trackingService := servicetracking.NewTrackingService(trackingRepository)

	// Customer/service-order creation are deliberately left unauthenticated
	// (nil requireAuth) in this router — this file only tests the tracking
	// route itself (which never requires the administrative JWT, RF12), not
	// the auth enforcement on those other two routes (covered by
	// customer_test.go/service_order_test.go instead). A login route is
	// still registered because createTrackingOrder/insertActiveCustomer
	// authenticate anyway via loginAsAdmin, matching the shape every other
	// router in this package uses.
	tokens := token.NewManager("integration-test-secret", time.Hour)
	authHandler := auth.NewHandler(auth.NewService(auth.NewRepository(pool), tokens))

	router := http.NewServeMux()
	router.HandleFunc("POST /api/v1/auth/login", authHandler.Login)
	customer.RegisterRoutes(router, customerService, nil)
	serviceorder.RegisterRoutes(router, serviceOrderService, nil)
	servicetracking.RegisterRoutes(router, trackingService)

	server := httptest.NewServer(router)
	t.Cleanup(server.Close)

	return pool, server.URL
}

// createTrackingOrder creates a service order through the real API and
// returns both its admin response and the raw tracking token issued at
// creation (requirements.md §0 item 1).
func createTrackingOrder(t *testing.T, server string, pool *pgxpool.Pool) serviceorder.Response {
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
	require.NotEmpty(t, created.TrackingToken)
	return created
}

// doTracking issues GET /api/v1/acompanhamento/{code}, setting the
// X-Tracking-Token header only when trackingToken is non-empty
// (requirements.md §0 item 2).
func doTracking(t *testing.T, server string, code int64, trackingToken string) *http.Response {
	t.Helper()

	request, err := http.NewRequest(http.MethodGet, server+"/api/v1/acompanhamento/"+strconv.FormatInt(code, 10), nil)
	require.NoError(t, err)
	if trackingToken != "" {
		request.Header.Set("X-Tracking-Token", trackingToken)
	}

	response, err := http.DefaultClient.Do(request)
	require.NoError(t, err)
	return response
}

// TestTrackingValidAccess covers AC1 (valid access) and AC6-AC8 (response
// shape and absence of sensitive/administrative data).
func TestTrackingValidAccess(t *testing.T) {
	pool, server := testTrackingServer(t)
	order := createTrackingOrder(t, server, pool)

	resp := doTracking(t, server, order.Code, order.TrackingToken)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	body := readBodyString(t, resp)

	var tracked struct {
		Code    int64  `json:"code"`
		Status  string `json:"status"`
		Vehicle struct {
			LicensePlate string `json:"licensePlate"`
			Brand        string `json:"brand"`
			Model        string `json:"model"`
			Year         int    `json:"year"`
			Color        string `json:"color"`
		} `json:"vehicle"`
		Milestones []struct {
			Event          string    `json:"event"`
			PreviousStatus string    `json:"previousStatus"`
			NewStatus      string    `json:"newStatus"`
			OccurredAt     time.Time `json:"occurredAt"`
		} `json:"milestones"`
	}
	decodeJSONString(t, body, &tracked)

	assert.Equal(t, order.Code, tracked.Code)
	assert.Equal(t, "RECEIVED", tracked.Status)
	assert.Equal(t, order.Vehicle.LicensePlate, tracked.Vehicle.LicensePlate)
	assert.Equal(t, "Fiat", tracked.Vehicle.Brand)
	require.Len(t, tracked.Milestones, 1)
	assert.Equal(t, "creation", tracked.Milestones[0].Event)

	// AC7/AC8: no PII or administrative data anywhere in the raw body.
	assert.NotContains(t, body, "document")
	assert.NotContains(t, body, "phone")
	assert.NotContains(t, body, "email")
	assert.NotContains(t, body, "customer")
	assert.NotContains(t, body, "notes")
	assert.NotContains(t, body, "description")
	assert.NotContains(t, body, order.ID) // no internal order id leaked
}

// TestTrackingMissingToken covers AC2: the code alone grants no access.
func TestTrackingMissingToken(t *testing.T) {
	pool, server := testTrackingServer(t)
	order := createTrackingOrder(t, server, pool)

	resp := doTracking(t, server, order.Code, "")
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

// TestTrackingWrongToken covers AC2/AC4: a garbled/incorrect token.
func TestTrackingWrongToken(t *testing.T) {
	pool, server := testTrackingServer(t)
	order := createTrackingOrder(t, server, pool)

	resp := doTracking(t, server, order.Code, "not-the-real-token")
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

// TestTrackingUnknownCode covers AC3.
func TestTrackingUnknownCode(t *testing.T) {
	_, server := testTrackingServer(t)

	resp := doTracking(t, server, 987654321, "any-token")
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

// TestTrackingCrossOrderToken covers AC5: order B's token must not unlock
// order A.
func TestTrackingCrossOrderToken(t *testing.T) {
	pool, server := testTrackingServer(t)
	orderA := createTrackingOrder(t, server, pool)
	orderB := createTrackingOrder(t, server, pool)

	resp := doTracking(t, server, orderA.Code, orderB.TrackingToken)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

// TestTrackingRevokedToken covers AC4 (revoked token) — there is no revoke
// endpoint in scope (AUTO-1, requirements.md §3.3), so this test revokes the
// token directly at the data layer to exercise the validator's handling of
// revoked_at.
func TestTrackingRevokedToken(t *testing.T) {
	pool, server := testTrackingServer(t)
	order := createTrackingOrder(t, server, pool)

	_, err := pool.Exec(context.Background(),
		`UPDATE service_order_tracking_tokens SET revoked_at = now() WHERE service_order_id = $1`,
		order.ID,
	)
	require.NoError(t, err)

	resp := doTracking(t, server, order.Code, order.TrackingToken)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

// TestTrackingDoesNotRequireAdminJWT covers AC10: no Authorization header is
// ever sent by doTracking, and access still succeeds with a valid tracking
// token — proving this route doesn't require the administrative JWT.
func TestTrackingDoesNotRequireAdminJWT(t *testing.T) {
	pool, server := testTrackingServer(t)
	order := createTrackingOrder(t, server, pool)

	resp := doTracking(t, server, order.Code, order.TrackingToken)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}
