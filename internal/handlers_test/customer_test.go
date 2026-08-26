// Package handlers_test holds handler/integration tests that exercise the
// real HTTP layer against a real Postgres database (see
// specs/customer-management/design.md §6). Every test in this file skips
// (never fails) when the database is unreachable, so `go test ./...` still
// passes without docker compose running.
package handlers_test

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"automotive-workshop-api/internal/features/auth"
	"automotive-workshop-api/internal/features/customer"
	"automotive-workshop-api/internal/shared/middleware"
	"automotive-workshop-api/internal/shared/token"
)

func testDatabaseURL() string {
	if value := os.Getenv("DATABASE_URL"); value != "" {
		return value
	}
	// Matches docker-compose.yml/.env.example defaults for local development.
	return "postgres://workshop:workshop@localhost:5432/automotive_workshop?sslmode=disable"
}

// testServer builds the real customer HTTP handlers wired to a real
// pgxpool.Pool, and registers cleanup for both. It skips the calling test
// when the database is unreachable. Every customer route requires a JWT
// (RNF02/specs/auth/design.md §7 — see cmd/api/main.go), so this also wires
// a real auth handler/token manager and returns a valid bearer token, same
// pattern as testProductServer (product_test.go) and testServiceOrderServer
// (service_order_test.go).
func testServer(t *testing.T) (*httptest.Server, *pgxpool.Pool, string) {
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

	repository := customer.NewPostgresCustomerRepository(pool)
	service := customer.NewCustomerService(repository)

	router := http.NewServeMux()
	router.HandleFunc("POST /api/v1/auth/login", authHandler.Login)
	customer.RegisterRoutes(router, service, requireAuth)

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

	return server, pool, authBody.AccessToken
}

