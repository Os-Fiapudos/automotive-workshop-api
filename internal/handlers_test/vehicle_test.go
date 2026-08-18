// Package handlers_test holds handler/integration tests that exercise the
// real HTTP layer against a real Postgres database (see
// specs/vehicle-management/design.md §6). Every test in this file skips
// (never fails) when the database is unreachable, so `go test ./...` still
// passes without docker compose running.
package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"automotive-workshop-api/internal/features/customer"
	"automotive-workshop-api/internal/features/vehicle"
	"automotive-workshop-api/internal/shared/middleware"
	"automotive-workshop-api/internal/shared/token"
)

const vehicleTestSecret = "vehicle-integration-test-secret"

// testCustomerLookup adapts *customer.CustomerService to vehicle.CustomerLookup
// for these tests — a test-local duplicate of cmd/api/main.go's
// customerLookupAdapter, since main is not importable (see design.md §1.3).
type testCustomerLookup struct{ service *customer.CustomerService }

func (adapter testCustomerLookup) IsActiveCustomer(ctx context.Context, id uuid.UUID) (bool, bool, error) {
	found, err := adapter.service.Get(ctx, id)
	if err != nil {
		if err == customer.ErrNotFound {
			return false, false, nil
		}
		return false, false, err
	}
	return true, found.IsActive(), nil
}

// vehicleTestServer builds the real vehicle (and underlying customer) HTTP
// wiring against a real pgxpool.Pool, mirroring cmd/api/main.go, and returns
// a ready-to-use "Authorization" header value for a valid JWT. It skips the
// calling test when the database is unreachable (mirrors testServer in
// customer_test.go).
func vehicleTestServer(t *testing.T) (*httptest.Server, *pgxpool.Pool, string) {
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

	customerService := customer.NewCustomerService(customer.NewPostgresCustomerRepository(pool))
	vehicleService := vehicle.NewVehicleService(vehicle.NewPostgresVehicleRepository(pool), testCustomerLookup{service: customerService})

	tokens := token.NewManager(vehicleTestSecret, time.Hour)
	requireAuth := middleware.RequireAuth(tokens)

	router := http.NewServeMux()
	vehicle.RegisterRoutes(router, vehicleService, requireAuth)

	server := httptest.NewServer(router)
	t.Cleanup(server.Close)

	bearer, err := tokens.Generate("integration-test-user")
	require.NoError(t, err)

	return server, pool, "Bearer " + bearer
}

