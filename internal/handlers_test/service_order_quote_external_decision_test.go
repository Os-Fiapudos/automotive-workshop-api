package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testExternalNotificationSecret is the shared secret testServiceOrderServer
// configures for the external quote-decision channel
// (specs/service-order-quote-decision/requirements.md §9).
const testExternalNotificationSecret = "integration-external-notification-secret"

// doExternalDecision issues POST .../quote/decision with the given secret
// (header omitted when empty) and decision body. Never sets Authorization:
// the route must work without the administrative JWT.
func doExternalDecision(t *testing.T, server, orderID, secret, decision string) *http.Response {
	t.Helper()

	payload, err := json.Marshal(map[string]string{"decision": decision})
	require.NoError(t, err)
	request, err := http.NewRequest(http.MethodPost, server+"/api/v1/service-orders/"+orderID+"/quote/decision", bytes.NewReader(payload))
	require.NoError(t, err)
	request.Header.Set("Content-Type", "application/json")
	if secret != "" {
		request.Header.Set("X-External-Notification-Secret", secret)
	}

	response, err := http.DefaultClient.Do(request)
	require.NoError(t, err)
	return response
}

// decisionState reads back everything a decision could touch, so tests can
// assert a rejected attempt left zero side effects.
type decisionState struct {
	OrderStatus  string
	QuoteStatus  string
	RespondedAt  *string
	HistoryCount int
}

func readDecisionState(t *testing.T, pool *pgxpool.Pool, orderID string) decisionState {
	t.Helper()
	ctx := context.Background()

	var state decisionState
	require.NoError(t, pool.QueryRow(ctx, `SELECT status FROM service_orders WHERE id = $1`, orderID).Scan(&state.OrderStatus))
	require.NoError(t, pool.QueryRow(ctx, `SELECT status, responded_at::text FROM quotes WHERE service_order_id = $1`, orderID).Scan(&state.QuoteStatus, &state.RespondedAt))
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM service_order_history WHERE service_order_id = $1`, orderID).Scan(&state.HistoryCount))
	return state
}

func countHistoryEvents(t *testing.T, pool *pgxpool.Pool, orderID, event string) int {
	t.Helper()
	var count int
	err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM service_order_history WHERE service_order_id = $1 AND event = $2`, orderID, event,
	).Scan(&count)
	require.NoError(t, err)
	return count
}

// AC15
func TestExternalQuoteDecisionApproveFullFlow(t *testing.T) {
	pool, server, authToken := testServiceOrderServer(t)
	order := createOrderWithSentQuote(t, server, pool, authToken)

	resp := doExternalDecision(t, server, order.ID, testExternalNotificationSecret, "approved")
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var body struct {
		Code        int64   `json:"code"`
		QuoteStatus string  `json:"quoteStatus"`
		OrderStatus string  `json:"orderStatus"`
		RespondedAt *string `json:"respondedAt"`
	}
	decodeBody(t, resp, &body)
	assert.Equal(t, order.Code, body.Code)
	assert.Equal(t, "APPROVED", body.QuoteStatus)
	assert.Equal(t, "IN_PROGRESS", body.OrderStatus)
	assert.NotNil(t, body.RespondedAt)

	state := readDecisionState(t, pool, order.ID)
	assert.Equal(t, "IN_PROGRESS", state.OrderStatus)
	assert.Equal(t, "APPROVED", state.QuoteStatus)
	require.NotNil(t, state.RespondedAt)

	var previousStatus, newStatus string
	err := pool.QueryRow(context.Background(),
		`SELECT previous_status, new_status FROM service_order_history WHERE service_order_id = $1 AND event = 'approval'`,
		order.ID,
	).Scan(&previousStatus, &newStatus)
	require.NoError(t, err)
	assert.Equal(t, "AWAITING_APPROVAL", previousStatus)
	assert.Equal(t, "IN_PROGRESS", newStatus)
}