// cleanupCustomer physically removes a row created by a test, bypassing the
// API's logical-delete-only DELETE endpoint, so repeated test runs never
// accumulate rows or collide on a reused document.
func cleanupCustomer(t *testing.T, pool *pgxpool.Pool, id string) {
	t.Helper()
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM customers WHERE id = $1`, id)
	})
}

// --- independent CPF/CNPJ generators -----------------------------------
//
// Deliberately not calling internal/shared/document here: generating test
// data with the same code that validates it would let a bug in the shared
// algorithm hide itself from these tests (see design.md §6).

func randomDigits(count int) []int {
	digits := make([]int, count)
	for index := range digits {
		digits[index] = rand.Intn(10)
	}
	return digits
}

func modCheckDigit(digits []int, weights []int) int {
	sum := 0
	for index, digit := range digits {
		sum += digit * weights[index]
	}
	remainder := sum % 11
	if remainder < 2 {
		return 0
	}
	return 11 - remainder
}

func randomValidCPF() string {
	base := randomDigits(9)
	firstCheckDigit := modCheckDigit(base, []int{10, 9, 8, 7, 6, 5, 4, 3, 2})
	secondCheckDigit := modCheckDigit(append(append([]int{}, base...), firstCheckDigit), []int{11, 10, 9, 8, 7, 6, 5, 4, 3, 2})
	allDigits := append(append([]int{}, base...), firstCheckDigit, secondCheckDigit)
	return joinDigits(allDigits)
}

func randomValidCNPJ() string {
	base := randomDigits(12)
	firstCheckDigit := modCheckDigit(base, []int{5, 4, 3, 2, 9, 8, 7, 6, 5, 4, 3, 2})
	secondCheckDigit := modCheckDigit(append(append([]int{}, base...), firstCheckDigit), []int{6, 5, 4, 3, 2, 9, 8, 7, 6, 5, 4, 3, 2})
	allDigits := append(append([]int{}, base...), firstCheckDigit, secondCheckDigit)
	return joinDigits(allDigits)
}

func joinDigits(digits []int) string {
	result := ""
	for _, digit := range digits {
		result += fmt.Sprintf("%d", digit)
	}
	return result
}

// --- HTTP helpers ---------------------------------------------------------
//
// doAuthJSON (defined in product_test.go) is used directly here — every
// customer route now requires a bearer token. doJSON stays as a thin
// no-token wrapper for the other test files in this package that still
// build their own deliberately unauthenticated router (e.g.
// testTrackingServer in service_order_tracking_test.go, which passes a nil
// requireAuth to customer.RegisterRoutes/serviceorder.RegisterRoutes to
// focus purely on the tracking route it tests).
func doJSON(t *testing.T, method, url string, body any) *http.Response {
	t.Helper()
	return doAuthJSON(t, method, url, body, "")
}

func decodeBody(t *testing.T, response *http.Response, out any) {
	t.Helper()
	defer response.Body.Close()
	require.NoError(t, json.NewDecoder(response.Body).Decode(out))
}

// --- tests -----------------------------------------------------------

// TestCustomerRoutesRequireAuth guards against VULN-01
// (docs/owasp-vulnerability-and-coverage-report.md): GET
// /api/v1/customers/document/{document} used to return a customer's name,
// phone and e-mail to anyone who could produce a valid-looking CPF/CNPJ,
// with no credential at all. Every customer route must now reject a request
// with no bearer token.
func TestCustomerRoutesRequireAuth(t *testing.T) {
	server, _, _ := testServer(t)

	response := doAuthJSON(t, http.MethodGet, server.URL+"/api/v1/customers/document/00000000000", nil, "")
	assert.Equal(t, http.StatusUnauthorized, response.StatusCode)

	createResponse := doAuthJSON(t, http.MethodPost, server.URL+"/api/v1/customers", customer.CreateRequest{
		Name:     "Should Not Be Created",
		Document: randomValidCPF(),
		Phone:    "+55 11 90000-0000",
	}, "")
	assert.Equal(t, http.StatusUnauthorized, createResponse.StatusCode)
}

func TestCustomerFullCRUDFlow(t *testing.T) {
	server, pool, authToken := testServer(t)

	// Create an individual with a valid CPF.
	createResp := doAuthJSON(t, http.MethodPost, server.URL+"/api/v1/customers", customer.CreateRequest{
		Name:     "Maria Silva",
		Document: randomValidCPF(),
		Phone:    "+55 11 91234-5678",
	}, authToken)
	require.Equal(t, http.StatusCreated, createResp.StatusCode)
	var created customer.Response
	decodeBody(t, createResp, &created)
	cleanupCustomer(t, pool, created.ID)

	assert.Equal(t, "ACTIVE", created.Status)
	assert.Equal(t, "CPF", created.DocumentType)
	assert.NotEmpty(t, created.ID)
	assert.NotZero(t, created.Code)

	// Create a company with a valid CNPJ.
	cnpjResp := doAuthJSON(t, http.MethodPost, server.URL+"/api/v1/customers", customer.CreateRequest{
		Name:     "Oficina Rota Sul Ltda",
		Document: randomValidCNPJ(),
		Phone:    "+55 11 3333-4444",
	}, authToken)
	require.Equal(t, http.StatusCreated, cnpjResp.StatusCode)
	var company customer.Response
	decodeBody(t, cnpjResp, &company)
	cleanupCustomer(t, pool, company.ID)
	assert.Equal(t, "CNPJ", company.DocumentType)

	// Get by id.
	getResp := doAuthJSON(t, http.MethodGet, server.URL+"/api/v1/customers/"+created.ID, nil, authToken)
	require.Equal(t, http.StatusOK, getResp.StatusCode)
	var fetched customer.Response
	decodeBody(t, getResp, &fetched)
	assert.Equal(t, created.ID, fetched.ID)

	// Get by document.
	byDocumentResp := doAuthJSON(t, http.MethodGet, server.URL+"/api/v1/customers/document/"+created.Document, nil, authToken)
	require.Equal(t, http.StatusOK, byDocumentResp.StatusCode)
	var byDocument customer.Response
	decodeBody(t, byDocumentResp, &byDocument)
	assert.Equal(t, created.ID, byDocument.ID)

	// Partial update: only name changes.
	newName := "Maria Silva Santos"
	patchResp := doAuthJSON(t, http.MethodPatch, server.URL+"/api/v1/customers/"+created.ID, customer.UpdateRequest{
		Name: &newName,
	}, authToken)
	require.Equal(t, http.StatusOK, patchResp.StatusCode)
	var updated customer.Response
	decodeBody(t, patchResp, &updated)
	assert.Equal(t, newName, updated.Name)
	assert.Equal(t, created.Phone, updated.Phone)
	assert.Equal(t, created.Document, updated.Document)

	// List includes it.
	listResp := doAuthJSON(t, http.MethodGet, server.URL+"/api/v1/customers?page=1&pageSize=100", nil, authToken)
	require.Equal(t, http.StatusOK, listResp.StatusCode)
	var list customer.ListResponse
	decodeBody(t, listResp, &list)
	assert.Contains(t, ids(list.Data), created.ID)

	// Deactivate (logical delete).
	deactivateResp := doAuthJSON(t, http.MethodDelete, server.URL+"/api/v1/customers/"+created.ID, nil, authToken)
	require.Equal(t, http.StatusOK, deactivateResp.StatusCode)
	var deactivated customer.Response
	decodeBody(t, deactivateResp, &deactivated)
	assert.Equal(t, "INACTIVE", deactivated.Status)

	// Row still physically exists and is still queryable — not a hard delete.
	var stillExists bool
	err := pool.QueryRow(context.Background(),
		`SELECT EXISTS(SELECT 1 FROM customers WHERE id = $1)`, created.ID,
	).Scan(&stillExists)
	require.NoError(t, err)
	assert.True(t, stillExists)

	stillGetResp := doAuthJSON(t, http.MethodGet, server.URL+"/api/v1/customers/"+created.ID, nil, authToken)
	require.Equal(t, http.StatusOK, stillGetResp.StatusCode)
	var stillFetched customer.Response
	decodeBody(t, stillGetResp, &stillFetched)
	assert.Equal(t, "INACTIVE", stillFetched.Status)

	// Deactivating twice is idempotent, not an error.
	againResp := doAuthJSON(t, http.MethodDelete, server.URL+"/api/v1/customers/"+created.ID, nil, authToken)
	require.Equal(t, http.StatusOK, againResp.StatusCode)
}

func ids(customers []customer.Response) []string {
	out := make([]string, len(customers))
	for index, customer := range customers {
		out[index] = customer.ID
	}
	return out
}

func TestCreateRejectsInvalidCPF(t *testing.T) {
	server, _, authToken := testServer(t)

	response := doAuthJSON(t, http.MethodPost, server.URL+"/api/v1/customers", customer.CreateRequest{
		Name:     "Invalid CPF Customer",
		Document: "111.111.111-11",
		Phone:    "+55 11 90000-0000",
	}, authToken)
	assert.Equal(t, http.StatusBadRequest, response.StatusCode)

	var body map[string]any
	decodeBody(t, response, &body)
	errorBody := body["error"].(map[string]any)
	assert.Equal(t, "VALIDATION_ERROR", errorBody["code"])
}

// TestCreateAcceptsAlphanumericCNPJ proves the API accepts the post-July-2026
// Receita Federal alphanumeric CNPJ format end to end (HTTP → service →
// domain → Postgres), not just the legacy numeric one exercised elsewhere in
// this file. Uses Receita Federal's own published example, cross-checked
// against this project's independent CNPJ algorithm in
// internal/shared/document (see cnpj_test.go).
func TestCreateAcceptsAlphanumericCNPJ(t *testing.T) {
	server, pool, authToken := testServer(t)

	response := doAuthJSON(t, http.MethodPost, server.URL+"/api/v1/customers", customer.CreateRequest{
		Name:     "Empresa Alfanumérica Ltda",
		Document: "12.ABC.345/0001-88",
		Phone:    "+55 11 90000-0000",
	}, authToken)
	require.Equal(t, http.StatusCreated, response.StatusCode)

	var created customer.Response
	decodeBody(t, response, &created)
	cleanupCustomer(t, pool, created.ID)

	assert.Equal(t, "12ABC345000188", created.Document)
	assert.Equal(t, "CNPJ", created.DocumentType)

	getResp := doAuthJSON(t, http.MethodGet, server.URL+"/api/v1/customers/document/"+created.Document, nil, authToken)
	require.Equal(t, http.StatusOK, getResp.StatusCode)
	var fetched customer.Response
	decodeBody(t, getResp, &fetched)
	assert.Equal(t, created.ID, fetched.ID)
}

func TestCreateRejectsInvalidCNPJ(t *testing.T) {
	server, _, authToken := testServer(t)

	response := doAuthJSON(t, http.MethodPost, server.URL+"/api/v1/customers", customer.CreateRequest{
		Name:     "Invalid CNPJ Customer",
		Document: "11.111.111/1111-11",
		Phone:    "+55 11 90000-0000",
	}, authToken)
	assert.Equal(t, http.StatusBadRequest, response.StatusCode)
}

func TestCreateRejectsDuplicateDocument(t *testing.T) {
	server, pool, authToken := testServer(t)
	document := randomValidCPF()

	first := doAuthJSON(t, http.MethodPost, server.URL+"/api/v1/customers", customer.CreateRequest{
		Name: "First Customer", Document: document, Phone: "+55 11 90000-0001",
	}, authToken)
	require.Equal(t, http.StatusCreated, first.StatusCode)
	var firstCustomer customer.Response
	decodeBody(t, first, &firstCustomer)
	cleanupCustomer(t, pool, firstCustomer.ID)

	second := doAuthJSON(t, http.MethodPost, server.URL+"/api/v1/customers", customer.CreateRequest{
		Name: "Second Customer", Document: document, Phone: "+55 11 90000-0002",
	}, authToken)
	assert.Equal(t, http.StatusConflict, second.StatusCode)

	var body map[string]any
	decodeBody(t, second, &body)
	errorBody := body["error"].(map[string]any)
	assert.Equal(t, "DUPLICATE_DOCUMENT", errorBody["code"])
}

func TestUpdateRejectsDuplicateDocument(t *testing.T) {
	server, pool, authToken := testServer(t)

	first := doAuthJSON(t, http.MethodPost, server.URL+"/api/v1/customers", customer.CreateRequest{
		Name: "Customer One", Document: randomValidCPF(), Phone: "+55 11 90000-0001",
	}, authToken)
	require.Equal(t, http.StatusCreated, first.StatusCode)
	var firstCustomer customer.Response
	decodeBody(t, first, &firstCustomer)
	cleanupCustomer(t, pool, firstCustomer.ID)

	second := doAuthJSON(t, http.MethodPost, server.URL+"/api/v1/customers", customer.CreateRequest{
		Name: "Customer Two", Document: randomValidCPF(), Phone: "+55 11 90000-0002",
	}, authToken)
	require.Equal(t, http.StatusCreated, second.StatusCode)
	var secondCustomer customer.Response
	decodeBody(t, second, &secondCustomer)
	cleanupCustomer(t, pool, secondCustomer.ID)

	patch := doAuthJSON(t, http.MethodPatch, server.URL+"/api/v1/customers/"+secondCustomer.ID, customer.UpdateRequest{
		Document: &firstCustomer.Document,
	}, authToken)
	assert.Equal(t, http.StatusConflict, patch.StatusCode)
}

// TestCreateRejectsDuplicateEmail guards against the bug found in manual
// testing: a duplicate e-mail (ux_customers_email, a pre-existing database
// invariant — see requirements.md §3.4.1) was being reported as
// "document already belongs to another customer" because the repository
// treated every unique-violation as a document conflict. The response here
// must be a distinct DUPLICATE_EMAIL, not DUPLICATE_DOCUMENT.
func TestCreateRejectsDuplicateEmail(t *testing.T) {
	server, pool, authToken := testServer(t)
	email := fmt.Sprintf("duplicate-%s@example.com", randomValidCPF())

	first := doAuthJSON(t, http.MethodPost, server.URL+"/api/v1/customers", customer.CreateRequest{
		Name: "First Customer", Document: randomValidCPF(), Phone: "+55 11 90000-0001", Email: &email,
	}, authToken)
	require.Equal(t, http.StatusCreated, first.StatusCode)
	var firstCustomer customer.Response
	decodeBody(t, first, &firstCustomer)
	cleanupCustomer(t, pool, firstCustomer.ID)

	second := doAuthJSON(t, http.MethodPost, server.URL+"/api/v1/customers", customer.CreateRequest{
		Name: "Second Customer", Document: randomValidCPF(), Phone: "+55 11 90000-0002", Email: &email,
	}, authToken)
	assert.Equal(t, http.StatusConflict, second.StatusCode)

	var body map[string]any
	decodeBody(t, second, &body)
	errorBody := body["error"].(map[string]any)
	assert.Equal(t, "DUPLICATE_EMAIL", errorBody["code"])
}

func TestUpdateRejectsDuplicateEmail(t *testing.T) {
	server, pool, authToken := testServer(t)
	email := fmt.Sprintf("duplicate-%s@example.com", randomValidCPF())

	first := doAuthJSON(t, http.MethodPost, server.URL+"/api/v1/customers", customer.CreateRequest{
		Name: "Customer One", Document: randomValidCPF(), Phone: "+55 11 90000-0001", Email: &email,
	}, authToken)
	require.Equal(t, http.StatusCreated, first.StatusCode)
	var firstCustomer customer.Response
	decodeBody(t, first, &firstCustomer)
	cleanupCustomer(t, pool, firstCustomer.ID)

	second := doAuthJSON(t, http.MethodPost, server.URL+"/api/v1/customers", customer.CreateRequest{
		Name: "Customer Two", Document: randomValidCPF(), Phone: "+55 11 90000-0002",
	}, authToken)
	require.Equal(t, http.StatusCreated, second.StatusCode)
	var secondCustomer customer.Response
	decodeBody(t, second, &secondCustomer)
	cleanupCustomer(t, pool, secondCustomer.ID)

	patch := doAuthJSON(t, http.MethodPatch, server.URL+"/api/v1/customers/"+secondCustomer.ID, customer.UpdateRequest{
		Email: &email,
	}, authToken)
	assert.Equal(t, http.StatusConflict, patch.StatusCode)

	var body map[string]any
	decodeBody(t, patch, &body)
	errorBody := body["error"].(map[string]any)
	assert.Equal(t, "DUPLICATE_EMAIL", errorBody["code"])
}

func TestGetByIDNotFound(t *testing.T) {
	server, _, authToken := testServer(t)

	response := doAuthJSON(t, http.MethodGet, server.URL+"/api/v1/customers/00000000-0000-0000-0000-000000000000", nil, authToken)
	assert.Equal(t, http.StatusNotFound, response.StatusCode)
}

func TestPagination(t *testing.T) {
	server, pool, authToken := testServer(t)

	const total = 5
	for index := 0; index < total; index++ {
		response := doAuthJSON(t, http.MethodPost, server.URL+"/api/v1/customers", customer.CreateRequest{
			Name: fmt.Sprintf("Pagination Customer %d", index), Document: randomValidCPF(), Phone: "+55 11 90000-0000",
		}, authToken)
		require.Equal(t, http.StatusCreated, response.StatusCode)
		var createdCustomer customer.Response
		decodeBody(t, response, &createdCustomer)
		cleanupCustomer(t, pool, createdCustomer.ID)
	}

	response := doAuthJSON(t, http.MethodGet, server.URL+"/api/v1/customers?page=1&pageSize=2", nil, authToken)
	require.Equal(t, http.StatusOK, response.StatusCode)
	var list customer.ListResponse
	decodeBody(t, response, &list)

	assert.Len(t, list.Data, 2)
	assert.Equal(t, 1, list.Page)
	assert.Equal(t, 2, list.PageSize)
	assert.GreaterOrEqual(t, list.Total, total)
	assert.GreaterOrEqual(t, list.TotalPages, total/2)
}

// TestDatabaseUniqueConstraintCatchesRaceCondition proves the application
// does not rely solely on ExistsByDocument to guarantee uniqueness
// (requirements.md §3.4): a row inserted directly (simulating a concurrent
// request the pre-check could not have seen) still causes the repository's
// Create to fail via the Postgres unique index, mapped to
// ErrDuplicateDocument.
func TestDatabaseUniqueConstraintCatchesRaceCondition(t *testing.T) {
	_, pool, _ := testServer(t)

	document := randomValidCPF()
	concurrentCustomerID := "11111111-1111-1111-1111-111111111111"

	_, err := pool.Exec(context.Background(),
		`INSERT INTO customers (id, name, document, document_type, phone) VALUES ($1, $2, $3, 'CPF', $4)`,
		concurrentCustomerID, "Concurrent Insert", document, "+55 11 90000-0000",
	)
	require.NoError(t, err)
	cleanupCustomer(t, pool, concurrentCustomerID)

	repository := customer.NewPostgresCustomerRepository(pool)
	racingCustomer, err := customer.NewCustomer("Racing Customer", document, "+55 11 90000-0001", nil)
	require.NoError(t, err)

	err = repository.Create(context.Background(), racingCustomer)
	assert.ErrorIs(t, err, customer.ErrDuplicateDocument)
}
