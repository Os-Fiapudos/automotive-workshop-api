package handlers_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	orderpostgres "automotive-workshop-api/internal/features/service-order/adapter/postgres"
	"automotive-workshop-api/internal/features/service-order/domain"
)

// The Postgres adapter re-checks the expected current status inside each
// transition's own transaction ("UPDATE ... WHERE status = ..."), so a request
// that read the order before a concurrent one changed it cannot apply a second
// transition (RNF07). TestExternalQuoteDecisionConcurrentDecisions exercises
// that race over HTTP, but whether a loser reaches these guards or is stopped
// earlier by the use case depends on scheduling. These tests reach every guard
// deterministically by handing the repository a stale copy of the order —
// exactly what the loser of the race holds
// (specs/service-order-architecture-refactor/ tasks.md §9).

func guardTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, testDatabaseURL())
	if err != nil {
		t.Skipf("skipping integration test: cannot create pool: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Skipf("skipping integration test: database unreachable (start it with `docker compose up -d db`): %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// openGuardTestOrder persists a RECEIVED order for a fresh customer/vehicle
// and returns it with the id of a catalog service for quote items.
func openGuardTestOrder(t *testing.T, pool *pgxpool.Pool, repository *orderpostgres.Repository) (*domain.ServiceOrder, uuid.UUID) {
	t.Helper()
	ctx := context.Background()

	var customerID string
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO customers (name, document, document_type, phone) VALUES ('Guard Test', $1, 'CPF', '+55 11 90000-0000') RETURNING id`,
		randomValidCPF(),
	).Scan(&customerID))
	cleanupCustomer(t, pool, customerID)

	vehicleID := insertVehicle(t, pool, customerID, "GRD"+uuid.NewString()[:4], true)
	serviceID := uuid.MustParse(insertService(t, pool))

	order, err := domain.NewServiceOrder(uuid.MustParse(customerID), uuid.MustParse(vehicleID), "guard test", nil, nil)
	require.NoError(t, err)
	_, err = repository.Create(ctx, order)
	require.NoError(t, err)
	// Registered last so it runs first: the order must go before the
	// vehicle/customer/service its rows reference.
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM service_orders WHERE id = $1`, order.ID)
	})
	return order, serviceID
}

// copyOrder returns an independent in-memory copy, standing in for the
// snapshot a concurrent request loaded before the order changed.
func copyOrder(order *domain.ServiceOrder) *domain.ServiceOrder {
	duplicate := *order
	return &duplicate
}

func TestRepositoryTransitionGuardsRejectStaleOrders(t *testing.T) {
	pool := guardTestPool(t)
	repository := orderpostgres.NewRepository(pool)
	ctx := context.Background()
	order, serviceID := openGuardTestOrder(t, pool, repository)

	// RECEIVED -> IN_DIAGNOSIS; a second, stale start must not apply again.
	stale := copyOrder(order)
	require.NoError(t, order.StartDiagnosis())
	require.NoError(t, repository.StartDiagnosis(ctx, order, ""))
	require.NoError(t, stale.StartDiagnosis())
	assert.ErrorIs(t, repository.StartDiagnosis(ctx, stale, ""), domain.ErrInvalidStatusTransition)

	// IN_DIAGNOSIS -> AWAITING_APPROVAL; a stale second send is refused.
	items := []domain.QuoteItem{{Kind: domain.QuoteItemService, ServiceID: serviceID, Description: "Oil Change", Quantity: 1, UnitPrice: 80, Total: 80}}
	quote, err := repository.SaveQuote(ctx, order, items, domain.CalculateTotal(items))
	require.NoError(t, err)
	stale = copyOrder(order)
	require.NoError(t, order.SendQuote())
	_, err = repository.SendQuote(ctx, order, quote)
	require.NoError(t, err)
	require.NoError(t, stale.SendQuote())
	_, err = repository.SendQuote(ctx, stale, quote)
	assert.ErrorIs(t, err, domain.ErrInvalidStatusTransition)

	// AWAITING_APPROVAL -> IN_PROGRESS. The loser still holds the quote as
	// PENDING, so the use case's own check would let it through; the guarded
	// UPDATE on quotes is what reports the decision as already taken.
	staleOrder, staleQuote := copyOrder(order), *quote
	require.NoError(t, order.ApproveQuote())
	_, err = repository.DecideQuote(ctx, order, quote, domain.QuoteStatusApproved)
	require.NoError(t, err)
	require.NoError(t, staleOrder.RejectQuote())
	_, err = repository.DecideQuote(ctx, staleOrder, &staleQuote, domain.QuoteStatusRejected)
	assert.ErrorIs(t, err, domain.ErrQuoteAlreadyDecided)

	// IN_PROGRESS -> COMPLETED -> DELIVERED, each refused when replayed stale.
	stale = copyOrder(order)
	require.NoError(t, order.Finalize())
	require.NoError(t, repository.FinalizeOrder(ctx, order, ""))
	require.NoError(t, stale.Finalize())
	assert.ErrorIs(t, repository.FinalizeOrder(ctx, stale, ""), domain.ErrInvalidStatusTransition)

	stale = copyOrder(order)
	require.NoError(t, order.Deliver())
	require.NoError(t, repository.DeliverOrder(ctx, order, ""))
	require.NoError(t, stale.Deliver())
	assert.ErrorIs(t, repository.DeliverOrder(ctx, stale, ""), domain.ErrInvalidStatusTransition)

	// Exactly one history entry per transition: no refused attempt wrote one.
	var events []string
	rows, err := pool.Query(ctx, `SELECT event::text FROM service_order_history WHERE service_order_id = $1 ORDER BY occurred_at`, order.ID)
	require.NoError(t, err)
	defer rows.Close()
	for rows.Next() {
		var event string
		require.NoError(t, rows.Scan(&event))
		events = append(events, event)
	}
	require.NoError(t, rows.Err())
	assert.Equal(t, []string{"creation", "diagnosis_started", "quote_composed", "quote_sent", "approval", "completion", "delivery"}, events)

	stored, err := repository.FindServiceOrderByID(ctx, order.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.StatusDelivered, stored.Status)
	storedQuote, err := repository.FindQuoteByServiceOrderID(ctx, order.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.QuoteStatusApproved, storedQuote.Status)
}
