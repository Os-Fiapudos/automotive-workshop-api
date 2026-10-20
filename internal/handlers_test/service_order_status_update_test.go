package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	serviceorder "automotive-workshop-api/internal/features/service-order"
)

// Integration tests for POST /api/v1/service-orders/{id}/status
// (specs/service-order-status-update/design.md §7).

// testExternalStatusSecret is the shared secret testServiceOrderServer
// configures for the external status-update tool — deliberately different
// from testExternalNotificationSecret (requirements.md BR4).
const testExternalStatusSecret = "integration-external-status-secret"

// doStatusUpdate issues POST .../status with the given secret (header
// omitted when empty) and body. Never sets Authorization: the route must
// work without the administrative JWT.
func doStatusUpdate(t *testing.T, server, orderID, secret string, body map[string]any) *http.Response {
	t.Helper()

	payload, err := json.Marshal(body)
	require.NoError(t, err)
	request, err := http.NewRequest(http.MethodPost, server+"/api/v1/service-orders/"+orderID+"/status", bytes.NewReader(payload))
	require.NoError(t, err)
	request.Header.Set("Content-Type", "application/json")
	if secret != "" {
		request.Header.Set("X-External-Status-Secret", secret)
	}

	response, err := http.DefaultClient.Do(request)
	require.NoError(t, err)
	return response
}

type statusUpdateBody struct {
	ID             string `json:"id"`
	Code           int64  `json:"code"`
	PreviousStatus string `json:"previousStatus"`
	Status         string `json:"status"`
}

type historyRow struct {
	Event, Description, PreviousStatus, NewStatus string
}

// readOrderStatusAndHistory reads back everything a transition could touch,
// so rejected attempts can be asserted to have zero side effects.
func readOrderStatusAndHistory(t *testing.T, pool *pgxpool.Pool, orderID string) (string, []historyRow) {
	t.Helper()
	ctx := context.Background()

	var status string
	require.NoError(t, pool.QueryRow(ctx, `SELECT status FROM service_orders WHERE id = $1`, orderID).Scan(&status))

	rows, err := pool.Query(ctx,
		`SELECT event, description, previous_status, new_status FROM service_order_history
		 WHERE service_order_id = $1 ORDER BY occurred_at, event`, orderID)
	require.NoError(t, err)
	defer rows.Close()
	var history []historyRow
	for rows.Next() {
		var row historyRow
		require.NoError(t, rows.Scan(&row.Event, &row.Description, &row.PreviousStatus, &row.NewStatus))
		history = append(history, row)
	}
	require.NoError(t, rows.Err())
	return status, history
}

