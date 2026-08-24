package handlers_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"automotive-workshop-api/internal/features/auth"
	servicecatalog "automotive-workshop-api/internal/features/service-catalog"
	"automotive-workshop-api/internal/shared/database"
	"automotive-workshop-api/internal/shared/middleware"
	"automotive-workshop-api/internal/shared/token"

	"github.com/jackc/pgx/v5/pgxpool"
)

// codeCounter keeps the generated codes unique within a single test binary run,
// since two calls can land on the same nanosecond.
var codeCounter atomic.Int64

// newCatalogMux mirrors the wiring in cmd/api/main.go for the auth login route
// (needed to obtain a real token) and the five catalog routes, and also returns
// the pool so tests can clean up the rows they create.
func newCatalogMux(t *testing.T) (*http.ServeMux, *pgxpool.Pool) {
	t.Helper()
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("DATABASE_URL not set; skipping integration tests")
	}
	pool, err := database.NewPool(context.Background(), url)
	if err != nil {
		t.Fatalf("database connect: %v", err)
	}
	t.Cleanup(pool.Close)

	tokens := token.NewManager(testSecret, time.Hour)
	authHandler := auth.NewHandler(auth.NewService(auth.NewRepository(pool), tokens))
	catalogHandler := servicecatalog.NewHandler(servicecatalog.NewCatalog(servicecatalog.NewRepository(pool)))
	requireAuth := middleware.RequireAuth(tokens)

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/auth/login", authHandler.Login)
	mux.Handle("POST /api/v1/services", requireAuth(http.HandlerFunc(catalogHandler.Create)))
	mux.Handle("GET /api/v1/services", requireAuth(http.HandlerFunc(catalogHandler.List)))
	mux.Handle("GET /api/v1/services/{id}", requireAuth(http.HandlerFunc(catalogHandler.Get)))
	mux.Handle("PATCH /api/v1/services/{id}", requireAuth(http.HandlerFunc(catalogHandler.Update)))
	mux.Handle("DELETE /api/v1/services/{id}", requireAuth(http.HandlerFunc(catalogHandler.Delete)))
	return mux, pool
}

