package serviceorder

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewServiceExecutionRejectsNilIDs(t *testing.T) {
	_, err := NewServiceExecution(uuid.Nil, uuid.New())
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidAggregate)

	_, err = NewServiceExecution(uuid.New(), uuid.Nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidAggregate)
}

func TestServiceExecutionFinishRecordsEndDate(t *testing.T) {
	execution, err := NewServiceExecution(uuid.New(), uuid.New())
	require.NoError(t, err)
	execution.StartedAt = time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)

	endedAt := execution.StartedAt.Add(2 * time.Hour)
	require.NoError(t, execution.finish(&endedAt))
	require.NotNil(t, execution.EndedAt)
	assert.Equal(t, endedAt, *execution.EndedAt)
}

// TestServiceExecutionFinishWithNilUsesServerDefault covers the "no
// caller-supplied endedAt" path (design.md §2.2): finish() must accept it
// without comparing against StartedAt, since resolving "now" is left to the
// database's own clock (execution_repository.go's FinishExecution).
func TestServiceExecutionFinishWithNilUsesServerDefault(t *testing.T) {
	execution, err := NewServiceExecution(uuid.New(), uuid.New())
	require.NoError(t, err)
	execution.StartedAt = time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)

	require.NoError(t, execution.finish(nil))
	assert.Nil(t, execution.EndedAt, "the repository, not this method, resolves the server-default end time")
}

func TestServiceExecutionFinishRejectsEndBeforeStart(t *testing.T) {
	execution, err := NewServiceExecution(uuid.New(), uuid.New())
	require.NoError(t, err)
	execution.StartedAt = time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)

	endedAt := execution.StartedAt.Add(-1 * time.Minute)
	err = execution.finish(&endedAt)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrServiceExecutionEndBeforeStart)
	assert.Nil(t, execution.EndedAt, "a rejected finish must not record an end date")
}

func TestServiceExecutionFinishRejectsAlreadyFinished(t *testing.T) {
	execution, err := NewServiceExecution(uuid.New(), uuid.New())
	require.NoError(t, err)
	execution.StartedAt = time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)

	firstEnd := execution.StartedAt.Add(time.Hour)
	require.NoError(t, execution.finish(&firstEnd))

	secondEnd := execution.StartedAt.Add(2 * time.Hour)
	err = execution.finish(&secondEnd)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrServiceExecutionAlreadyFinished)
}

func TestFinalizeFromEmExecucao(t *testing.T) {
	order := newTestOrder(t, StatusEmExecucao)
	require.NoError(t, order.finalize())
	assert.Equal(t, StatusFinalizada, order.Status)
}

func TestFinalizeRejectsNonEmExecucao(t *testing.T) {
	for _, status := range []Status{StatusRecebida, StatusEmDiagnostico, StatusAguardandoAprovacao, StatusFinalizada, StatusEntregue} {
		order := newTestOrder(t, status)
		err := order.finalize()
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrInvalidStatusTransition)
		assert.Equal(t, status, order.Status, "status must not change on a rejected transition")
	}
}

func TestDeliverFromFinalizada(t *testing.T) {
	order := newTestOrder(t, StatusFinalizada)
	require.NoError(t, order.deliver())
	assert.Equal(t, StatusEntregue, order.Status)
}

func TestDeliverRejectsNonFinalizada(t *testing.T) {
	for _, status := range []Status{StatusRecebida, StatusEmDiagnostico, StatusAguardandoAprovacao, StatusEmExecucao, StatusEntregue} {
		order := newTestOrder(t, status)
		err := order.deliver()
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrInvalidStatusTransition)
		assert.Equal(t, status, order.Status, "status must not change on a rejected transition")
	}
}
