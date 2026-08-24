package serviceorder

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// seedExecution registers a completed (or, when endedAt is nil, still
// in-progress) execution of serviceID within a fresh order, started at
// startedAt.
func seedExecution(repo *fakeRepository, customerID, vehicleID, serviceID uuid.UUID, startedAt time.Time, endedAt *time.Time) {
	order := seedListOrder(repo, customerID, vehicleID, StatusEmExecucao, startedAt)
	execution := &ServiceExecution{
		ID:             uuid.New(),
		ServiceOrderID: order.ID,
		ServiceID:      serviceID,
		StartedAt:      startedAt,
		EndedAt:        endedAt,
	}
	repo.executions[order.ID] = append(repo.executions[order.ID], execution)
}

func TestAverageExecutionTimeAveragesCompletedExecutions(t *testing.T) {
	repo := newFakeRepository()
	service := newTestService(repo)
	customerID, vehicleID := seedActiveCustomerAndVehicle(repo)
	serviceID := uuid.New()
	repo.addService(&serviceRef{ID: serviceID, Code: 1, Name: "Troca de óleo"})

	start := time.Now().Add(-2 * time.Hour)
	end30 := start.Add(30 * time.Minute)
	end50 := start.Add(50 * time.Minute)
	seedExecution(repo, customerID, vehicleID, serviceID, start, &end30)
	seedExecution(repo, customerID, vehicleID, serviceID, start, &end50)

	metrics, err := service.AverageExecutionTime(context.Background(), MetricsFilter{})
	require.NoError(t, err)
	require.Len(t, metrics, 1)
	assert.Equal(t, serviceID, metrics[0].ServiceID)
	assert.Equal(t, 2, metrics[0].ExecutionCount)
	assert.InDelta(t, 40.0, metrics[0].AverageDurationMinutes, 0.001)
}

func TestAverageExecutionTimeExcludesInProgressExecutions(t *testing.T) {
	repo := newFakeRepository()
	service := newTestService(repo)
	customerID, vehicleID := seedActiveCustomerAndVehicle(repo)
	serviceID := uuid.New()
	repo.addService(&serviceRef{ID: serviceID, Code: 1, Name: "Troca de óleo"})

	start := time.Now().Add(-time.Hour)
	end := start.Add(20 * time.Minute)
	seedExecution(repo, customerID, vehicleID, serviceID, start, &end)
	seedExecution(repo, customerID, vehicleID, serviceID, start, nil) // still in progress

	metrics, err := service.AverageExecutionTime(context.Background(), MetricsFilter{})
	require.NoError(t, err)
	require.Len(t, metrics, 1)
	assert.Equal(t, 1, metrics[0].ExecutionCount)
	assert.InDelta(t, 20.0, metrics[0].AverageDurationMinutes, 0.001)
}

func TestAverageExecutionTimeNoDataReturnsEmptyList(t *testing.T) {
	repo := newFakeRepository()
	service := newTestService(repo)

	metrics, err := service.AverageExecutionTime(context.Background(), MetricsFilter{})
	require.NoError(t, err)
	assert.Empty(t, metrics)
}

func TestAverageExecutionTimeFiltersByService(t *testing.T) {
	repo := newFakeRepository()
	service := newTestService(repo)
	customerID, vehicleID := seedActiveCustomerAndVehicle(repo)
	targetID := uuid.New()
	otherID := uuid.New()
	repo.addService(&serviceRef{ID: targetID, Code: 1, Name: "Troca de óleo"})
	repo.addService(&serviceRef{ID: otherID, Code: 2, Name: "Alinhamento"})

	start := time.Now().Add(-time.Hour)
	end := start.Add(15 * time.Minute)
	seedExecution(repo, customerID, vehicleID, targetID, start, &end)
	seedExecution(repo, customerID, vehicleID, otherID, start, &end)

	metrics, err := service.AverageExecutionTime(context.Background(), MetricsFilter{ServiceID: &targetID})
	require.NoError(t, err)
	require.Len(t, metrics, 1)
	assert.Equal(t, targetID, metrics[0].ServiceID)
}

func TestAverageExecutionTimeFiltersByDateRange(t *testing.T) {
	repo := newFakeRepository()
	service := newTestService(repo)
	customerID, vehicleID := seedActiveCustomerAndVehicle(repo)
	serviceID := uuid.New()
	repo.addService(&serviceRef{ID: serviceID, Code: 1, Name: "Troca de óleo"})

	oldStart := time.Now().Add(-30 * 24 * time.Hour)
	oldEnd := oldStart.Add(10 * time.Minute)
	recentStart := time.Now().Add(-time.Hour)
	recentEnd := recentStart.Add(10 * time.Minute)
	seedExecution(repo, customerID, vehicleID, serviceID, oldStart, &oldEnd)
	seedExecution(repo, customerID, vehicleID, serviceID, recentStart, &recentEnd)

	from := time.Now().Add(-24 * time.Hour)
	to := time.Now()
	metrics, err := service.AverageExecutionTime(context.Background(), MetricsFilter{StartDate: &from, EndDate: &to})
	require.NoError(t, err)
	require.Len(t, metrics, 1)
	assert.Equal(t, 1, metrics[0].ExecutionCount)
}

func TestAverageExecutionTimeCombinedFilters(t *testing.T) {
	repo := newFakeRepository()
	service := newTestService(repo)
	customerID, vehicleID := seedActiveCustomerAndVehicle(repo)
	targetID := uuid.New()
	otherID := uuid.New()
	repo.addService(&serviceRef{ID: targetID, Code: 1, Name: "Troca de óleo"})
	repo.addService(&serviceRef{ID: otherID, Code: 2, Name: "Alinhamento"})

	recentStart := time.Now().Add(-time.Hour)
	recentEnd := recentStart.Add(10 * time.Minute)
	oldStart := time.Now().Add(-30 * 24 * time.Hour)
	oldEnd := oldStart.Add(10 * time.Minute)
	seedExecution(repo, customerID, vehicleID, targetID, recentStart, &recentEnd)
	seedExecution(repo, customerID, vehicleID, targetID, oldStart, &oldEnd) // outside range
	seedExecution(repo, customerID, vehicleID, otherID, recentStart, &recentEnd) // wrong service

	from := time.Now().Add(-24 * time.Hour)
	to := time.Now()
	metrics, err := service.AverageExecutionTime(context.Background(), MetricsFilter{ServiceID: &targetID, StartDate: &from, EndDate: &to})
	require.NoError(t, err)
	require.Len(t, metrics, 1)
	assert.Equal(t, targetID, metrics[0].ServiceID)
	assert.Equal(t, 1, metrics[0].ExecutionCount)
}
