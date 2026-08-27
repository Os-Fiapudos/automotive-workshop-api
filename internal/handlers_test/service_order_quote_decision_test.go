package handlers_test

import (
	"context"
	"net/http"
	"strconv"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	serviceorder "automotive-workshop-api/internal/features/service-order"
)

// doTrackingPost issues a POST to server+path with no body, setting the
// X-Tracking-Token header only when trackingToken is non-empty — the POST
// counterpart to doTracking in service_order_tracking_test.go, used by the
// customer-facing quote decision endpoints
// (specs/service-order-quote-decision/).
func doTrackingPost(t *testing.T, server, path, trackingToken string) *http.Response {
	t.Helper()

	request, err := http.NewRequest(http.MethodPost, server+path, nil)
	require.NoError(t, err)
	if trackingToken != "" {
		request.Header.Set("X-Tracking-Token", trackingToken)
	}

	response, err := http.DefaultClient.Do(request)
	require.NoError(t, err)
	return response
}

// createOrderWithSentQuote drives an order through diagnosis, quote
// composition, and sending, via the real authenticated API, returning it
// with AWAITING_APPROVAL and a token ready to approve/reject through the
// public /acompanhamento endpoints.
func createOrderWithSentQuote(t *testing.T, server string, pool *pgxpool.Pool, authToken string) serviceorder.Response {
	t.Helper()

	order := createServiceOrder(t, server, pool)
	productID := insertProduct(t, pool, 42.50, true)

	resp := doAuthJSON(t, http.MethodPost, server+"/api/v1/service-orders/"+order.ID+"/diagnosis", nil, authToken)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	body := map[string]any{"items": []map[string]any{{"productId": productID, "quantity": 1}}}
	resp = doAuthJSON(t, http.MethodPut, server+"/api/v1/service-orders/"+order.ID+"/quote", body, authToken)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	resp = doAuthJSON(t, http.MethodPost, server+"/api/v1/service-orders/"+order.ID+"/quote/send", nil, authToken)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	return order
}

func orcamentoPath(code int64, action string) string {
	return "/api/v1/acompanhamento/" + strconv.FormatInt(code, 10) + "/orcamento/" + action
}

// ---- Send ----

func TestQuoteDecisionSendFullFlow(t *testing.T) {
	pool, server, authToken := testServiceOrderServer(t)
	order := createServiceOrder(t, server, pool)
	productID := insertProduct(t, pool, 42.50, true)

	resp := doAuthJSON(t, http.MethodPost, server+"/api/v1/service-orders/"+order.ID+"/diagnosis", nil, authToken)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	body := map[string]any{"items": []map[string]any{{"productId": productID, "quantity": 1}}}
	resp = doAuthJSON(t, http.MethodPut, server+"/api/v1/service-orders/"+order.ID+"/quote", body, authToken)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	resp = doAuthJSON(t, http.MethodPost, server+"/api/v1/service-orders/"+order.ID+"/quote/send", nil, authToken)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var quote serviceorder.QuoteResponse
	decodeBody(t, resp, &quote)
	require.NotNil(t, quote.SentAt)
	require.NotNil(t, quote.SentVersion)
	assert.Equal(t, quote.Version, *quote.SentVersion)

	var status string
	err := pool.QueryRow(context.Background(), `SELECT status FROM service_orders WHERE id = $1`, order.ID).Scan(&status)
	require.NoError(t, err)
	assert.Equal(t, "AWAITING_APPROVAL", status)

	var previousStatus, newStatus string
	err = pool.QueryRow(context.Background(),
		`SELECT previous_status, new_status FROM service_order_history WHERE service_order_id = $1 AND event = 'quote_sent'`,
		order.ID,
	).Scan(&previousStatus, &newStatus)
	require.NoError(t, err)
	assert.Equal(t, "IN_DIAGNOSIS", previousStatus)
	assert.Equal(t, "AWAITING_APPROVAL", newStatus)
}