// AC16
func TestExternalQuoteDecisionRejectFullFlow(t *testing.T) {
	pool, server, authToken := testServiceOrderServer(t)
	order := createOrderWithSentQuote(t, server, pool, authToken)

	resp := doExternalDecision(t, server, order.ID, testExternalNotificationSecret, "rejected")
	require.Equal(t, http.StatusOK, resp.StatusCode)

	state := readDecisionState(t, pool, order.ID)
	assert.Equal(t, "CANCELED", state.OrderStatus)
	assert.Equal(t, "REJECTED", state.QuoteStatus)
	require.NotNil(t, state.RespondedAt)

	var previousStatus, newStatus string
	err := pool.QueryRow(context.Background(),
		`SELECT previous_status, new_status FROM service_order_history WHERE service_order_id = $1 AND event = 'cancellation'`,
		order.ID,
	).Scan(&previousStatus, &newStatus)
	require.NoError(t, err)
	assert.Equal(t, "AWAITING_APPROVAL", previousStatus)
	assert.Equal(t, "CANCELED", newStatus)
}

// AC17: missing/wrong secret → 401, nothing changes. The admin JWT is not an
// accepted substitute for the secret either.
func TestExternalQuoteDecisionRejectsMissingOrWrongSecret(t *testing.T) {
	pool, server, authToken := testServiceOrderServer(t)
	order := createOrderWithSentQuote(t, server, pool, authToken)
	before := readDecisionState(t, pool, order.ID)

	for _, secret := range []string{"", "wrong-secret"} {
		resp := doExternalDecision(t, server, order.ID, secret, "approved")
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
		assert.Equal(t, "INVALID_EXTERNAL_SECRET", errorCode(t, resp))
	}

	resp := doAuthJSON(t, http.MethodPost, server+"/api/v1/service-orders/"+order.ID+"/quote/decision",
		map[string]string{"decision": "approved"}, authToken)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode, "the administrative JWT must not replace the external secret")

	assert.Equal(t, before, readDecisionState(t, pool, order.ID), "a rejected secret must leave zero side effects")
}

// A wrong secret is 401 even for an unknown id — the caller learns nothing
// about which orders exist (requirements.md §9 item 5).
func TestExternalQuoteDecisionWrongSecretUnknownOrder(t *testing.T) {
	_, server, _ := testServiceOrderServer(t)

	resp := doExternalDecision(t, server, "00000000-0000-0000-0000-000000000000", "wrong-secret", "approved")
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)

	resp = doExternalDecision(t, server, "00000000-0000-0000-0000-000000000000", testExternalNotificationSecret, "approved")
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

// AC18
func TestExternalQuoteDecisionRejectsInvalidDecision(t *testing.T) {
	pool, server, authToken := testServiceOrderServer(t)
	order := createOrderWithSentQuote(t, server, pool, authToken)
	before := readDecisionState(t, pool, order.ID)

	resp := doExternalDecision(t, server, order.ID, testExternalNotificationSecret, "maybe")
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	assert.Equal(t, "VALIDATION_ERROR", errorCode(t, resp))

	assert.Equal(t, before, readDecisionState(t, pool, order.ID))
}

// The "quote sent" precondition is preserved: a composed-but-unsent quote
// (order still IN_DIAGNOSIS) cannot be decided externally.
func TestExternalQuoteDecisionRequiresSentQuote(t *testing.T) {
	pool, server, authToken := testServiceOrderServer(t)
	order := createServiceOrder(t, server, pool)
	productID := insertProduct(t, pool, 10, true)

	resp := doAuthJSON(t, http.MethodPost, server+"/api/v1/service-orders/"+order.ID+"/diagnosis", nil, authToken)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	body := map[string]any{"items": []map[string]any{{"productId": productID, "quantity": 1}}}
	resp = doAuthJSON(t, http.MethodPut, server+"/api/v1/service-orders/"+order.ID+"/quote", body, authToken)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	before := readDecisionState(t, pool, order.ID)

	resp = doExternalDecision(t, server, order.ID, testExternalNotificationSecret, "approved")
	assert.Equal(t, http.StatusConflict, resp.StatusCode)
	assert.Equal(t, "INVALID_STATUS_TRANSITION", errorCode(t, resp))
	assert.Equal(t, before, readDecisionState(t, pool, order.ID))
}