// cleanupVehicle physically removes a row created by a test, bypassing the
// API's logical-delete-only DELETE endpoint, so repeated test runs never
// accumulate rows or collide on a reused license plate.
func cleanupVehicle(t *testing.T, pool *pgxpool.Pool, id string) {
	t.Helper()
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM vehicles WHERE id = $1`, id)
	})
}

// createTestCustomer creates a customer directly through the domain/
// repository (not HTTP) with a random valid CPF, optionally deactivated, and
// registers its cleanup. A precondition helper for vehicle tests, not itself
// what's under test here (see customer_test.go for Customer Management's own
// coverage).
func createTestCustomer(t *testing.T, pool *pgxpool.Pool, active bool) uuid.UUID {
	t.Helper()

	repository := customer.NewPostgresCustomerRepository(pool)
	created, err := customer.NewCustomer("Vehicle Test Customer", randomValidCPF(), "+55 11 90000-0000", nil)
	require.NoError(t, err)
	if !active {
		created.Deactivate()
	}
	require.NoError(t, repository.Create(context.Background(), created))
	cleanupCustomer(t, pool, created.ID.String())
	return created.ID
}

// randomValidPlate builds a structurally valid Mercosul-format plate.
// Deliberately not calling vehicle.NormalizePlate/ValidatePlate here — same
// "don't let the generator and the validator share a bug" rationale
// customer_test.go documents for its own CPF/CNPJ generators.
func randomValidPlate() string {
	const letters = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	pick := func() byte { return letters[rand.Intn(len(letters))] }
	digit := func() byte { return byte('0' + rand.Intn(10)) }
	return string([]byte{pick(), pick(), pick(), digit(), pick(), digit(), digit()})
}

// doWithAuth is doJSON (customer_test.go) plus an Authorization header — every
// vehicle route requires one (RNF02), unlike Customer Management's routes.
func doWithAuth(t *testing.T, method, url, authorization string, body any) *http.Response {
	t.Helper()

	var reader *bytes.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		require.NoError(t, err)
		reader = bytes.NewReader(encoded)
	} else {
		reader = bytes.NewReader(nil)
	}

	request, err := http.NewRequest(method, url, reader)
	require.NoError(t, err)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", authorization)

	response, err := http.DefaultClient.Do(request)
	require.NoError(t, err)
	return response
}

func TestVehicleRoutesRequireAuth(t *testing.T) {
	server, _, _ := vehicleTestServer(t)

	placeholderID := "00000000-0000-0000-0000-000000000000"
	routes := []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/api/v1/vehicles"},
		{http.MethodGet, "/api/v1/vehicles"},
		{http.MethodGet, "/api/v1/vehicles/plate/ABC1234"},
		{http.MethodGet, "/api/v1/vehicles/" + placeholderID},
		{http.MethodGet, "/api/v1/vehicles/customer/" + placeholderID},
		{http.MethodPatch, "/api/v1/vehicles/" + placeholderID},
		{http.MethodDelete, "/api/v1/vehicles/" + placeholderID},
	}

	for _, route := range routes {
		request, err := http.NewRequest(route.method, server.URL+route.path, nil)
		require.NoError(t, err)
		response, err := http.DefaultClient.Do(request)
		require.NoError(t, err)
		assert.Equal(t, http.StatusUnauthorized, response.StatusCode, "%s %s", route.method, route.path)
		response.Body.Close()
	}
}

func TestVehicleFullCRUDFlow(t *testing.T) {
	server, pool, authorization := vehicleTestServer(t)
	customerID := createTestCustomer(t, pool, true)

	createResp := doWithAuth(t, http.MethodPost, server.URL+"/api/v1/vehicles", authorization, vehicle.CreateRequest{
		LicensePlate: randomValidPlate(),
		Brand:        "Fiat",
		Model:        "Uno",
		Year:         2018,
		Color:        "White",
		CustomerID:   customerID.String(),
	})
	require.Equal(t, http.StatusCreated, createResp.StatusCode)
	var created vehicle.Response
	decodeBody(t, createResp, &created)
	cleanupVehicle(t, pool, created.ID)

	assert.Equal(t, "ACTIVE", created.Status)
	assert.NotEmpty(t, created.ID)
	assert.NotZero(t, created.Code)
	assert.Equal(t, customerID.String(), created.CustomerID)

	// Get by id.
	getResp := doWithAuth(t, http.MethodGet, server.URL+"/api/v1/vehicles/"+created.ID, authorization, nil)
	require.Equal(t, http.StatusOK, getResp.StatusCode)
	var fetched vehicle.Response
	decodeBody(t, getResp, &fetched)
	assert.Equal(t, created.ID, fetched.ID)

	// Get by plate.
	byPlateResp := doWithAuth(t, http.MethodGet, server.URL+"/api/v1/vehicles/plate/"+created.LicensePlate, authorization, nil)
	require.Equal(t, http.StatusOK, byPlateResp.StatusCode)
	var byPlate vehicle.Response
	decodeBody(t, byPlateResp, &byPlate)
	assert.Equal(t, created.ID, byPlate.ID)

	// List includes it.
	listResp := doWithAuth(t, http.MethodGet, server.URL+"/api/v1/vehicles?page=1&pageSize=100", authorization, nil)
	require.Equal(t, http.StatusOK, listResp.StatusCode)
	var list vehicle.ListResponse
	decodeBody(t, listResp, &list)
	assert.Contains(t, vehicleIDs(list.Data), created.ID)

	// List by customer includes it.
	byCustomerResp := doWithAuth(t, http.MethodGet, server.URL+"/api/v1/vehicles/customer/"+customerID.String(), authorization, nil)
	require.Equal(t, http.StatusOK, byCustomerResp.StatusCode)
	var byCustomer vehicle.ListResponse
	decodeBody(t, byCustomerResp, &byCustomer)
	assert.Contains(t, vehicleIDs(byCustomer.Data), created.ID)

	// Partial update: brand, model, year, color.
	newColor := "Red"
	patchResp := doWithAuth(t, http.MethodPatch, server.URL+"/api/v1/vehicles/"+created.ID, authorization, vehicle.UpdateRequest{
		Color: &newColor,
	})
	require.Equal(t, http.StatusOK, patchResp.StatusCode)
	var updated vehicle.Response
	decodeBody(t, patchResp, &updated)
	assert.Equal(t, "Red", updated.Color)
	assert.Equal(t, created.Brand, updated.Brand)
	assert.Equal(t, created.LicensePlate, updated.LicensePlate)

	// Deactivate (logical delete).
	deactivateResp := doWithAuth(t, http.MethodDelete, server.URL+"/api/v1/vehicles/"+created.ID, authorization, nil)
	require.Equal(t, http.StatusOK, deactivateResp.StatusCode)
	var deactivated vehicle.Response
	decodeBody(t, deactivateResp, &deactivated)
	assert.Equal(t, "INACTIVE", deactivated.Status)

	// Row still physically exists and is still queryable — not a hard delete.
	var stillExists bool
	err := pool.QueryRow(context.Background(),
		`SELECT EXISTS(SELECT 1 FROM vehicles WHERE id = $1)`, created.ID,
	).Scan(&stillExists)
	require.NoError(t, err)
	assert.True(t, stillExists)

	stillGetResp := doWithAuth(t, http.MethodGet, server.URL+"/api/v1/vehicles/"+created.ID, authorization, nil)
	require.Equal(t, http.StatusOK, stillGetResp.StatusCode)
	var stillFetched vehicle.Response
	decodeBody(t, stillGetResp, &stillFetched)
	assert.Equal(t, "INACTIVE", stillFetched.Status)

	// Deactivating twice is idempotent, not an error.
	againResp := doWithAuth(t, http.MethodDelete, server.URL+"/api/v1/vehicles/"+created.ID, authorization, nil)
	require.Equal(t, http.StatusOK, againResp.StatusCode)
}

func vehicleIDs(vehicles []vehicle.Response) []string {
	out := make([]string, len(vehicles))
	for index, v := range vehicles {
		out[index] = v.ID
	}
	return out
}

func TestVehicleCreateRejectsInvalidPlate(t *testing.T) {
	server, pool, authorization := vehicleTestServer(t)
	customerID := createTestCustomer(t, pool, true)

	response := doWithAuth(t, http.MethodPost, server.URL+"/api/v1/vehicles", authorization, vehicle.CreateRequest{
		LicensePlate: "not-a-plate",
		Brand:        "Fiat", Model: "Uno", Year: 2018, Color: "White",
		CustomerID: customerID.String(),
	})
	assert.Equal(t, http.StatusBadRequest, response.StatusCode)

	var body map[string]any
	decodeBody(t, response, &body)
	errorBody := body["error"].(map[string]any)
	assert.Equal(t, "VALIDATION_ERROR", errorBody["code"])
}

func TestVehicleCreateRejectsYearOutOfRange(t *testing.T) {
	server, pool, authorization := vehicleTestServer(t)
	customerID := createTestCustomer(t, pool, true)

	response := doWithAuth(t, http.MethodPost, server.URL+"/api/v1/vehicles", authorization, vehicle.CreateRequest{
		LicensePlate: randomValidPlate(),
		Brand:        "Fiat", Model: "Uno", Year: 1900, Color: "White",
		CustomerID: customerID.String(),
	})
	assert.Equal(t, http.StatusBadRequest, response.StatusCode)
}

func TestVehicleCreateRejectsDuplicatePlate(t *testing.T) {
	server, pool, authorization := vehicleTestServer(t)
	customerID := createTestCustomer(t, pool, true)
	plate := randomValidPlate()

	first := doWithAuth(t, http.MethodPost, server.URL+"/api/v1/vehicles", authorization, vehicle.CreateRequest{
		LicensePlate: plate, Brand: "Fiat", Model: "Uno", Year: 2018, Color: "White", CustomerID: customerID.String(),
	})
	require.Equal(t, http.StatusCreated, first.StatusCode)
	var firstVehicle vehicle.Response
	decodeBody(t, first, &firstVehicle)
	cleanupVehicle(t, pool, firstVehicle.ID)

	second := doWithAuth(t, http.MethodPost, server.URL+"/api/v1/vehicles", authorization, vehicle.CreateRequest{
		LicensePlate: plate, Brand: "Volkswagen", Model: "Gol", Year: 2020, Color: "Silver", CustomerID: customerID.String(),
	})
	assert.Equal(t, http.StatusConflict, second.StatusCode)

	var body map[string]any
	decodeBody(t, second, &body)
	errorBody := body["error"].(map[string]any)
	assert.Equal(t, "DUPLICATE_LICENSE_PLATE", errorBody["code"])
}

func TestVehicleCreateRejectsNonexistentCustomer(t *testing.T) {
	server, _, authorization := vehicleTestServer(t)

	response := doWithAuth(t, http.MethodPost, server.URL+"/api/v1/vehicles", authorization, vehicle.CreateRequest{
		LicensePlate: randomValidPlate(), Brand: "Fiat", Model: "Uno", Year: 2018, Color: "White",
		CustomerID: uuid.New().String(),
	})
	assert.Equal(t, http.StatusNotFound, response.StatusCode)

	var body map[string]any
	decodeBody(t, response, &body)
	errorBody := body["error"].(map[string]any)
	assert.Equal(t, "CUSTOMER_NOT_FOUND", errorBody["code"])
}

func TestVehicleCreateRejectsInactiveCustomer(t *testing.T) {
	server, pool, authorization := vehicleTestServer(t)
	customerID := createTestCustomer(t, pool, false)

	response := doWithAuth(t, http.MethodPost, server.URL+"/api/v1/vehicles", authorization, vehicle.CreateRequest{
		LicensePlate: randomValidPlate(), Brand: "Fiat", Model: "Uno", Year: 2018, Color: "White",
		CustomerID: customerID.String(),
	})
	assert.Equal(t, http.StatusConflict, response.StatusCode)

	var body map[string]any
	decodeBody(t, response, &body)
	errorBody := body["error"].(map[string]any)
	assert.Equal(t, "CUSTOMER_INACTIVE", errorBody["code"])
}

func TestVehicleGetByIDNotFound(t *testing.T) {
	server, _, authorization := vehicleTestServer(t)

	response := doWithAuth(t, http.MethodGet, server.URL+"/api/v1/vehicles/00000000-0000-0000-0000-000000000000", authorization, nil)
	assert.Equal(t, http.StatusNotFound, response.StatusCode)
}

func TestVehicleListByCustomerNotFound(t *testing.T) {
	server, _, authorization := vehicleTestServer(t)

	response := doWithAuth(t, http.MethodGet, server.URL+"/api/v1/vehicles/customer/00000000-0000-0000-0000-000000000000", authorization, nil)
	assert.Equal(t, http.StatusNotFound, response.StatusCode)
}

func TestVehiclePagination(t *testing.T) {
	server, pool, authorization := vehicleTestServer(t)
	customerID := createTestCustomer(t, pool, true)

	const total = 5
	for index := 0; index < total; index++ {
		response := doWithAuth(t, http.MethodPost, server.URL+"/api/v1/vehicles", authorization, vehicle.CreateRequest{
			LicensePlate: randomValidPlate(),
			Brand:        "Fiat", Model: fmt.Sprintf("Model %d", index), Year: 2018, Color: "White",
			CustomerID: customerID.String(),
		})
		require.Equal(t, http.StatusCreated, response.StatusCode)
		var createdVehicle vehicle.Response
		decodeBody(t, response, &createdVehicle)
		cleanupVehicle(t, pool, createdVehicle.ID)
	}

	response := doWithAuth(t, http.MethodGet, server.URL+"/api/v1/vehicles?page=1&pageSize=2", authorization, nil)
	require.Equal(t, http.StatusOK, response.StatusCode)
	var list vehicle.ListResponse
	decodeBody(t, response, &list)

	assert.Len(t, list.Data, 2)
	assert.Equal(t, 1, list.Page)
	assert.Equal(t, 2, list.PageSize)
	assert.GreaterOrEqual(t, list.Total, total)
}

// TestVehicleDatabaseUniqueConstraintCatchesRaceCondition proves the
// application does not rely solely on ExistsByPlate to guarantee uniqueness
// (requirements.md BR4): a row inserted directly (simulating a concurrent
// request the pre-check could not have seen) still causes the repository's
// Create to fail via the Postgres unique index, mapped to ErrDuplicatePlate.
func TestVehicleDatabaseUniqueConstraintCatchesRaceCondition(t *testing.T) {
	_, pool, _ := vehicleTestServer(t)
	customerID := createTestCustomer(t, pool, true)
	plate := randomValidPlate()

	concurrentVehicleID := uuid.New()
	_, err := pool.Exec(context.Background(),
		`INSERT INTO vehicles (id, license_plate, brand, model, year, color, customer_id) VALUES ($1, $2, 'Fiat', 'Uno', 2018, 'White', $3)`,
		concurrentVehicleID, plate, customerID,
	)
	require.NoError(t, err)
	cleanupVehicle(t, pool, concurrentVehicleID.String())

	repository := vehicle.NewPostgresVehicleRepository(pool)
	racingVehicle, err := vehicle.NewVehicle(plate, "Volkswagen", "Gol", 2020, "Silver", customerID)
	require.NoError(t, err)

	err = repository.Create(context.Background(), racingVehicle)
	assert.ErrorIs(t, err, vehicle.ErrDuplicatePlate)
}
