package serviceorder

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStartExecutionSuccess(t *testing.T) {
	repo := newFakeRepository()
	service := newTestService(repo)
	order := seedOrder(repo, StatusInProgress)
	serviceID := uuid.New()
	repo.addService(&serviceRef{ID: serviceID, Code: 1, Name: "Oil Change"})

	execution, err := service.StartExecution(context.Background(), order.ID, serviceID)
	require.NoError(t, err)
	assert.Equal(t, order.ID, execution.ServiceOrderID)
	assert.Equal(t, serviceID, execution.ServiceID)
	assert.Nil(t, execution.EndedAt)
	assert.False(t, execution.StartedAt.IsZero())
}

// TestStartExecutionRejectsWithoutApproval covers BR2 ("execução não pode
// ser iniciada sem orçamento aprovado") via its IN_PROGRESS proxy
// (requirements.md §2.1).
func TestStartExecutionRejectsWithoutApproval(t *testing.T) {
	repo := newFakeRepository()
	service := newTestService(repo)
	serviceID := uuid.New()
	repo.addService(&serviceRef{ID: serviceID, Code: 1, Name: "Oil Change"})

	for _, status := range []Status{StatusReceived, StatusInDiagnosis, StatusAwaitingApproval, StatusCompleted, StatusDelivered} {
		order := seedOrder(repo, status)
		_, err := service.StartExecution(context.Background(), order.ID, serviceID)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrInvalidStatusTransition)
	}
}

func TestStartExecutionRejectsUnknownService(t *testing.T) {
	repo := newFakeRepository()
	service := newTestService(repo)
	order := seedOrder(repo, StatusInProgress)

	_, err := service.StartExecution(context.Background(), order.ID, uuid.New())
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrRequestedServiceNotFound)
}

func TestFinishExecutionSuccess(t *testing.T) {
	repo := newFakeRepository()
	service := newTestService(repo)
	order := seedOrder(repo, StatusInProgress)
	serviceID := uuid.New()
	repo.addService(&serviceRef{ID: serviceID, Code: 1, Name: "Oil Change"})

	execution, err := service.StartExecution(context.Background(), order.ID, serviceID)
	require.NoError(t, err)

	finished, err := service.FinishExecution(context.Background(), order.ID, execution.ID, nil)
	require.NoError(t, err)
	require.NotNil(t, finished.EndedAt)
	assert.False(t, finished.EndedAt.Before(finished.StartedAt))
}

func TestFinishExecutionRejectsEndBeforeStart(t *testing.T) {
	repo := newFakeRepository()
	service := newTestService(repo)
	order := seedOrder(repo, StatusInProgress)
	serviceID := uuid.New()
	repo.addService(&serviceRef{ID: serviceID, Code: 1, Name: "Oil Change"})

	execution, err := service.StartExecution(context.Background(), order.ID, serviceID)
	require.NoError(t, err)

	before := execution.StartedAt.Add(-1 * time.Hour)
	_, err = service.FinishExecution(context.Background(), order.ID, execution.ID, &before)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrServiceExecutionEndBeforeStart)
}

func TestFinishExecutionRejectsUnknownExecution(t *testing.T) {
	repo := newFakeRepository()
	service := newTestService(repo)
	order := seedOrder(repo, StatusInProgress)

	_, err := service.FinishExecution(context.Background(), order.ID, uuid.New(), nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrServiceExecutionNotFound)
}

// TestFinishExecutionRejectsAfterFinalized covers BR6 ("uma OS finalizada
// não aceita ... baixas comuns").
func TestFinishExecutionRejectsAfterFinalized(t *testing.T) {
	repo := newFakeRepository()
	service := newTestService(repo)
	order := seedOrder(repo, StatusInProgress)
	serviceID := uuid.New()
	repo.addService(&serviceRef{ID: serviceID, Code: 1, Name: "Oil Change"})

	execution, err := service.StartExecution(context.Background(), order.ID, serviceID)
	require.NoError(t, err)

	order.Status = StatusCompleted

	_, err = service.FinishExecution(context.Background(), order.ID, execution.ID, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidStatusTransition)
}

func TestFinalizeOrderSuccessWithoutQuote(t *testing.T) {
	repo := newFakeRepository()
	service := newTestService(repo)
	order := seedOrder(repo, StatusInProgress)

	finalized, err := service.FinalizeOrder(context.Background(), order.ID)
	require.NoError(t, err)
	assert.Equal(t, StatusCompleted, finalized.Status)
}

func TestFinalizeOrderRejectsNonInProgress(t *testing.T) {
	repo := newFakeRepository()
	service := newTestService(repo)
	order := seedOrder(repo, StatusAwaitingApproval)

	_, err := service.FinalizeOrder(context.Background(), order.ID)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidStatusTransition)
}

// TestFinalizeOrderRequiresCompletedExecutions covers BR5: every service
// line item of the approved quote must have a completed execution.
func TestFinalizeOrderRequiresCompletedExecutions(t *testing.T) {
	repo := newFakeRepository()
	service := newTestService(repo)
	order := seedOrder(repo, StatusInProgress)
	serviceID := uuid.New()
	repo.addService(&serviceRef{ID: serviceID, Code: 1, Name: "Oil Change"})
	repo.quotes[order.ID] = &Quote{
		ID:             uuid.New(),
		ServiceOrderID: order.ID,
		Status:         QuoteStatusApproved,
		Items:          []QuoteItem{{Kind: QuoteItemService, ServiceID: serviceID, Quantity: 1}},
	}

	_, err := service.FinalizeOrder(context.Background(), order.ID)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrExecutionsNotCompleted)

	execution, err := service.StartExecution(context.Background(), order.ID, serviceID)
	require.NoError(t, err)

	_, err = service.FinalizeOrder(context.Background(), order.ID)
	require.Error(t, err, "an execution that has started but not finished must still block finalization")
	assert.ErrorIs(t, err, ErrExecutionsNotCompleted)

	_, err = service.FinishExecution(context.Background(), order.ID, execution.ID, nil)
	require.NoError(t, err)

	finalized, err := service.FinalizeOrder(context.Background(), order.ID)
	require.NoError(t, err)
	assert.Equal(t, StatusCompleted, finalized.Status)
}

func TestDeliverOrderSuccess(t *testing.T) {
	repo := newFakeRepository()
	service := newTestService(repo)
	order := seedOrder(repo, StatusCompleted)

	delivered, err := service.DeliverOrder(context.Background(), order.ID)
	require.NoError(t, err)
	assert.Equal(t, StatusDelivered, delivered.Status)
}

func TestDeliverOrderRejectsNonCompleted(t *testing.T) {
	repo := newFakeRepository()
	service := newTestService(repo)
	order := seedOrder(repo, StatusInProgress)

	_, err := service.DeliverOrder(context.Background(), order.ID)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidStatusTransition)
}
