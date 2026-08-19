package servicecatalog

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// newTestMux mirrors the route patterns registered in cmd/api/main.go (without
// the auth middleware, which is exercised by its own package's tests).
func newTestMux(store Store) *http.ServeMux {
	handler := NewHandler(NewCatalog(store))
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/services", handler.Create)
	mux.HandleFunc("GET /api/v1/services", handler.List)
	mux.HandleFunc("GET /api/v1/services/{id}", handler.Get)
	mux.HandleFunc("PATCH /api/v1/services/{id}", handler.Update)
	mux.HandleFunc("DELETE /api/v1/services/{id}", handler.Delete)
	return mux
}

func do(mux *http.ServeMux, method, target, body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	var req *http.Request
	if body == "" {
		req = httptest.NewRequest(method, target, nil)
	} else {
		req = httptest.NewRequest(method, target, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	}
	mux.ServeHTTP(rec, req)
	return rec
}

func decodeService(t *testing.T, rec *httptest.ResponseRecorder) serviceResponse {
	t.Helper()
	var body serviceResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON: %v; body: %s", err, rec.Body.String())
	}
	return body
}

func decodeErrorCode(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON: %v; body: %s", err, rec.Body.String())
	}
	return body.Error.Code
}

// AC1: POST with code, name, and price -> 201 with the created service.
func TestCreateHandlerCreated(t *testing.T) {
	mux := newTestMux(newFakeStore())
	rec := do(mux, "POST", "/api/v1/services", `{"code":1001,"name":"Oil Change","description":"d","price":150.5,"estimated_time":30}`)
	if rec.Code != 201 {
		t.Fatalf("status = %d, want 201; body: %s", rec.Code, rec.Body.String())
	}
	body := decodeService(t, rec)
	if body.Code != 1001 || body.Name != "Oil Change" || body.Price != 150.5 || *body.EstimatedTime != 30 || !body.Active {
		t.Fatalf("body = %+v", body)
	}
}

func TestCreateHandlerNullEstimatedTimeInPayload(t *testing.T) {
	mux := newTestMux(newFakeStore())
	rec := do(mux, "POST", "/api/v1/services", `{"name":"Diagnostics","price":60}`)
	if rec.Code != 201 {
		t.Fatalf("status = %d, want 201; body: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"estimated_time":null`) {
		t.Fatalf("estimated_time is not null in body: %s", rec.Body.String())
	}
}

func TestCreateHandlerBadRequests(t *testing.T) {
	cases := map[string]string{
		"malformed JSON":      `{oops`,
		"missing price":       `{"name":"Oil Change"}`,
		"missing name":        `{"price":10}`,
		"negative price":      `{"name":"Oil Change","price":-1}`,
		"zero estimated time": `{"name":"Oil Change","price":10,"estimated_time":0}`,
		"non positive code":   `{"code":0,"name":"Oil Change","price":10}`,
	}
	for name, payload := range cases {
		mux := newTestMux(newFakeStore())
		rec := do(mux, "POST", "/api/v1/services", payload)
		if rec.Code != 400 {
			t.Fatalf("%s: status = %d, want 400; body: %s", name, rec.Code, rec.Body.String())
		}
		if code := decodeErrorCode(t, rec); code != "INVALID_REQUEST" {
			t.Fatalf("%s: error code = %q, want INVALID_REQUEST", name, code)
		}
	}
}

// AC2: duplicate code -> 409.
func TestCreateHandlerDuplicateCode(t *testing.T) {
	mux := newTestMux(newFakeStore(seededService()))
	rec := do(mux, "POST", "/api/v1/services", `{"code":10,"name":"Other","price":10}`)
	if rec.Code != 409 {
		t.Fatalf("status = %d, want 409; body: %s", rec.Code, rec.Body.String())
	}
	if code := decodeErrorCode(t, rec); code != "CODE_ALREADY_EXISTS" {
		t.Fatalf("error code = %q, want CODE_ALREADY_EXISTS", code)
	}
}

// AC5: the listing carries the active flag and honours the filter.
func TestListHandler(t *testing.T) {
	inactive := &Service{ID: "22222222-2222-2222-2222-222222222222", Code: 11, Name: "Retired", Active: false}
	mux := newTestMux(newFakeStore(seededService(), inactive))

	rec := do(mux, "GET", "/api/v1/services", "")
	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	var all listResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &all); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(all.Items) != 2 {
		t.Fatalf("items = %d, want 2", len(all.Items))
	}

	rec = do(mux, "GET", "/api/v1/services?active=false", "")
	var filtered listResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &filtered); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(filtered.Items) != 1 || filtered.Items[0].Active {
		t.Fatalf("filtered items = %+v", filtered.Items)
	}
}

func TestListHandlerEmptyCatalogReturnsEmptyArray(t *testing.T) {
	mux := newTestMux(newFakeStore())
	rec := do(mux, "GET", "/api/v1/services", "")
	if body := strings.TrimSpace(rec.Body.String()); body != `{"items":[]}` {
		t.Fatalf("body = %s, want {\"items\":[]}", body)
	}
}

