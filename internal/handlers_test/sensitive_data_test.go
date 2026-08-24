package handlers_test

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	serviceorder "automotive-workshop-api/internal/features/service-order"
)

// Enforces RNF08 / BR-Q7 (specs/quality-and-security/requirements.md §4): no error
// response may disclose a credential, a token, a password hash, or infrastructure
// detail. The audit in specs/quality-and-security/design.md §4.1 found the code already
// compliant; these tests are what keeps it that way.

// infrastructureLeaks are substrings that must never reach a client in any error
// response, regardless of the endpoint that produced it.
var infrastructureLeaks = []string{
	"postgres://",             // database connection string
	"sslmode",                 // connection string parameter
	"pgx",                     // driver internals
	"SELECT ",                 // SQL fragments
	"INSERT ",                 // SQL fragments
	"$2a$",                    // bcrypt hash prefix
	"$2b$",                    // bcrypt hash prefix
	"integration-test-secret", // JWT signing secret
}

// assertNoSensitiveData reads the response body and fails when it contains any
// infrastructure leak or any of the extra case-specific secrets.
func assertNoSensitiveData(t *testing.T, response *http.Response, extraSecrets ...string) string {
	t.Helper()

	raw, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	body := string(raw)

	for _, leak := range infrastructureLeaks {
		assert.NotContains(t, body, leak, "error response leaked infrastructure detail %q: %s", leak, body)
	}
	for _, secret := range extraSecrets {
		if secret == "" {
			continue
		}
		assert.NotContains(t, body, secret, "error response leaked a secret: %s", body)
	}
	return body
}

// TestFailedLoginResponseHidesCredentials covers the login failure path: neither the
// submitted password nor the stored bcrypt hash may appear in the response.
func TestFailedLoginResponseHidesCredentials(t *testing.T) {
	mux := newTestMux(t)

	const wrongPassword = "wrong-password-9f2b"
	recorder := doLogin(t, mux, `{"email":"admin@workshop.local","password":"`+wrongPassword+`"}`)

	require.Equal(t, http.StatusUnauthorized, recorder.Code)
	body := recorder.Body.String()
	for _, leak := range infrastructureLeaks {
		assert.NotContains(t, body, leak, "login failure leaked %q: %s", leak, body)
	}
	assert.NotContains(t, body, wrongPassword, "login failure echoed the submitted password: %s", body)
	assert.NotContains(t, body, "admin123", "login failure leaked the stored credential: %s", body)
}

// TestMissingTokenResponseHidesToken covers the unauthenticated path: the 401 body must
// not contain a JWT.
func TestMissingTokenResponseHidesToken(t *testing.T) {
	mux := newTestMux(t)

	recorder := getMe(t, mux, "")

	require.Equal(t, http.StatusUnauthorized, recorder.Code)
	body := recorder.Body.String()
	assert.NotContains(t, body, "eyJ", "401 body contains something shaped like a JWT: %s", body)
	for _, leak := range infrastructureLeaks {
		assert.NotContains(t, body, leak, "401 body leaked %q: %s", leak, body)
	}
}

// TestInvalidTokenResponseDoesNotEchoToken covers the tampered-token path: the rejected
// token must not be reflected back, so it cannot reach a log aggregator or a browser
// history through the response.
func TestInvalidTokenResponseDoesNotEchoToken(t *testing.T) {
	mux := newTestMux(t)

	validToken := loginToken(t, mux)
	tamperedToken := validToken[:len(validToken)-4] + "AAAA"

	recorder := getMe(t, mux, "Bearer "+tamperedToken)

	require.Equal(t, http.StatusUnauthorized, recorder.Code)
	body := recorder.Body.String()
	assert.NotContains(t, body, tamperedToken, "401 body echoed the rejected token: %s", body)
	assert.NotContains(t, body, validToken, "401 body leaked a valid token: %s", body)
}

// TestImproperTrackingAccessHidesTokens covers the customer-facing tracking endpoint: a
// wrong tracking token must be rejected without echoing it and without disclosing the
// order's real token.
func TestImproperTrackingAccessHidesTokens(t *testing.T) {
	pool, server := testTrackingServer(t)
	order := createTrackingOrder(t, server, pool)

	const wrongToken = "0123456789abcdef0123456789abcdef"
	response := doTracking(t, server, order.Code, wrongToken)

	require.Equal(t, http.StatusUnauthorized, response.StatusCode)
	assertNoSensitiveData(t, response, wrongToken, order.TrackingToken)
}

// TestInternalErrorResponseHidesInfrastructure covers the 500 path. The duplicated
// requested-service id makes the creation transaction fail inside the repository
// (same trigger as TestServiceOrderCreateRollsBackOnPartialFailure), which is the only
// deterministic way to reach the `default` branch of writeServiceError over real HTTP.
// The client must get the generic envelope, never the driver's error.
func TestInternalErrorResponseHidesInfrastructure(t *testing.T) {
	pool, server, _ := testServiceOrderServer(t)

	createdCustomer := insertActiveCustomer(t, server, pool)
	vehicleID := insertVehicle(t, pool, createdCustomer.ID, randomLicensePlate(), true)
	serviceID := insertService(t, pool)

	response := doJSON(t, http.MethodPost, server+"/api/v1/service-orders", serviceorder.CreateRequest{
		CustomerID:          createdCustomer.ID,
		VehicleID:           vehicleID,
		RequestedServiceIDs: []string{serviceID, serviceID},
	})

	require.Equal(t, http.StatusInternalServerError, response.StatusCode)
	body := assertNoSensitiveData(t, response)
	assert.NotContains(t, body, "duplicate key", "500 body leaked the database error: %s", body)
	assert.NotContains(t, body, "constraint", "500 body leaked the database error: %s", body)
}

// TestErrorResponsesNeverLeakStoredPasswordHash asserts the strongest form of the rule
// for the credential that actually exists in the database: the administrative user's
// bcrypt hash, read straight from `users`, must not appear in any authentication error
// response.
func TestErrorResponsesNeverLeakStoredPasswordHash(t *testing.T) {
	mux := newTestMux(t)
	storedHash := readAdminPasswordHash(t)

	recorder := doLogin(t, mux, `{"email":"admin@workshop.local","password":"not-the-password"}`)
	require.Equal(t, http.StatusUnauthorized, recorder.Code)
	assert.NotContains(t, recorder.Body.String(), storedHash, "login failure leaked the stored password hash")

	recorder = doLogin(t, mux, `{"email":"ghost@workshop.local","password":"not-the-password"}`)
	require.Equal(t, http.StatusUnauthorized, recorder.Code)
	assert.NotContains(t, recorder.Body.String(), storedHash, "unknown-user login leaked the stored password hash")
}

// readAdminPasswordHash reads the seeded administrator's bcrypt hash directly, so the
// assertions above compare against the real value rather than a guessed prefix.
func readAdminPasswordHash(t *testing.T) string {
	t.Helper()

	pool, err := pgxpool.New(context.Background(), testDatabaseURL())
	if err != nil {
		t.Skipf("skipping integration test: cannot create pool: %v", err)
	}
	t.Cleanup(pool.Close)

	var hash string
	err = pool.QueryRow(context.Background(),
		`SELECT password_hash FROM users WHERE email = 'admin@workshop.local'`).Scan(&hash)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(hash, "$2"), "seeded password should be a bcrypt hash")
	return hash
}
