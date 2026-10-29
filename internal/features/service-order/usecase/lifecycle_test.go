package usecase

import (
	"automotive-workshop-api/internal/features/service-order/domain"
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Unit tests for specs/service-order-status-update/ (design.md §7).

func TestUpdateStatusExternalDispatchesEachSupportedStatus(t *testing.T) {
	cases := []struct {
		from, to    domain.Status
		event, text string
	}{
		{domain.StatusReceived, domain.StatusInDiagnosis, "diagnosis_started", domain.HistoryDiagnosisStarted},
		{domain.StatusInProgress, domain.StatusCompleted, "completion", domain.HistoryOrderFinalized},
		{domain.StatusCompleted, domain.StatusDelivered, "delivery", domain.HistoryVehicleDelivered},
	}
	for _, tc := range cases {
		t.Run(string(tc.to), func(t *testing.T) {
			repo := newFakeRepository()
			service := newTestService(repo)
			order := seedOrder(repo, tc.from)

			result, err := service.UpdateStatusExternal(context.Background(), order.ID, tc.to, "")
			require.NoError(t, err)
			assert.Equal(t, tc.to, result.Order.Status)
			assert.Equal(t, tc.from, result.PreviousStatus)

			history := repo.history[order.ID]
			require.Len(t, history, 1)
			assert.Equal(t, tc.event, history[0].Event)
			assert.Equal(t, tc.text, history[0].Description, "without notes the description matches the administrative endpoint's")
			assert.Equal(t, tc.from, history[0].PreviousStatus)
			assert.Equal(t, tc.to, history[0].NewStatus)
		})
	}
}

func TestUpdateStatusExternalPersistsTrimmedNotes(t *testing.T) {
	repo := newFakeRepository()
	service := newTestService(repo)
	order := seedOrder(repo, domain.StatusReceived)

	_, err := service.UpdateStatusExternal(context.Background(), order.ID, domain.StatusInDiagnosis, "  Vehicle on lift 2  ")
	require.NoError(t, err)

	history := repo.history[order.ID]
	require.Len(t, history, 1)
	assert.Equal(t, "Diagnosis started. Notes: Vehicle on lift 2", history[0].Description)
}

func TestUpdateStatusExternalBlankNotesAreAbsent(t *testing.T) {
	repo := newFakeRepository()
	service := newTestService(repo)
	order := seedOrder(repo, domain.StatusCompleted)

	_, err := service.UpdateStatusExternal(context.Background(), order.ID, domain.StatusDelivered, "   ")
	require.NoError(t, err)
	assert.Equal(t, domain.HistoryVehicleDelivered, repo.history[order.ID][0].Description)
}

func TestUpdateStatusExternalRejectsUnsupportedStatuses(t *testing.T) {
	for _, status := range []domain.Status{domain.StatusReceived, domain.StatusAwaitingApproval, domain.StatusInProgress, domain.StatusCanceled, "UNKNOWN", ""} {
		t.Run(string(status), func(t *testing.T) {
			repo := newFakeRepository()
			service := newTestService(repo)
			order := seedOrder(repo, domain.StatusReceived)

			_, err := service.UpdateStatusExternal(context.Background(), order.ID, status, "")
			assert.ErrorIs(t, err, domain.ErrUnsupportedExternalStatus)
			assert.Equal(t, domain.StatusReceived, order.Status)
			assert.Empty(t, repo.history[order.ID])
		})
	}
}

// TestUpdateStatusExternalCannotSkipApproval covers BR2/BR3: an order still
// waiting for quote approval can be neither finalized nor delivered through
// the external channel.
func TestUpdateStatusExternalCannotSkipApproval(t *testing.T) {
	for _, target := range []domain.Status{domain.StatusCompleted, domain.StatusDelivered, domain.StatusInDiagnosis} {
		t.Run(string(target), func(t *testing.T) {
			repo := newFakeRepository()
			service := newTestService(repo)
			order := seedOrder(repo, domain.StatusAwaitingApproval)

			_, err := service.UpdateStatusExternal(context.Background(), order.ID, target, "note")
			assert.ErrorIs(t, err, domain.ErrInvalidStatusTransition)
			assert.Equal(t, domain.StatusAwaitingApproval, order.Status)
			assert.Empty(t, repo.history[order.ID])
		})
	}
}

func TestUpdateStatusExternalDeliverOnReceivedIsInvalidTransition(t *testing.T) {
	repo := newFakeRepository()
	service := newTestService(repo)
	order := seedOrder(repo, domain.StatusReceived)

	_, err := service.UpdateStatusExternal(context.Background(), order.ID, domain.StatusDelivered, "")
	assert.ErrorIs(t, err, domain.ErrInvalidStatusTransition)
	assert.Equal(t, domain.StatusReceived, order.Status)
	assert.Empty(t, repo.history[order.ID])
}

// TestUpdateStatusExternalCompletedRequiresExecutions covers AC2: the same
// required-executions precondition as POST /finalize.
func TestUpdateStatusExternalCompletedRequiresExecutions(t *testing.T) {
	repo := newFakeRepository()
	service := newTestService(repo)
	order := seedOrder(repo, domain.StatusInProgress)
	serviceID := uuid.New()
	repo.addService(&ServiceRef{ID: serviceID, Code: 1, Name: "Oil Change"})
	repo.quotes[order.ID] = &domain.Quote{
		ID:             uuid.New(),
		ServiceOrderID: order.ID,
		Status:         domain.QuoteStatusApproved,
		Items:          []domain.QuoteItem{{Kind: domain.QuoteItemService, ServiceID: serviceID, Quantity: 1}},
	}

	_, err := service.UpdateStatusExternal(context.Background(), order.ID, domain.StatusCompleted, "")
	assert.ErrorIs(t, err, domain.ErrExecutionsNotCompleted)
	assert.Equal(t, domain.StatusInProgress, order.Status)
	assert.Empty(t, repo.history[order.ID])
}

func TestUpdateStatusExternalUnknownOrder(t *testing.T) {
	service := newTestService(newFakeRepository())

	_, err := service.UpdateStatusExternal(context.Background(), uuid.New(), domain.StatusInDiagnosis, "")
	assert.ErrorIs(t, err, domain.ErrServiceOrderNotFound)
}

func TestUpdateStatusExternalNotesLength(t *testing.T) {
	repo := newFakeRepository()
	service := newTestService(repo)
	order := seedOrder(repo, domain.StatusReceived)

	_, err := service.UpdateStatusExternal(context.Background(), order.ID, domain.StatusInDiagnosis, strings.Repeat("é", MaxStatusUpdateNotesLength+1))
	assert.ErrorIs(t, err, domain.ErrInvalidStatusUpdateNotes)
	assert.Equal(t, domain.StatusReceived, order.Status)
	assert.Empty(t, repo.history[order.ID])

	// Exactly the limit, counted in characters (not bytes), is accepted.
	_, err = service.UpdateStatusExternal(context.Background(), order.ID, domain.StatusInDiagnosis, strings.Repeat("é", MaxStatusUpdateNotesLength))
	require.NoError(t, err)
}

// TestAdministrativeTransitionsKeepFixedDescriptions covers AC10: the
// JWT-protected endpoints still write their original description.
func TestAdministrativeTransitionsKeepFixedDescriptions(t *testing.T) {
	repo := newFakeRepository()
	service := newTestService(repo)
	order := seedOrder(repo, domain.StatusReceived)

	_, err := service.StartDiagnosis(context.Background(), order.ID)
	require.NoError(t, err)
	assert.Equal(t, "Diagnosis started.", repo.history[order.ID][0].Description)

	order.Status = domain.StatusInProgress
	_, err = service.FinalizeOrder(context.Background(), order.ID)
	require.NoError(t, err)
	assert.Equal(t, "Service order finalized.", repo.history[order.ID][1].Description)

	_, err = service.DeliverOrder(context.Background(), order.ID)
	require.NoError(t, err)
	assert.Equal(t, "Vehicle delivered to customer.", repo.history[order.ID][2].Description)
}

func TestAuthenticateExternalStatusUpdate(t *testing.T) {
	service := newTestService(newFakeRepository()).
		WithExternalNotificationSecret("quote-secret").
		WithExternalStatusUpdateSecret("status-secret")

	assert.True(t, service.AuthenticateExternalStatusUpdate("status-secret"))
	assert.False(t, service.AuthenticateExternalStatusUpdate(""))
	assert.False(t, service.AuthenticateExternalStatusUpdate("status-secre"))
	assert.False(t, service.AuthenticateExternalStatusUpdate("quote-secret"), "the quote-decision secret must not grant status updates")
	assert.False(t, service.AuthenticateExternalNotification("status-secret"), "and vice versa")
}

func TestAuthenticateExternalStatusUpdateRejectsWhenUnconfigured(t *testing.T) {
	service := newTestService(newFakeRepository())

	assert.False(t, service.AuthenticateExternalStatusUpdate(""))
	assert.False(t, service.AuthenticateExternalStatusUpdate("anything"))
}