func lastHistory(t *testing.T, pool *pgxpool.Pool, orderID, event string) historyRow {
	t.Helper()
	var row historyRow
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT event, description, previous_status, new_status FROM service_order_history
		 WHERE service_order_id = $1 AND event = $2 ORDER BY occurred_at DESC LIMIT 1`, orderID, event,
	).Scan(&row.Event, &row.Description, &row.PreviousStatus, &row.NewStatus))
	return row
}

func assertStatusUpdated(t *testing.T, resp *http.Response, orderCode int64, previous, current string) {
	t.Helper()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var body statusUpdateBody
	decodeBody(t, resp, &body)
	assert.Equal(t, orderCode, body.Code)
	assert.Equal(t, previous, body.PreviousStatus)
	assert.Equal(t, current, body.Status)
}

// TestStatusUpdateFullLifecycle covers AC1, AC2, AC3, AC7: the external tool
// drives IN_DIAGNOSIS, COMPLETED and DELIVERED, while quote send/approval
// and executions keep going through their own channels. Each transition
// writes the same history event as the dedicated endpoint, with the notes.
func TestStatusUpdateFullLifecycle(t *testing.T) {
	pool, server, authToken := testServiceOrderServer(t)
	order := createServiceOrder(t, server, pool)
	serviceID := insertService(t, pool)

	resp := doStatusUpdate(t, server, order.ID, testExternalStatusSecret, map[string]any{"status": "IN_DIAGNOSIS", "notes": "  Car on lift 2  "})
	assertStatusUpdated(t, resp, order.Code, "RECEIVED", "IN_DIAGNOSIS")
	assert.Equal(t, historyRow{"diagnosis_started", "Diagnosis started. Notes: Car on lift 2", "RECEIVED", "IN_DIAGNOSIS"},
		lastHistory(t, pool, order.ID, "diagnosis_started"))

	quoteBody := map[string]any{"items": []map[string]any{{"serviceId": serviceID, "quantity": 1}}}
	resp = doAuthJSON(t, http.MethodPut, server+"/api/v1/service-orders/"+order.ID+"/quote", quoteBody, authToken)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	resp = doAuthJSON(t, http.MethodPost, server+"/api/v1/service-orders/"+order.ID+"/quote/send", nil, authToken)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// Approval cannot be skipped through this channel (BR2).
	for _, target := range []string{"IN_PROGRESS", "COMPLETED"} {
		statusBefore, historyBefore := readOrderStatusAndHistory(t, pool, order.ID)
		resp = doStatusUpdate(t, server, order.ID, testExternalStatusSecret, map[string]any{"status": target})
		assert.Contains(t, []int{http.StatusBadRequest, http.StatusConflict}, resp.StatusCode, target)
		statusAfter, historyAfter := readOrderStatusAndHistory(t, pool, order.ID)
		assert.Equal(t, statusBefore, statusAfter)
		assert.Equal(t, historyBefore, historyAfter)
	}

	resp = doExternalDecision(t, server, order.ID, testExternalNotificationSecret, "approved")
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// A started but unfinished execution blocks COMPLETED exactly like
	// POST /finalize (AC2), with zero side effects.
	startResp := doAuthJSON(t, http.MethodPost, server+"/api/v1/service-orders/"+order.ID+"/executions",
		map[string]any{"serviceId": serviceID}, authToken)
	require.Equal(t, http.StatusCreated, startResp.StatusCode)
	var started serviceorder.ServiceExecutionResponse
	decodeBody(t, startResp, &started)

	statusBefore, historyBefore := readOrderStatusAndHistory(t, pool, order.ID)
	resp = doStatusUpdate(t, server, order.ID, testExternalStatusSecret, map[string]any{"status": "COMPLETED"})
	assert.Equal(t, http.StatusConflict, resp.StatusCode)
	assert.Equal(t, "EXECUTIONS_NOT_COMPLETED", errorCode(t, resp))
	statusAfter, historyAfter := readOrderStatusAndHistory(t, pool, order.ID)
	assert.Equal(t, statusBefore, statusAfter)
	assert.Equal(t, historyBefore, historyAfter)

	resp = doAuthJSON(t, http.MethodPost, server+"/api/v1/service-orders/"+order.ID+"/executions/"+started.ID+"/finish", nil, authToken)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	resp = doStatusUpdate(t, server, order.ID, testExternalStatusSecret, map[string]any{"status": "COMPLETED", "notes": "All services done"})
	assertStatusUpdated(t, resp, order.Code, "IN_PROGRESS", "COMPLETED")
	assert.Equal(t, historyRow{"completion", "Service order finalized. Notes: All services done", "IN_PROGRESS", "COMPLETED"},
		lastHistory(t, pool, order.ID, "completion"))

	resp = doStatusUpdate(t, server, order.ID, testExternalStatusSecret, map[string]any{"status": "DELIVERED"})
	assertStatusUpdated(t, resp, order.Code, "COMPLETED", "DELIVERED")
	assert.Equal(t, historyRow{"delivery", "Vehicle delivered to customer.", "COMPLETED", "DELIVERED"},
		lastHistory(t, pool, order.ID, "delivery"))

	// The resulting history is visible through the administrative query.
	resp = doAuthJSON(t, http.MethodGet, server+"/api/v1/service-orders/"+order.ID, nil, authToken)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var detail struct {
		Status  string `json:"status"`
		History []struct {
			Description string `json:"description"`
		} `json:"history"`
	}
	decodeBody(t, resp, &detail)
	assert.Equal(t, "DELIVERED", detail.Status)
	var descriptions []string
	for _, entry := range detail.History {
		descriptions = append(descriptions, entry.Description)
	}
	assert.Contains(t, descriptions, "Diagnosis started. Notes: Car on lift 2")
}

// TestStatusUpdateRejectsMissingOrWrongSecret covers AC6/AC9: 401 with no
// side effect, for the quote-decision secret and an admin JWT too.
func TestStatusUpdateRejectsMissingOrWrongSecret(t *testing.T) {
	pool, server, authToken := testServiceOrderServer(t)
	order := createServiceOrder(t, server, pool)
	statusBefore, historyBefore := readOrderStatusAndHistory(t, pool, order.ID)

	for _, secret := range []string{"", "wrong-secret", testExternalNotificationSecret} {
		resp := doStatusUpdate(t, server, order.ID, secret, map[string]any{"status": "IN_DIAGNOSIS"})
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
		assert.Equal(t, "INVALID_EXTERNAL_SECRET", errorCode(t, resp))
	}

	resp := doAuthJSON(t, http.MethodPost, server+"/api/v1/service-orders/"+order.ID+"/status", map[string]any{"status": "IN_DIAGNOSIS"}, authToken)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	assert.Equal(t, "INVALID_EXTERNAL_SECRET", errorCode(t, resp))

	// An unknown order with a wrong secret is the same 401, not a 404.
	resp = doStatusUpdate(t, server, "00000000-0000-0000-0000-000000000000", "wrong-secret", map[string]any{"status": "IN_DIAGNOSIS"})
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)

	statusAfter, historyAfter := readOrderStatusAndHistory(t, pool, order.ID)
	assert.Equal(t, statusBefore, statusAfter)
	assert.Equal(t, historyBefore, historyAfter)
}

// TestStatusUpdateRejectsOutOfContract covers AC4/AC8/AC9.
func TestStatusUpdateRejectsOutOfContract(t *testing.T) {
	pool, server, _ := testServiceOrderServer(t)
	order := createServiceOrder(t, server, pool)
	statusBefore, historyBefore := readOrderStatusAndHistory(t, pool, order.ID)

	bodies := []map[string]any{
		{"status": "AWAITING_APPROVAL"},
		{"status": "IN_PROGRESS"},
		{"status": "CANCELED"},
		{"status": "RECEIVED"},
		{"status": "FINISHED"},
		{},
		{"status": "IN_DIAGNOSIS", "notes": strings.Repeat("x", 501)},
	}
	for _, body := range bodies {
		resp := doStatusUpdate(t, server, order.ID, testExternalStatusSecret, body)
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode, body)
		assert.Equal(t, "VALIDATION_ERROR", errorCode(t, resp))
	}

	statusAfter, historyAfter := readOrderStatusAndHistory(t, pool, order.ID)
	assert.Equal(t, statusBefore, statusAfter)
	assert.Equal(t, historyBefore, historyAfter)
}

// TestStatusUpdateIllegalTransition covers AC5/AC9: same 409 as the
// dedicated endpoint, nothing written.
func TestStatusUpdateIllegalTransition(t *testing.T) {
	pool, server, authToken := testServiceOrderServer(t)
	order := createServiceOrder(t, server, pool)
	statusBefore, historyBefore := readOrderStatusAndHistory(t, pool, order.ID)

	for _, target := range []string{"COMPLETED", "DELIVERED"} {
		resp := doStatusUpdate(t, server, order.ID, testExternalStatusSecret, map[string]any{"status": target, "notes": "skip ahead"})
		assert.Equal(t, http.StatusConflict, resp.StatusCode, target)
		assert.Equal(t, "INVALID_STATUS_TRANSITION", errorCode(t, resp))
	}

	// Parity: the dedicated administrative endpoint answers the same way.
	resp := doAuthJSON(t, http.MethodPost, server+"/api/v1/service-orders/"+order.ID+"/deliver", nil, authToken)
	assert.Equal(t, http.StatusConflict, resp.StatusCode)
	assert.Equal(t, "INVALID_STATUS_TRANSITION", errorCode(t, resp))

	// A repeated IN_DIAGNOSIS is a conflict too, after the first succeeds.
	resp = doStatusUpdate(t, server, order.ID, testExternalStatusSecret, map[string]any{"status": "IN_DIAGNOSIS"})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	_, historyAfterFirst := readOrderStatusAndHistory(t, pool, order.ID)
	assert.Len(t, historyAfterFirst, len(historyBefore)+1)
	assert.Equal(t, "RECEIVED", statusBefore)

	resp = doStatusUpdate(t, server, order.ID, testExternalStatusSecret, map[string]any{"status": "IN_DIAGNOSIS"})
	assert.Equal(t, http.StatusConflict, resp.StatusCode)
	_, historyAfterSecond := readOrderStatusAndHistory(t, pool, order.ID)
	assert.Equal(t, historyAfterFirst, historyAfterSecond)
}

func TestStatusUpdateUnknownOrder(t *testing.T) {
	_, server, _ := testServiceOrderServer(t)

	resp := doStatusUpdate(t, server, "00000000-0000-0000-0000-000000000000", testExternalStatusSecret, map[string]any{"status": "IN_DIAGNOSIS"})
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

// TestStatusUpdateAdministrativeEndpointsUnchanged covers AC10: /diagnosis
// still requires the JWT and writes its fixed description.
func TestStatusUpdateAdministrativeEndpointsUnchanged(t *testing.T) {
	pool, server, authToken := testServiceOrderServer(t)
	order := createServiceOrder(t, server, pool)

	request, err := http.NewRequest(http.MethodPost, server+"/api/v1/service-orders/"+order.ID+"/diagnosis", nil)
	require.NoError(t, err)
	request.Header.Set("X-External-Status-Secret", testExternalStatusSecret)
	resp, err := http.DefaultClient.Do(request)
	require.NoError(t, err)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode, "the external secret is not a substitute for the JWT")

	resp = doAuthJSON(t, http.MethodPost, server+"/api/v1/service-orders/"+order.ID+"/diagnosis", nil, authToken)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "Diagnosis started.", lastHistory(t, pool, order.ID, "diagnosis_started").Description)
}
