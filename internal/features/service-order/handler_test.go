package serviceorder

import (
	"automotive-workshop-api/internal/features/service-order/domain"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testExternalSecret = "external-test-secret"

// TestRegisterRoutesDoesNotPanic registers every route against a real
// *http.ServeMux — a pattern conflict (e.g. the new
// POST .../{id}/quote/decision against the existing {id}-shaped routes)
// panics at registration, which this catches without a database
// (specs/service-order-quote-decision/tasks.md, Fase 2).
func TestRegisterRoutesDoesNotPanic(t *testing.T) {
	passThrough := func(next http.Handler) http.Handler { return next }
	assert.NotPanics(t, func() {
		RegisterRoutes(http.NewServeMux(), newTestService(newFakeRepository()), passThrough)
	})
}

// newExternalDecisionMux wires the real routes with a requireAuth that
// rejects everything, proving the external decision route is not behind the
// administrative JWT.
func newExternalDecisionMux(repo *fakeRepository) (*http.ServeMux, *ServiceOrderService) {
	service := newTestService(repo).WithExternalNotificationSecret(testExternalSecret)
	denyAll := func(http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		})
	}
	mux := http.NewServeMux()
	RegisterRoutes(mux, service, denyAll)
	return mux, service
}

func postExternalDecision(mux *http.ServeMux, id, secret, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, "/api/v1/service-orders/"+id+"/quote/decision", strings.NewReader(body))
	if secret != "" {
		request.Header.Set(externalNotificationSecretHeader, secret)
	}
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, request)
	return recorder
}

func TestExternalDecisionApprovesWithoutJWT(t *testing.T) {
	repo := newFakeRepository()
	mux, service := newExternalDecisionMux(repo)
	order := seedSentQuote(t, service, repo, "the-real-token")

	response := postExternalDecision(mux, order.ID.String(), testExternalSecret, `{"decision":"approved"}`)
	require.Equal(t, http.StatusOK, response.Code)
	assert.Contains(t, response.Body.String(), `"orderStatus":"IN_PROGRESS"`)
	assert.Contains(t, response.Body.String(), `"quoteStatus":"APPROVED"`)
}

func TestExternalDecisionRejects(t *testing.T) {
	repo := newFakeRepository()
	mux, service := newExternalDecisionMux(repo)
	order := seedSentQuote(t, service, repo, "the-real-token")

	response := postExternalDecision(mux, order.ID.String(), testExternalSecret, `{"decision":"rejected"}`)
	require.Equal(t, http.StatusOK, response.Code)
	assert.Contains(t, response.Body.String(), `"orderStatus":"CANCELED"`)
	assert.Contains(t, response.Body.String(), `"quoteStatus":"REJECTED"`)
}

// TestExternalDecisionMissingOrWrongSecret covers AC17: 401 with no side
// effect — and the same 401 for an unknown order, so the secret check
// reveals nothing about which ids exist.
func TestExternalDecisionMissingOrWrongSecret(t *testing.T) {
	repo := newFakeRepository()
	mux, service := newExternalDecisionMux(repo)
	order := seedSentQuote(t, service, repo, "the-real-token")

	for _, secret := range []string{"", "wrong-secret"} {
		response := postExternalDecision(mux, order.ID.String(), secret, `{"decision":"approved"}`)
		assert.Equal(t, http.StatusUnauthorized, response.Code)
		assert.Contains(t, response.Body.String(), "INVALID_EXTERNAL_SECRET")
	}
	response := postExternalDecision(mux, "not-a-uuid", "wrong-secret", `{"decision":"approved"}`)
	assert.Equal(t, http.StatusUnauthorized, response.Code)

	assert.Equal(t, domain.StatusAwaitingApproval, order.Status)
	quote, err := repo.FindQuoteByServiceOrderID(t.Context(), order.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.QuoteStatusPending, quote.Status)
}

// TestExternalDecisionInvalidDecision covers AC18.
func TestExternalDecisionInvalidDecision(t *testing.T) {
	repo := newFakeRepository()
	mux, service := newExternalDecisionMux(repo)
	order := seedSentQuote(t, service, repo, "the-real-token")

	for _, body := range []string{`{"decision":"APPROVED"}`, `{"decision":"maybe"}`, `{}`} {
		response := postExternalDecision(mux, order.ID.String(), testExternalSecret, body)
		assert.Equal(t, http.StatusBadRequest, response.Code, body)
	}
	assert.Equal(t, domain.StatusAwaitingApproval, order.Status)
}

func TestExternalDecisionRepeatIsConflict(t *testing.T) {
	repo := newFakeRepository()
	mux, service := newExternalDecisionMux(repo)
	order := seedSentQuote(t, service, repo, "the-real-token")

	response := postExternalDecision(mux, order.ID.String(), testExternalSecret, `{"decision":"approved"}`)
	require.Equal(t, http.StatusOK, response.Code)

	response = postExternalDecision(mux, order.ID.String(), testExternalSecret, `{"decision":"rejected"}`)
	assert.Equal(t, http.StatusConflict, response.Code)
	assert.Contains(t, response.Body.String(), "QUOTE_ALREADY_DECIDED")
	assert.Equal(t, domain.StatusInProgress, order.Status)
}

// TestCustomerDecisionRoutesStillUnwrapped is the AC20 regression: the
// tracking-token routes keep working without the JWT alongside the new one.
func TestCustomerDecisionRoutesStillUnwrapped(t *testing.T) {
	repo := newFakeRepository()
	mux, service := newExternalDecisionMux(repo)
	order := seedSentQuote(t, service, repo, "the-real-token")

	request := httptest.NewRequest(http.MethodPost, "/api/v1/acompanhamento/"+strconv.FormatInt(order.Code, 10)+"/orcamento/aprovar", nil)
	request.Header.Set(trackingTokenHeader, "the-real-token")
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, request)
	assert.Equal(t, http.StatusOK, recorder.Code)
}