func TestListHandlerInvalidActiveFilter(t *testing.T) {
	mux := newTestMux(newFakeStore())
	rec := do(mux, "GET", "/api/v1/services?active=maybe", "")
	if rec.Code != 400 {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestGetHandler(t *testing.T) {
	service := seededService()
	mux := newTestMux(newFakeStore(service))

	rec := do(mux, "GET", "/api/v1/services/"+service.ID, "")
	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	if decodeService(t, rec).ID != service.ID {
		t.Fatalf("body = %s", rec.Body.String())
	}

	rec = do(mux, "GET", "/api/v1/services/99999999-9999-9999-9999-999999999999", "")
	if rec.Code != 404 {
		t.Fatalf("unknown id: status = %d, want 404", rec.Code)
	}
	if code := decodeErrorCode(t, rec); code != "SERVICE_NOT_FOUND" {
		t.Fatalf("error code = %q, want SERVICE_NOT_FOUND", code)
	}

	rec = do(mux, "GET", "/api/v1/services/not-a-uuid", "")
	if rec.Code != 400 {
		t.Fatalf("malformed id: status = %d, want 400", rec.Code)
	}
}

// AC6: description, price, and estimated time are updatable.
func TestUpdateHandler(t *testing.T) {
	service := seededService()
	mux := newTestMux(newFakeStore(service))

	rec := do(mux, "PATCH", "/api/v1/services/"+service.ID, `{"description":"Full synthetic.","price":180.5,"estimated_time":45}`)
	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	body := decodeService(t, rec)
	if body.Description != "Full synthetic." || body.Price != 180.5 || *body.EstimatedTime != 45 {
		t.Fatalf("body = %+v", body)
	}
	if body.Name != "Oil Change" {
		t.Fatalf("untouched field changed: name = %q", body.Name)
	}
}

func TestUpdateHandlerBadRequests(t *testing.T) {
	service := seededService()
	cases := map[string]string{
		"malformed JSON": `{oops`,
		"empty payload":  `{}`,
		"negative price": `{"price":-1}`,
	}
	for name, payload := range cases {
		mux := newTestMux(newFakeStore(service))
		rec := do(mux, "PATCH", "/api/v1/services/"+service.ID, payload)
		if rec.Code != 400 {
			t.Fatalf("%s: status = %d, want 400; body: %s", name, rec.Code, rec.Body.String())
		}
	}
}

func TestUpdateHandlerNotFound(t *testing.T) {
	mux := newTestMux(newFakeStore())
	rec := do(mux, "PATCH", "/api/v1/services/99999999-9999-9999-9999-999999999999", `{"price":10}`)
	if rec.Code != 404 {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

// AC7: DELETE deactivates and preserves the record.
func TestDeleteHandlerDeactivates(t *testing.T) {
	service := seededService()
	mux := newTestMux(newFakeStore(service))

	rec := do(mux, "DELETE", "/api/v1/services/"+service.ID, "")
	if rec.Code != 204 {
		t.Fatalf("status = %d, want 204; body: %s", rec.Code, rec.Body.String())
	}
	if rec.Body.Len() != 0 {
		t.Fatalf("204 carries a body: %s", rec.Body.String())
	}

	rec = do(mux, "GET", "/api/v1/services/"+service.ID, "")
	if rec.Code != 200 {
		t.Fatalf("service disappeared after delete: status = %d", rec.Code)
	}
	if decodeService(t, rec).Active {
		t.Fatal("service is still active after delete")
	}
}

func TestDeleteHandlerNotFound(t *testing.T) {
	mux := newTestMux(newFakeStore())
	rec := do(mux, "DELETE", "/api/v1/services/99999999-9999-9999-9999-999999999999", "")
	if rec.Code != 404 {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

// An unexpected failure answers the generic envelope and never leaks the cause.
func TestHandlerInternalErrorDoesNotLeak(t *testing.T) {
	store := newFakeStore(seededService())
	store.failWith = errors.New("db down")
	mux := newTestMux(store)

	requests := []struct{ method, target, body string }{
		{"POST", "/api/v1/services", `{"name":"Oil Change","price":10}`},
		{"GET", "/api/v1/services", ""},
		{"GET", "/api/v1/services/" + seededService().ID, ""},
		{"PATCH", "/api/v1/services/" + seededService().ID, `{"price":10}`},
		{"DELETE", "/api/v1/services/" + seededService().ID, ""},
	}
	for _, request := range requests {
		rec := do(mux, request.method, request.target, request.body)
		if rec.Code != 500 {
			t.Fatalf("%s %s: status = %d, want 500", request.method, request.target, rec.Code)
		}
		if strings.Contains(rec.Body.String(), "db down") {
			t.Fatalf("%s %s: body leaks internal error: %s", request.method, request.target, rec.Body.String())
		}
		if code := decodeErrorCode(t, rec); code != "INTERNAL_ERROR" {
			t.Fatalf("%s %s: error code = %q, want INTERNAL_ERROR", request.method, request.target, code)
		}
	}
}
