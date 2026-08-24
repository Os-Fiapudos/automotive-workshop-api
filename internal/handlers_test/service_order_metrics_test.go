package handlers_test

import (
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	serviceorder "automotive-workshop-api/internal/features/service-order"
)

// startAndFinishExecution starts an execution of serviceID on order and
// finishes it endedAfter later, returning the started execution.
func startAndFinishExecution(t *testing.T, server, authToken, orderID, serviceID string, endedAfter time.Duration) serviceorder.ServiceExecutionResponse {
	t.Helper()

	startResp := doAuthJSON(t, http.MethodPost, server+"/api/v1/service-orders/"+orderID+"/executions",
		map[string]any{"serviceId": serviceID}, authToken)
	require.Equal(t, http.StatusCreated, startResp.StatusCode)
	var started serviceorder.ServiceExecutionResponse
	decodeBody(t, startResp, &started)

	endedAt := started.StartedAt.Add(endedAfter)
	finishResp := doAuthJSON(t, http.MethodPost, server+"/api/v1/service-orders/"+orderID+"/executions/"+started.ID+"/finish",
		map[string]any{"endedAt": endedAt.Format(time.RFC3339)}, authToken)
	require.Equal(t, http.StatusOK, finishResp.StatusCode)
	var finished serviceorder.ServiceExecutionResponse
	decodeBody(t, finishResp, &finished)
	return finished
}

// TestAverageExecutionTimeAveragesCompletedExecutions covers
// requirements.md AC1/AC2 (specs/service-order-metrics/): two completed
// executions of the same service average correctly, and a still-in-progress
// third execution is excluded.
func TestAverageExecutionTimeAveragesCompletedExecutions(t *testing.T) {
	pool, server, authToken := testServiceOrderServer(t)
	order, serviceID := createOrderInExecutionWithRequiredService(t, server, pool, authToken)

	startAndFinishExecution(t, server, authToken, order.ID, serviceID, 30*time.Minute)
	startAndFinishExecution(t, server, authToken, order.ID, serviceID, 50*time.Minute)

	// A third, still-in-progress execution must not affect the average.
	startResp := doAuthJSON(t, http.MethodPost, server+"/api/v1/service-orders/"+order.ID+"/executions",
		map[string]any{"serviceId": serviceID}, authToken)
	require.Equal(t, http.StatusCreated, startResp.StatusCode)

	resp := doAuthJSON(t, http.MethodGet,
		server+"/api/v1/service-orders/metrics/average-execution-time?serviceId="+url.QueryEscape(serviceID),
		nil, authToken)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var body serviceorder.AverageExecutionTimeResponse
	decodeBody(t, resp, &body)
	require.Len(t, body.Services, 1)
	assert.Equal(t, serviceID, body.Services[0].ServiceID)
	assert.Equal(t, 2, body.Services[0].ExecutionCount)
	assert.InDelta(t, 40.0, body.Services[0].AverageDurationMinutes, 0.5)
}

// TestAverageExecutionTimeNoDataReturnsEmptyList covers requirements.md AC6:
// a serviceId with no completed executions returns 200 with an empty list,
// never an error.
func TestAverageExecutionTimeNoDataReturnsEmptyList(t *testing.T) {
	_, server, authToken := testServiceOrderServer(t)

	resp := doAuthJSON(t, http.MethodGet,
		server+"/api/v1/service-orders/metrics/average-execution-time?serviceId="+uuid.NewString(),
		nil, authToken)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var body serviceorder.AverageExecutionTimeResponse
	decodeBody(t, resp, &body)
	assert.Empty(t, body.Services)
}

// TestAverageExecutionTimeFiltersByDateRange covers requirements.md AC4:
// an execution started outside the requested range is excluded.
func TestAverageExecutionTimeFiltersByDateRange(t *testing.T) {
	pool, server, authToken := testServiceOrderServer(t)
	order, serviceID := createOrderInExecutionWithRequiredService(t, server, pool, authToken)
	startAndFinishExecution(t, server, authToken, order.ID, serviceID, 10*time.Minute)

	future := time.Now().Add(24 * time.Hour)
	resp := doAuthJSON(t, http.MethodGet,
		server+"/api/v1/service-orders/metrics/average-execution-time?serviceId="+url.QueryEscape(serviceID)+
			"&startDate="+url.QueryEscape(future.Format(time.RFC3339)),
		nil, authToken)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var body serviceorder.AverageExecutionTimeResponse
	decodeBody(t, resp, &body)
	assert.Empty(t, body.Services)
}

// TestAverageExecutionTimeRequiresAuth covers requirements.md AC8 (RNF02).
func TestAverageExecutionTimeRequiresAuth(t *testing.T) {
	_, server, _ := testServiceOrderServer(t)

	resp := doAuthJSON(t, http.MethodGet, server+"/api/v1/service-orders/metrics/average-execution-time", nil, "")
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

// TestAverageExecutionTimeRejectsInvalidServiceID covers filter validation
// (design.md §1.3): a malformed serviceId is a 400, not a silently-ignored
// filter.
func TestAverageExecutionTimeRejectsInvalidServiceID(t *testing.T) {
	_, server, authToken := testServiceOrderServer(t)

	resp := doAuthJSON(t, http.MethodGet,
		server+"/api/v1/service-orders/metrics/average-execution-time?serviceId=not-a-uuid", nil, authToken)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}
