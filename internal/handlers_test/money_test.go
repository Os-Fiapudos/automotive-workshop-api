package handlers_test

import (
	"context"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"automotive-workshop-api/internal/features/product"
	servicecatalog "automotive-workshop-api/internal/features/service-catalog"
	"automotive-workshop-api/internal/shared/database"
	"automotive-workshop-api/internal/shared/money"
)

// Round-trip tests for specs/monetary-value-precision/ (requirements.md AC3):
// a money.Cents value written to a real NUMERIC(12,2) column and read back
// is exactly the same value, in every feature that stores money.

// roundTripValues are known to lose precision on a naive float64 path, plus
// zero and the NUMERIC(12,2) limit.
var roundTripValues = []money.Cents{0, 10, 1990, 123456789, money.MaxAmount}

func moneyTestPool(t *testing.T) *pgxpool.Pool {
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

// The adapter itself, against a NUMERIC(12,2) column: exact value back, and
// Postgres stores exactly the decimal the cents denote.
func TestMoneyNumericColumnRoundTrip(t *testing.T) {
	pool := moneyTestPool(t)
	ctx := context.Background()

	tx, err := pool.Begin(ctx)
	require.NoError(t, err)
	defer tx.Rollback(ctx) //nolint:errcheck // test transaction is never committed

	_, err = tx.Exec(ctx, `CREATE TEMP TABLE money_round_trip (amount NUMERIC(12, 2) NOT NULL) ON COMMIT DROP`)
	require.NoError(t, err)

	for _, cents := range append(roundTripValues, -525) {
		_, err := tx.Exec(ctx, `DELETE FROM money_round_trip`)
		require.NoError(t, err)
		_, err = tx.Exec(ctx, `INSERT INTO money_round_trip (amount) VALUES ($1)`, database.NumericFromCents(cents))
		require.NoError(t, err)

		var got money.Cents
		var text string
		require.NoError(t, tx.QueryRow(ctx, `SELECT amount, amount::text FROM money_round_trip`).Scan(database.ScanCents(&got), &text))
		assert.Equal(t, cents, got)
		assert.Equal(t, cents.String(), text)
	}

	// One cent beyond the limit is refused by the column itself; the money
	// type rejects it before it ever gets here (ErrOutOfRange).
	_, err = tx.Exec(ctx, `SAVEPOINT beyond_limit`)
	require.NoError(t, err)
	_, err = tx.Exec(ctx, `INSERT INTO money_round_trip (amount) VALUES ($1)`, database.NumericFromCents(money.MaxAmount+1))
	assert.Error(t, err)
	_, err = tx.Exec(ctx, `ROLLBACK TO SAVEPOINT beyond_limit`)
	require.NoError(t, err)
}

func TestServiceCatalogPriceRoundTrip(t *testing.T) {
	pool := moneyTestPool(t)
	ctx := context.Background()
	repository := servicecatalog.NewRepository(pool)

	for _, cents := range roundTripValues {
		created, err := repository.Create(ctx, servicecatalog.NewService{Name: "Integration Money Round Trip", Price: cents})
		require.NoError(t, err)
		t.Cleanup(func() { _, _ = pool.Exec(context.Background(), `DELETE FROM services WHERE id = $1`, created.ID) })
		assert.Equal(t, cents, created.Price)

		fetched, err := repository.FindByID(ctx, created.ID)
		require.NoError(t, err)
		assert.Equal(t, cents, fetched.Price)

		var text string
		require.NoError(t, pool.QueryRow(ctx, `SELECT price::text FROM services WHERE id = $1`, created.ID).Scan(&text))
		assert.Equal(t, cents.String(), text)

		updatedPrice := cents / 2
		updated, err := repository.Update(ctx, created.ID, servicecatalog.Changes{Price: &updatedPrice})
		require.NoError(t, err)
		assert.Equal(t, updatedPrice, updated.Price)
	}
}

func TestProductUnitPriceRoundTrip(t *testing.T) {
	pool := moneyTestPool(t)
	ctx := context.Background()
	repository := product.NewPostgresProductRepository(pool)

	for _, cents := range roundTripValues {
		created, err := product.NewProduct(nil, "Integration Money Round Trip", "d", cents, 1, "PART")
		require.NoError(t, err)
		require.NoError(t, repository.Create(ctx, created))
		t.Cleanup(func() { _, _ = pool.Exec(context.Background(), `DELETE FROM products WHERE id = $1`, created.ID) })

		fetched, err := repository.FindByID(ctx, created.ID)
		require.NoError(t, err)
		assert.Equal(t, cents, fetched.UnitPrice)

		var text string
		require.NoError(t, pool.QueryRow(ctx, `SELECT unit_price::text FROM products WHERE id = $1`, created.ID).Scan(&text))
		assert.Equal(t, cents.String(), text)

		fetched.UnitPrice = cents / 2
		require.NoError(t, repository.Update(ctx, fetched))
		reread, err := repository.FindByID(ctx, created.ID)
		require.NoError(t, err)
		assert.Equal(t, cents/2, reread.UnitPrice)
	}
}

// End to end through the quote: 3 × 19.90 + 7 × 0.10. In float64 these give
// 59.699999999999996 and 0.7000000000000001; the stored columns and the
// response must hold exactly 59.70, 0.70 and 60.40, and the JSON keeps the
// same number format as before.
func TestServiceOrderQuoteMoneyRoundTrip(t *testing.T) {
	pool, server, authToken := testServiceOrderServer(t)
	ctx := context.Background()
	order := createServiceOrder(t, server, pool)
	productID := insertProduct(t, pool, 19.90, true)

	var serviceID string
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO services (name, description, price) VALUES ('Integration Money', 'Ten-cent service.', 0.10) RETURNING id`,
	).Scan(&serviceID))
	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), `DELETE FROM services WHERE id = $1`, serviceID) })

	resp := doAuthJSON(t, http.MethodPost, server+"/api/v1/service-orders/"+order.ID+"/diagnosis", nil, authToken)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	resp = doAuthJSON(t, http.MethodPut, server+"/api/v1/service-orders/"+order.ID+"/quote", map[string]any{
		"items": []map[string]any{
			{"productId": productID, "quantity": 3},
			{"serviceId": serviceID, "quantity": 7},
		},
	}, authToken)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	_ = resp.Body.Close()

	getResp := doAuthJSON(t, http.MethodGet, server+"/api/v1/service-orders/"+order.ID+"/quote", nil, authToken)
	require.Equal(t, http.StatusOK, getResp.StatusCode)
	raw, err := io.ReadAll(getResp.Body)
	require.NoError(t, err)
	_ = getResp.Body.Close()
	body := string(raw)
	assert.Contains(t, body, `"totalAmount":60.4,`)
	assert.Contains(t, body, `"unitPrice":19.9,"total":59.7}`)
	assert.Contains(t, body, `"unitPrice":0.1,"total":0.7}`)

	var total, productTotal, serviceTotal string
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT q.total_amount::text, qp.applied_total_price::text, qs.applied_total_price::text
		 FROM quotes q
		 JOIN quote_products qp ON qp.quote_id = q.id
		 JOIN quote_services qs ON qs.quote_id = q.id
		 WHERE q.service_order_id = $1`, order.ID,
	).Scan(&total, &productTotal, &serviceTotal))
	assert.Equal(t, "60.40", total)
	assert.Equal(t, "59.70", productTotal)
	assert.Equal(t, "0.70", serviceTotal)
}
