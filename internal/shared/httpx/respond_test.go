package httpx

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
)

func TestJSONWritesStatusAndBody(t *testing.T) {
	rec := httptest.NewRecorder()
	JSON(rec, 201, map[string]string{"hello": "world"})

	if rec.Code != 201 {
		t.Fatalf("status = %d, want 201", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("content-type = %q, want application/json", ct)
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON body: %v", err)
	}
	if body["hello"] != "world" {
		t.Fatalf("body = %v, want hello=world", body)
	}
}

func TestErrorUsesStandardEnvelope(t *testing.T) {
	rec := httptest.NewRecorder()
	Error(rec, 401, "UNAUTHORIZED", "invalid credentials")

	if rec.Code != 401 {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
	var body struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON body: %v", err)
	}
	if body.Error.Code != "UNAUTHORIZED" || body.Error.Message != "invalid credentials" {
		t.Fatalf("body = %+v, want UNAUTHORIZED/invalid credentials", body)
	}
}
