package serviceorder

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Handler tests for POST /api/v1/service-orders/{id}/status
// (specs/service-order-status-update/design.md §4, §7).

const testStatusSecret = "status-test-secret"

// newStatusUpdateMux wires the real routes with a requireAuth that rejects
// everything, proving the route is reachable without an administrative JWT.
func newStatusUpdateMux(repo *fakeRepository) *http.ServeMux {
	service := newTestService(repo).
		WithExternalNotificationSecret(testExternalSecret).
		WithExternalStatusUpdateSecret(testStatusSecret)
	denyAll := func(http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		})
	}
	mux := http.NewServeMux()
	RegisterRoutes(mux, service, denyAll)
	return mux
}

func postStatusUpdate(mux *http.ServeMux, id, secret, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, "/api/v1/service-orders/"+id+"/status", strings.NewReader(body))
	if secret != "" {
		request.Header.Set(externalStatusSecretHeader, secret)
	}
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, request)
	return recorder
}

func TestStatusUpdateSuccessReducedResponse(t *testing.T) {
	repo := newFakeRepository()
	mux := newStatusUpdateMux(repo)
	order := seedOrder(repo, StatusReceived)

	response := postStatusUpdate(mux, order.ID.String(), testStatusSecret, `{"status":"IN_DIAGNOSIS","notes":"lift 2"}`)
	require.Equal(t, http.StatusOK, response.Code)
	assert.JSONEq(t,
		`{"id":"`+order.ID.String()+`","code":`+strconv.FormatInt(order.Code, 10)+`,"previousStatus":"RECEIVED","status":"IN_DIAGNOSIS"}`,
		response.Body.String())
	assert.Equal(t, "Diagnosis started. Notes: lift 2", repo.history[order.ID][0].Description)
}

// TestStatusUpdateUnauthorized covers AC6: missing secret, wrong secret, the
// quote-decision secret, and a bearer token alone are all 401 — for an
// unknown order too, so the check reveals nothing about which ids exist.
func TestStatusUpdateUnauthorized(t *testing.T) {
	repo := newFakeRepository()
	mux := newStatusUpdateMux(repo)
	order := seedOrder(repo, StatusReceived)

	for name, secret := range map[string]string{"missing": "", "wrong": "nope", "quote secret": testExternalSecret} {
		t.Run(name, func(t *testing.T) {
			for _, id := range []string{order.ID.String(), uuid.NewString(), "not-a-uuid"} {
				response := postStatusUpdate(mux, id, secret, `{"status":"IN_DIAGNOSIS"}`)
				assert.Equal(t, http.StatusUnauthorized, response.Code)
				assert.Contains(t, response.Body.String(), "INVALID_EXTERNAL_SECRET")
			}
		})
	}

	request := httptest.NewRequest(http.MethodPost, "/api/v1/service-orders/"+order.ID.String()+"/status", strings.NewReader(`{"status":"IN_DIAGNOSIS"}`))
	request.Header.Set("Authorization", "Bearer some.jwt.token")
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, request)
	assert.Equal(t, http.StatusUnauthorized, recorder.Code)

	assert.Equal(t, StatusReceived, order.Status)
	assert.Empty(t, repo.history[order.ID])
}

func TestStatusUpdateValidation(t *testing.T) {
	cases := map[string]string{
		"awaiting approval": `{"status":"AWAITING_APPROVAL"}`,
		"in progress":       `{"status":"IN_PROGRESS"}`,
		"canceled":          `{"status":"CANCELED"}`,
		"received":          `{"status":"RECEIVED"}`,
		"lowercase":         `{"status":"in_diagnosis"}`,
		"missing":           `{}`,
		"notes too long":    `{"status":"IN_DIAGNOSIS","notes":"` + strings.Repeat("x", 501) + `"}`,
		"invalid json":      `{"status":`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			repo := newFakeRepository()
			mux := newStatusUpdateMux(repo)
			order := seedOrder(repo, StatusReceived)

			response := postStatusUpdate(mux, order.ID.String(), testStatusSecret, body)
			assert.Equal(t, http.StatusBadRequest, response.Code)
			assert.Equal(t, StatusReceived, order.Status)
			assert.Empty(t, repo.history[order.ID])
		})
	}
}

func TestStatusUpdateUnknownOrder(t *testing.T) {
	mux := newStatusUpdateMux(newFakeRepository())

	for _, id := range []string{uuid.NewString(), "not-a-uuid"} {
		response := postStatusUpdate(mux, id, testStatusSecret, `{"status":"IN_DIAGNOSIS"}`)
		assert.Equal(t, http.StatusNotFound, response.Code)
	}
}

// TestStatusUpdateIllegalTransitionIsConflict covers AC5/AC9.
func TestStatusUpdateIllegalTransitionIsConflict(t *testing.T) {
	repo := newFakeRepository()
	mux := newStatusUpdateMux(repo)
	order := seedOrder(repo, StatusReceived)

	response := postStatusUpdate(mux, order.ID.String(), testStatusSecret, `{"status":"DELIVERED","notes":"skip ahead"}`)
	assert.Equal(t, http.StatusConflict, response.Code)
	assert.Contains(t, response.Body.String(), "INVALID_STATUS_TRANSITION")
	assert.Equal(t, StatusReceived, order.Status)
	assert.Empty(t, repo.history[order.ID])
}

func TestStatusUpdatePendingExecutionsIsConflict(t *testing.T) {
	repo := newFakeRepository()
	mux := newStatusUpdateMux(repo)
	order := seedOrder(repo, StatusInProgress)
	serviceID := uuid.New()
	repo.quotes[order.ID] = &Quote{
		ID:             uuid.New(),
		ServiceOrderID: order.ID,
		Status:         QuoteStatusApproved,
		Items:          []QuoteItem{{Kind: QuoteItemService, ServiceID: serviceID, Quantity: 1}},
	}

	response := postStatusUpdate(mux, order.ID.String(), testStatusSecret, `{"status":"COMPLETED"}`)
	assert.Equal(t, http.StatusConflict, response.Code)
	assert.Contains(t, response.Body.String(), "EXECUTIONS_NOT_COMPLETED")
	assert.Equal(t, StatusInProgress, order.Status)
}