func TestQuoteDecisionSendRequiresAuth(t *testing.T) {
	pool, server, _ := testServiceOrderServer(t)
	order := createServiceOrder(t, server, pool)

	resp := doAuthJSON(t, http.MethodPost, server+"/api/v1/service-orders/"+order.ID+"/quote/send", nil, "")
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

// TestQuoteDecisionSendRejectsIncompleteQuote covers "orçamento incompleto
// não pode ser enviado": no quote was ever composed for this order.
func TestQuoteDecisionSendRejectsIncompleteQuote(t *testing.T) {
	pool, server, authToken := testServiceOrderServer(t)
	order := createServiceOrder(t, server, pool)

	resp := doAuthJSON(t, http.MethodPost, server+"/api/v1/service-orders/"+order.ID+"/diagnosis", nil, authToken)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	resp = doAuthJSON(t, http.MethodPost, server+"/api/v1/service-orders/"+order.ID+"/quote/send", nil, authToken)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestQuoteDecisionSendRejectsBeforeDiagnosis(t *testing.T) {
	pool, server, authToken := testServiceOrderServer(t)
	order := createServiceOrder(t, server, pool)

	resp := doAuthJSON(t, http.MethodPost, server+"/api/v1/service-orders/"+order.ID+"/quote/send", nil, authToken)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

// ---- Approve ----

func TestQuoteDecisionApproveFullFlow(t *testing.T) {
	pool, server, authToken := testServiceOrderServer(t)
	order := createOrderWithSentQuote(t, server, pool, authToken)

	resp := doTrackingPost(t, server, orcamentoPath(order.Code, "aprovar"), order.TrackingToken)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var status, quoteStatus string
	var respondedAt *string
	err := pool.QueryRow(context.Background(), `SELECT status FROM service_orders WHERE id = $1`, order.ID).Scan(&status)
	require.NoError(t, err)
	assert.Equal(t, "IN_PROGRESS", status)

	err = pool.QueryRow(context.Background(), `SELECT status, responded_at::text FROM quotes WHERE service_order_id = $1`, order.ID).Scan(&quoteStatus, &respondedAt)
	require.NoError(t, err)
	assert.Equal(t, "APPROVED", quoteStatus)
	require.NotNil(t, respondedAt)

	var previousStatus, newStatus string
	err = pool.QueryRow(context.Background(),
		`SELECT previous_status, new_status FROM service_order_history WHERE service_order_id = $1 AND event = 'approval'`,
		order.ID,
	).Scan(&previousStatus, &newStatus)
	require.NoError(t, err)
	assert.Equal(t, "AWAITING_APPROVAL", previousStatus)
	assert.Equal(t, "IN_PROGRESS", newStatus)
}

func TestQuoteDecisionRejectFullFlow(t *testing.T) {
	pool, server, authToken := testServiceOrderServer(t)
	order := createOrderWithSentQuote(t, server, pool, authToken)

	resp := doTrackingPost(t, server, orcamentoPath(order.Code, "reprovar"), order.TrackingToken)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var status, quoteStatus string
	err := pool.QueryRow(context.Background(), `SELECT status FROM service_orders WHERE id = $1`, order.ID).Scan(&status)
	require.NoError(t, err)
	assert.Equal(t, "CANCELED", status)

	err = pool.QueryRow(context.Background(), `SELECT status FROM quotes WHERE service_order_id = $1`, order.ID).Scan(&quoteStatus)
	require.NoError(t, err)
	assert.Equal(t, "REJECTED", quoteStatus)

	var previousStatus, newStatus string
	err = pool.QueryRow(context.Background(),
		`SELECT previous_status, new_status FROM service_order_history WHERE service_order_id = $1 AND event = 'cancellation'`,
		order.ID,
	).Scan(&previousStatus, &newStatus)
	require.NoError(t, err)
	assert.Equal(t, "AWAITING_APPROVAL", previousStatus)
	assert.Equal(t, "CANCELED", newStatus)
}

func TestQuoteDecisionApproveMissingToken(t *testing.T) {
	pool, server, authToken := testServiceOrderServer(t)
	order := createOrderWithSentQuote(t, server, pool, authToken)

	resp := doTrackingPost(t, server, orcamentoPath(order.Code, "aprovar"), "")
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestQuoteDecisionApproveWrongToken(t *testing.T) {
	pool, server, authToken := testServiceOrderServer(t)
	order := createOrderWithSentQuote(t, server, pool, authToken)

	resp := doTrackingPost(t, server, orcamentoPath(order.Code, "aprovar"), "not-the-real-token")
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestQuoteDecisionApproveUnknownCode(t *testing.T) {
	_, server, _ := testServiceOrderServer(t)

	resp := doTrackingPost(t, server, orcamentoPath(987654321, "aprovar"), "any-token")
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

// TestQuoteDecisionCrossOrderToken covers "o cliente não consegue responder
// a orçamento de outra OS": order B's token must not unlock order A.
func TestQuoteDecisionCrossOrderToken(t *testing.T) {
	pool, server, authToken := testServiceOrderServer(t)
	orderA := createOrderWithSentQuote(t, server, pool, authToken)
	orderB := createOrderWithSentQuote(t, server, pool, authToken)

	resp := doTrackingPost(t, server, orcamentoPath(orderA.Code, "aprovar"), orderB.TrackingToken)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)

	var status string
	err := pool.QueryRow(context.Background(), `SELECT status FROM service_orders WHERE id = $1`, orderA.ID).Scan(&status)
	require.NoError(t, err)
	assert.Equal(t, "AWAITING_APPROVAL", status, "a rejected cross-order attempt must not change order A")
}

func TestQuoteDecisionDoesNotRequireAdminJWT(t *testing.T) {
	pool, server, authToken := testServiceOrderServer(t)
	order := createOrderWithSentQuote(t, server, pool, authToken)

	// No Authorization header is ever set by doTrackingPost.
	resp := doTrackingPost(t, server, orcamentoPath(order.Code, "aprovar"), order.TrackingToken)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

// TestQuoteDecisionApproveThenRejectConflict covers "não é possível aprovar
// e reprovar o mesmo orçamento" and, since it proves the order/quote/history
// state left by the first (successful) decision is completely unaffected by
// the second (rejected) one, the RNF07 guarantee that a failed decision
// leaves no partial trace.
func TestQuoteDecisionApproveThenRejectConflict(t *testing.T) {
	pool, server, authToken := testServiceOrderServer(t)
	order := createOrderWithSentQuote(t, server, pool, authToken)

	resp := doTrackingPost(t, server, orcamentoPath(order.Code, "aprovar"), order.TrackingToken)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	resp = doTrackingPost(t, server, orcamentoPath(order.Code, "reprovar"), order.TrackingToken)
	assert.Equal(t, http.StatusConflict, resp.StatusCode)

	var status, quoteStatus string
	err := pool.QueryRow(context.Background(), `SELECT status FROM service_orders WHERE id = $1`, order.ID).Scan(&status)
	require.NoError(t, err)
	assert.Equal(t, "IN_PROGRESS", status, "the rejected second decision must not alter the order reached by the first")

	err = pool.QueryRow(context.Background(), `SELECT status FROM quotes WHERE service_order_id = $1`, order.ID).Scan(&quoteStatus)
	require.NoError(t, err)
	assert.Equal(t, "APPROVED", quoteStatus)

	var cancellationCount int
	err = pool.QueryRow(context.Background(),
		`SELECT count(*) FROM service_order_history WHERE service_order_id = $1 AND event = 'cancellation'`,
		order.ID,
	).Scan(&cancellationCount)
	require.NoError(t, err)
	assert.Zero(t, cancellationCount, "the rejected second decision must not write a history entry")
}

// TestQuoteDecisionRepeatApproveConflict covers "repetição da mesma decisão
// é idempotente ou tratada de forma consistente": a repeated identical
// decision is treated the same way as a differing one (409 conflict), with
// no duplicated history entry from the second attempt.
func TestQuoteDecisionRepeatApproveConflict(t *testing.T) {
	pool, server, authToken := testServiceOrderServer(t)
	order := createOrderWithSentQuote(t, server, pool, authToken)

	resp := doTrackingPost(t, server, orcamentoPath(order.Code, "aprovar"), order.TrackingToken)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	resp = doTrackingPost(t, server, orcamentoPath(order.Code, "aprovar"), order.TrackingToken)
	assert.Equal(t, http.StatusConflict, resp.StatusCode)

	var approvalCount int
	err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM service_order_history WHERE service_order_id = $1 AND event = 'approval'`,
		order.ID,
	).Scan(&approvalCount)
	require.NoError(t, err)
	assert.Equal(t, 1, approvalCount)
}
