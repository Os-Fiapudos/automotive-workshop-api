// Package handlers_test holds handler/integration tests that exercise the
// real HTTP layer against a real Postgres database (see
// specs/customer-management/design.md §6). Every test in this file skips
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
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"automotive-workshop-api/internal/features/customer"
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
// when the database is unreachable.
func testServer(t *testing.T) (*httptest.Server, *pgxpool.Pool) {
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

	repository := customer.NewPostgresCustomerRepository(pool)
	service := customer.NewCustomerService(repository)

	router := http.NewServeMux()
	customer.RegisterRoutes(router, service)

	server := httptest.NewServer(router)
	t.Cleanup(server.Close)

	return server, pool
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

func doJSON(t *testing.T, method, url string, body any) *http.Response {
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

	response, err := http.DefaultClient.Do(request)
	require.NoError(t, err)
	return response
}

func decodeBody(t *testing.T, response *http.Response, out any) {
	t.Helper()
	defer response.Body.Close()
	require.NoError(t, json.NewDecoder(response.Body).Decode(out))
}

// --- tests -----------------------------------------------------------

func TestCustomerFullCRUDFlow(t *testing.T) {
	server, pool := testServer(t)

	// Create an individual with a valid CPF.
	createResp := doJSON(t, http.MethodPost, server.URL+"/api/v1/customers", customer.CreateRequest{
		Name:     "Maria Silva",
		Document: randomValidCPF(),
		Phone:    "+55 11 91234-5678",
	})
	require.Equal(t, http.StatusCreated, createResp.StatusCode)
	var created customer.Response
	decodeBody(t, createResp, &created)
	cleanupCustomer(t, pool, created.ID)

	assert.Equal(t, "ACTIVE", created.Status)
	assert.Equal(t, "CPF", created.DocumentType)
	assert.NotEmpty(t, created.ID)
	assert.NotZero(t, created.Code)

	// Create a company with a valid CNPJ.
	cnpjResp := doJSON(t, http.MethodPost, server.URL+"/api/v1/customers", customer.CreateRequest{
		Name:     "Oficina Rota Sul Ltda",
		Document: randomValidCNPJ(),
		Phone:    "+55 11 3333-4444",
	})
	require.Equal(t, http.StatusCreated, cnpjResp.StatusCode)
	var company customer.Response
	decodeBody(t, cnpjResp, &company)
	cleanupCustomer(t, pool, company.ID)
	assert.Equal(t, "CNPJ", company.DocumentType)

	// Get by id.
	getResp := doJSON(t, http.MethodGet, server.URL+"/api/v1/customers/"+created.ID, nil)
	require.Equal(t, http.StatusOK, getResp.StatusCode)
	var fetched customer.Response
	decodeBody(t, getResp, &fetched)
	assert.Equal(t, created.ID, fetched.ID)

	// Get by document.
	byDocumentResp := doJSON(t, http.MethodGet, server.URL+"/api/v1/customers/document/"+created.Document, nil)
	require.Equal(t, http.StatusOK, byDocumentResp.StatusCode)
	var byDocument customer.Response
	decodeBody(t, byDocumentResp, &byDocument)
	assert.Equal(t, created.ID, byDocument.ID)

	// Partial update: only name changes.
	newName := "Maria Silva Santos"
	patchResp := doJSON(t, http.MethodPatch, server.URL+"/api/v1/customers/"+created.ID, customer.UpdateRequest{
		Name: &newName,
	})
	require.Equal(t, http.StatusOK, patchResp.StatusCode)
	var updated customer.Response
	decodeBody(t, patchResp, &updated)
	assert.Equal(t, newName, updated.Name)
	assert.Equal(t, created.Phone, updated.Phone)
	assert.Equal(t, created.Document, updated.Document)

	// List includes it.
	listResp := doJSON(t, http.MethodGet, server.URL+"/api/v1/customers?page=1&pageSize=100", nil)
	require.Equal(t, http.StatusOK, listResp.StatusCode)
	var list customer.ListResponse
	decodeBody(t, listResp, &list)
	assert.Contains(t, ids(list.Data), created.ID)

	// Deactivate (logical delete).
	deactivateResp := doJSON(t, http.MethodDelete, server.URL+"/api/v1/customers/"+created.ID, nil)
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

	stillGetResp := doJSON(t, http.MethodGet, server.URL+"/api/v1/customers/"+created.ID, nil)
	require.Equal(t, http.StatusOK, stillGetResp.StatusCode)
	var stillFetched customer.Response
	decodeBody(t, stillGetResp, &stillFetched)
	assert.Equal(t, "INACTIVE", stillFetched.Status)

	// Deactivating twice is idempotent, not an error.
	againResp := doJSON(t, http.MethodDelete, server.URL+"/api/v1/customers/"+created.ID, nil)
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
	server, _ := testServer(t)

	response := doJSON(t, http.MethodPost, server.URL+"/api/v1/customers", customer.CreateRequest{
		Name:     "Invalid CPF Customer",
		Document: "111.111.111-11",
		Phone:    "+55 11 90000-0000",
	})
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
	server, pool := testServer(t)

	response := doJSON(t, http.MethodPost, server.URL+"/api/v1/customers", customer.CreateRequest{
		Name:     "Empresa Alfanumérica Ltda",
		Document: "12.ABC.345/0001-88",
		Phone:    "+55 11 90000-0000",
	})
	require.Equal(t, http.StatusCreated, response.StatusCode)

	var created customer.Response
	decodeBody(t, response, &created)
	cleanupCustomer(t, pool, created.ID)

	assert.Equal(t, "12ABC345000188", created.Document)
	assert.Equal(t, "CNPJ", created.DocumentType)

	getResp := doJSON(t, http.MethodGet, server.URL+"/api/v1/customers/document/"+created.Document, nil)
	require.Equal(t, http.StatusOK, getResp.StatusCode)
	var fetched customer.Response
	decodeBody(t, getResp, &fetched)
	assert.Equal(t, created.ID, fetched.ID)
}

func TestCreateRejectsInvalidCNPJ(t *testing.T) {
	server, _ := testServer(t)

	response := doJSON(t, http.MethodPost, server.URL+"/api/v1/customers", customer.CreateRequest{
		Name:     "Invalid CNPJ Customer",
		Document: "11.111.111/1111-11",
		Phone:    "+55 11 90000-0000",
	})
	assert.Equal(t, http.StatusBadRequest, response.StatusCode)
}

func TestCreateRejectsDuplicateDocument(t *testing.T) {
	server, pool := testServer(t)
	document := randomValidCPF()

	first := doJSON(t, http.MethodPost, server.URL+"/api/v1/customers", customer.CreateRequest{
		Name: "First Customer", Document: document, Phone: "+55 11 90000-0001",
	})
	require.Equal(t, http.StatusCreated, first.StatusCode)
	var firstCustomer customer.Response
	decodeBody(t, first, &firstCustomer)
	cleanupCustomer(t, pool, firstCustomer.ID)

	second := doJSON(t, http.MethodPost, server.URL+"/api/v1/customers", customer.CreateRequest{
		Name: "Second Customer", Document: document, Phone: "+55 11 90000-0002",
	})
	assert.Equal(t, http.StatusConflict, second.StatusCode)

	var body map[string]any
	decodeBody(t, second, &body)
	errorBody := body["error"].(map[string]any)
	assert.Equal(t, "DUPLICATE_DOCUMENT", errorBody["code"])
}

func TestUpdateRejectsDuplicateDocument(t *testing.T) {
	server, pool := testServer(t)

	first := doJSON(t, http.MethodPost, server.URL+"/api/v1/customers", customer.CreateRequest{
		Name: "Customer One", Document: randomValidCPF(), Phone: "+55 11 90000-0001",
	})
	require.Equal(t, http.StatusCreated, first.StatusCode)
	var firstCustomer customer.Response
	decodeBody(t, first, &firstCustomer)
	cleanupCustomer(t, pool, firstCustomer.ID)

	second := doJSON(t, http.MethodPost, server.URL+"/api/v1/customers", customer.CreateRequest{
		Name: "Customer Two", Document: randomValidCPF(), Phone: "+55 11 90000-0002",
	})
	require.Equal(t, http.StatusCreated, second.StatusCode)
	var secondCustomer customer.Response
	decodeBody(t, second, &secondCustomer)
	cleanupCustomer(t, pool, secondCustomer.ID)

	patch := doJSON(t, http.MethodPatch, server.URL+"/api/v1/customers/"+secondCustomer.ID, customer.UpdateRequest{
		Document: &firstCustomer.Document,
	})
	assert.Equal(t, http.StatusConflict, patch.StatusCode)
}

// TestCreateRejectsDuplicateEmail guards against the bug found in manual
// testing: a duplicate e-mail (ux_customers_email, a pre-existing database
// invariant — see requirements.md §3.4.1) was being reported as
// "document already belongs to another customer" because the repository
// treated every unique-violation as a document conflict. The response here
// must be a distinct DUPLICATE_EMAIL, not DUPLICATE_DOCUMENT.
func TestCreateRejectsDuplicateEmail(t *testing.T) {
	server, pool := testServer(t)
	email := fmt.Sprintf("duplicate-%s@example.com", randomValidCPF())

	first := doJSON(t, http.MethodPost, server.URL+"/api/v1/customers", customer.CreateRequest{
		Name: "First Customer", Document: randomValidCPF(), Phone: "+55 11 90000-0001", Email: &email,
	})
	require.Equal(t, http.StatusCreated, first.StatusCode)
	var firstCustomer customer.Response
	decodeBody(t, first, &firstCustomer)
	cleanupCustomer(t, pool, firstCustomer.ID)

	second := doJSON(t, http.MethodPost, server.URL+"/api/v1/customers", customer.CreateRequest{
		Name: "Second Customer", Document: randomValidCPF(), Phone: "+55 11 90000-0002", Email: &email,
	})
	assert.Equal(t, http.StatusConflict, second.StatusCode)

	var body map[string]any
	decodeBody(t, second, &body)
	errorBody := body["error"].(map[string]any)
	assert.Equal(t, "DUPLICATE_EMAIL", errorBody["code"])
}

func TestUpdateRejectsDuplicateEmail(t *testing.T) {
	server, pool := testServer(t)
	email := fmt.Sprintf("duplicate-%s@example.com", randomValidCPF())

	first := doJSON(t, http.MethodPost, server.URL+"/api/v1/customers", customer.CreateRequest{
		Name: "Customer One", Document: randomValidCPF(), Phone: "+55 11 90000-0001", Email: &email,
	})
	require.Equal(t, http.StatusCreated, first.StatusCode)
	var firstCustomer customer.Response
	decodeBody(t, first, &firstCustomer)
	cleanupCustomer(t, pool, firstCustomer.ID)

	second := doJSON(t, http.MethodPost, server.URL+"/api/v1/customers", customer.CreateRequest{
		Name: "Customer Two", Document: randomValidCPF(), Phone: "+55 11 90000-0002",
	})
	require.Equal(t, http.StatusCreated, second.StatusCode)
	var secondCustomer customer.Response
	decodeBody(t, second, &secondCustomer)
	cleanupCustomer(t, pool, secondCustomer.ID)

	patch := doJSON(t, http.MethodPatch, server.URL+"/api/v1/customers/"+secondCustomer.ID, customer.UpdateRequest{
		Email: &email,
	})
	assert.Equal(t, http.StatusConflict, patch.StatusCode)

	var body map[string]any
	decodeBody(t, patch, &body)
	errorBody := body["error"].(map[string]any)
	assert.Equal(t, "DUPLICATE_EMAIL", errorBody["code"])
}

func TestGetByIDNotFound(t *testing.T) {
	server, _ := testServer(t)

	response := doJSON(t, http.MethodGet, server.URL+"/api/v1/customers/00000000-0000-0000-0000-000000000000", nil)
	assert.Equal(t, http.StatusNotFound, response.StatusCode)
}

func TestPagination(t *testing.T) {
	server, pool := testServer(t)

	const total = 5
	for index := 0; index < total; index++ {
		response := doJSON(t, http.MethodPost, server.URL+"/api/v1/customers", customer.CreateRequest{
			Name: fmt.Sprintf("Pagination Customer %d", index), Document: randomValidCPF(), Phone: "+55 11 90000-0000",
		})
		require.Equal(t, http.StatusCreated, response.StatusCode)
		var createdCustomer customer.Response
		decodeBody(t, response, &createdCustomer)
		cleanupCustomer(t, pool, createdCustomer.ID)
	}

	response := doJSON(t, http.MethodGet, server.URL+"/api/v1/customers?page=1&pageSize=2", nil)
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
	server, pool := testServer(t)
	_ = server

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