// AC19: a repeated decision (same or different) is a 409 with no further
// change and no duplicated history.
func TestExternalQuoteDecisionRepeatConflict(t *testing.T) {
	pool, server, authToken := testServiceOrderServer(t)
	order := createOrderWithSentQuote(t, server, pool, authToken)

	resp := doExternalDecision(t, server, order.ID, testExternalNotificationSecret, "approved")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	after := readDecisionState(t, pool, order.ID)

	for _, decision := range []string{"approved", "rejected"} {
		resp = doExternalDecision(t, server, order.ID, testExternalNotificationSecret, decision)
		assert.Equal(t, http.StatusConflict, resp.StatusCode)
		assert.Equal(t, "QUOTE_ALREADY_DECIDED", errorCode(t, resp))
	}

	assert.Equal(t, after, readDecisionState(t, pool, order.ID), "repeated decisions must not alter state or add history")
	assert.Equal(t, 1, countHistoryEvents(t, pool, order.ID, "approval"))
	assert.Zero(t, countHistoryEvents(t, pool, order.ID, "cancellation"))
}

// AC19 across channels: a decision taken by the customer (tracking token)
// cannot be overridden by the external channel, and vice versa.
func TestExternalQuoteDecisionConflictsAcrossChannels(t *testing.T) {
	pool, server, authToken := testServiceOrderServer(t)

	customerFirst := createOrderWithSentQuote(t, server, pool, authToken)
	resp := doTrackingPost(t, server, orcamentoPath(customerFirst.Code, "aprovar"), customerFirst.TrackingToken)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	resp = doExternalDecision(t, server, customerFirst.ID, testExternalNotificationSecret, "rejected")
	assert.Equal(t, http.StatusConflict, resp.StatusCode)
	assert.Equal(t, "IN_PROGRESS", readDecisionState(t, pool, customerFirst.ID).OrderStatus)

	externalFirst := createOrderWithSentQuote(t, server, pool, authToken)
	resp = doExternalDecision(t, server, externalFirst.ID, testExternalNotificationSecret, "rejected")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	resp = doTrackingPost(t, server, orcamentoPath(externalFirst.Code, "aprovar"), externalFirst.TrackingToken)
	assert.Equal(t, http.StatusConflict, resp.StatusCode)
	assert.Equal(t, "CANCELED", readDecisionState(t, pool, externalFirst.ID).OrderStatus)
	assert.Zero(t, countHistoryEvents(t, pool, externalFirst.ID, "approval"))
}

// Concurrent decisions race on the guarded UPDATEs inside DecideQuote's
// single transaction (design.md §1.5): exactly one wins, every other
// attempt gets 409 and is rolled back — one history entry, never two.
func TestExternalQuoteDecisionConcurrentDecisions(t *testing.T) {
	pool, server, authToken := testServiceOrderServer(t)
	order := createOrderWithSentQuote(t, server, pool, authToken)

	const attempts = 6
	statuses := make([]int, attempts)
	var wg sync.WaitGroup
	for i := range attempts {
		wg.Add(1)
		go func() {
			defer wg.Done()
			decision := "approved"
			if i%2 == 1 {
				decision = "rejected"
			}
			payload, _ := json.Marshal(map[string]string{"decision": decision})
			request, err := http.NewRequest(http.MethodPost, server+"/api/v1/service-orders/"+order.ID+"/quote/decision", bytes.NewReader(payload))
			if err != nil {
				return
			}
			request.Header.Set("X-External-Notification-Secret", testExternalNotificationSecret)
			response, err := http.DefaultClient.Do(request)
			if err != nil {
				return
			}
			defer response.Body.Close()
			statuses[i] = response.StatusCode
		}()
	}
	wg.Wait()

	successes, conflicts := 0, 0
	for _, status := range statuses {
		switch status {
		case http.StatusOK:
			successes++
		case http.StatusConflict:
			conflicts++
		}
	}
	assert.Equal(t, 1, successes, "exactly one concurrent decision must win: %v", statuses)
	assert.Equal(t, attempts-1, conflicts, "every other concurrent decision must be a 409: %v", statuses)

	decisionEvents := countHistoryEvents(t, pool, order.ID, "approval") + countHistoryEvents(t, pool, order.ID, "cancellation")
	assert.Equal(t, 1, decisionEvents, "a single decision history entry, never duplicated")

	state := readDecisionState(t, pool, order.ID)
	switch state.QuoteStatus {
	case "APPROVED":
		assert.Equal(t, "IN_PROGRESS", state.OrderStatus)
	case "REJECTED":
		assert.Equal(t, "CANCELED", state.OrderStatus)
	default:
		t.Fatalf("unexpected quote status %q", state.QuoteStatus)
	}
}