func request(t *testing.T, mux *http.ServeMux, method, target, bearer, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	var req *http.Request
	if body == "" {
		req = httptest.NewRequest(method, target, nil)
	} else {
		req = httptest.NewRequest(method, target, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	mux.ServeHTTP(rec, req)
	return rec
}

type catalogService struct {
	ID            string  `json:"id"`
	Code          int64   `json:"code"`
	Name          string  `json:"name"`
	Description   string  `json:"description"`
	Price         float64 `json:"price"`
	EstimatedTime *int    `json:"estimated_time"`
	Active        bool    `json:"active"`
}

func decodeCatalogService(t *testing.T, rec *httptest.ResponseRecorder) catalogService {
	t.Helper()
	var body catalogService
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON: %v; body: %s", err, rec.Body.String())
	}
	return body
}

func uniqueCode() int64 {
	return time.Now().UnixNano() + codeCounter.Add(1)
}

// createService registers a service and schedules its removal, so the catalog is
// left as the test found it (logical deletion would otherwise keep the row).
func createService(t *testing.T, mux *http.ServeMux, pool *pgxpool.Pool, bearer, payload string) catalogService {
	t.Helper()
	rec := request(t, mux, "POST", "/api/v1/services", bearer, payload)
	if rec.Code != 201 {
		t.Fatalf("create status = %d, body: %s", rec.Code, rec.Body.String())
	}
	created := decodeCatalogService(t, rec)
	t.Cleanup(func() {
		if _, err := pool.Exec(context.Background(), "DELETE FROM services WHERE id = $1", created.ID); err != nil {
			t.Errorf("cleanup of service %s failed: %v", created.ID, err)
		}
	})
	return created
}

// AC8: every catalog route rejects an unauthenticated request.
func TestCatalogRoutesRequireAuthentication(t *testing.T) {
	mux, _ := newCatalogMux(t)
	id := "d0000000-0000-0000-0000-000000000001"
	requests := []struct{ method, target, body string }{
		{"POST", "/api/v1/services", `{"name":"Oil Change","price":10}`},
		{"GET", "/api/v1/services", ""},
		{"GET", "/api/v1/services/" + id, ""},
		{"PATCH", "/api/v1/services/" + id, `{"price":10}`},
		{"DELETE", "/api/v1/services/" + id, ""},
	}
	for _, r := range requests {
		if rec := request(t, mux, r.method, r.target, "", r.body); rec.Code != 401 {
			t.Fatalf("%s %s without token: status = %d, want 401", r.method, r.target, rec.Code)
		}
		if rec := request(t, mux, r.method, r.target, "this.is.garbage", r.body); rec.Code != 401 {
			t.Fatalf("%s %s with bad token: status = %d, want 401", r.method, r.target, rec.Code)
		}
	}
}

// AC1 + FR3: a registered service is persisted and retrievable by id.
func TestCreateAndGetService(t *testing.T) {
	mux, pool := newCatalogMux(t)
	bearer := loginToken(t, mux)
	code := uniqueCode()

	created := createService(t, mux, pool, bearer,
		fmt.Sprintf(`{"code":%d,"name":"Integration Oil Change","description":"d","price":150.5,"estimated_time":30}`, code))
	if created.Code != code || created.Price != 150.5 || *created.EstimatedTime != 30 || !created.Active {
		t.Fatalf("created = %+v", created)
	}

	rec := request(t, mux, "GET", "/api/v1/services/"+created.ID, bearer, "")
	if rec.Code != 200 {
		t.Fatalf("get status = %d, body: %s", rec.Code, rec.Body.String())
	}
	// EstimatedTime is a pointer, so the structs are compared field by field.
	fetched := decodeCatalogService(t, rec)
	if fetched.ID != created.ID || fetched.Code != created.Code || fetched.Name != created.Name ||
		fetched.Description != created.Description || fetched.Price != created.Price ||
		fetched.Active != created.Active || *fetched.EstimatedTime != *created.EstimatedTime {
		t.Fatalf("fetched = %+v, want %+v", fetched, created)
	}
}

// D1: omitting the code lets the database generate it.
func TestCreateServiceWithoutCode(t *testing.T) {
	mux, pool := newCatalogMux(t)
	bearer := loginToken(t, mux)

	created := createService(t, mux, pool, bearer, `{"name":"Integration Generated Code","price":60}`)
	if created.Code == 0 {
		t.Fatal("database did not generate a code")
	}
	if created.EstimatedTime != nil {
		t.Fatalf("estimated_time = %d, want null", *created.EstimatedTime)
	}
}

// AC2: a duplicate code is a conflict.
func TestCreateServiceDuplicateCode(t *testing.T) {
	mux, pool := newCatalogMux(t)
	bearer := loginToken(t, mux)
	code := uniqueCode()

	createService(t, mux, pool, bearer, fmt.Sprintf(`{"code":%d,"name":"Integration First","price":10}`, code))

	rec := request(t, mux, "POST", "/api/v1/services", bearer,
		fmt.Sprintf(`{"code":%d,"name":"Integration Second","price":20}`, code))
	if rec.Code != 409 {
		t.Fatalf("status = %d, want 409; body: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "CODE_ALREADY_EXISTS") {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

// AC3 + AC4: invalid price and estimated time never reach the database.
func TestCreateServiceInvalidValues(t *testing.T) {
	mux, _ := newCatalogMux(t)
	bearer := loginToken(t, mux)

	payloads := map[string]string{
		"negative price":          `{"name":"Integration Invalid","price":-1}`,
		"zero estimated time":     `{"name":"Integration Invalid","price":10,"estimated_time":0}`,
		"negative estimated time": `{"name":"Integration Invalid","price":10,"estimated_time":-5}`,
	}
	for name, payload := range payloads {
		rec := request(t, mux, "POST", "/api/v1/services", bearer, payload)
		if rec.Code != 400 {
			t.Fatalf("%s: status = %d, want 400; body: %s", name, rec.Code, rec.Body.String())
		}
	}
}

// AC5: the listing distinguishes active from inactive services.
func TestListDistinguishesActiveFromInactive(t *testing.T) {
	mux, pool := newCatalogMux(t)
	bearer := loginToken(t, mux)

	active := createService(t, mux, pool, bearer,
		fmt.Sprintf(`{"code":%d,"name":"Integration Active","price":10}`, uniqueCode()))
	retired := createService(t, mux, pool, bearer,
		fmt.Sprintf(`{"code":%d,"name":"Integration Retired","price":10}`, uniqueCode()))

	if rec := request(t, mux, "DELETE", "/api/v1/services/"+retired.ID, bearer, ""); rec.Code != 204 {
		t.Fatalf("delete status = %d, want 204", rec.Code)
	}

	list := func(query string) map[string]bool {
		rec := request(t, mux, "GET", "/api/v1/services"+query, bearer, "")
		if rec.Code != 200 {
			t.Fatalf("list%s status = %d, body: %s", query, rec.Code, rec.Body.String())
		}
		var body struct {
			Items []catalogService `json:"items"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("invalid JSON: %v", err)
		}
		states := map[string]bool{}
		for _, item := range body.Items {
			states[item.ID] = item.Active
		}
		return states
	}

	all := list("")
	if activeState, ok := all[active.ID]; !ok || !activeState {
		t.Fatalf("active service missing or not flagged active in the full listing: %v", all[active.ID])
	}
	if retiredState, ok := all[retired.ID]; !ok || retiredState {
		t.Fatalf("retired service missing or still flagged active in the full listing: %v", all[retired.ID])
	}

	onlyActive := list("?active=true")
	if _, present := onlyActive[retired.ID]; present {
		t.Fatal("retired service returned by active=true")
	}
	if _, present := onlyActive[active.ID]; !present {
		t.Fatal("active service missing from active=true")
	}

	onlyInactive := list("?active=false")
	if _, present := onlyInactive[active.ID]; present {
		t.Fatal("active service returned by active=false")
	}
	if _, present := onlyInactive[retired.ID]; !present {
		t.Fatal("retired service missing from active=false")
	}
}

// AC6: description, price, and estimated time are updatable and persisted.
func TestUpdateServicePersists(t *testing.T) {
	mux, pool := newCatalogMux(t)
	bearer := loginToken(t, mux)

	created := createService(t, mux, pool, bearer,
		fmt.Sprintf(`{"code":%d,"name":"Integration Update","description":"before","price":100,"estimated_time":30}`, uniqueCode()))

	rec := request(t, mux, "PATCH", "/api/v1/services/"+created.ID, bearer,
		`{"description":"after","price":222.25,"estimated_time":45}`)
	if rec.Code != 200 {
		t.Fatalf("patch status = %d, body: %s", rec.Code, rec.Body.String())
	}

	rec = request(t, mux, "GET", "/api/v1/services/"+created.ID, bearer, "")
	fetched := decodeCatalogService(t, rec)
	if fetched.Description != "after" || fetched.Price != 222.25 || *fetched.EstimatedTime != 45 {
		t.Fatalf("fetched = %+v", fetched)
	}
	if fetched.Name != created.Name || fetched.Code != created.Code {
		t.Fatalf("untouched fields changed: %+v", fetched)
	}

	if rec := request(t, mux, "PATCH", "/api/v1/services/"+created.ID, bearer, `{"price":-1}`); rec.Code != 400 {
		t.Fatalf("negative price on update: status = %d, want 400", rec.Code)
	}
}

// AC7 + BR7: deletion is logical — the record survives, flagged inactive.
func TestDeleteServiceIsLogical(t *testing.T) {
	mux, pool := newCatalogMux(t)
	bearer := loginToken(t, mux)

	created := createService(t, mux, pool, bearer,
		fmt.Sprintf(`{"code":%d,"name":"Integration Delete","price":10}`, uniqueCode()))

	if rec := request(t, mux, "DELETE", "/api/v1/services/"+created.ID, bearer, ""); rec.Code != 204 {
		t.Fatalf("delete status = %d, want 204", rec.Code)
	}

	rec := request(t, mux, "GET", "/api/v1/services/"+created.ID, bearer, "")
	if rec.Code != 200 {
		t.Fatalf("service disappeared after delete: status = %d", rec.Code)
	}
	if decodeCatalogService(t, rec).Active {
		t.Fatal("service is still active after delete")
	}

	var remaining int
	if err := pool.QueryRow(context.Background(),
		"SELECT count(*) FROM services WHERE id = $1", created.ID).Scan(&remaining); err != nil {
		t.Fatalf("count: %v", err)
	}
	if remaining != 1 {
		t.Fatalf("row count = %d, want 1 (logical deletion must preserve the record)", remaining)
	}
}

func TestGetUnknownServiceReturnsNotFound(t *testing.T) {
	mux, _ := newCatalogMux(t)
	bearer := loginToken(t, mux)
	rec := request(t, mux, "GET", "/api/v1/services/99999999-9999-9999-9999-999999999999", bearer, "")
	if rec.Code != 404 {
		t.Fatalf("status = %d, want 404; body: %s", rec.Code, rec.Body.String())
	}
}
