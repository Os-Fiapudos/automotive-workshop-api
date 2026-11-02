package httpapi

import (
	"net/http"

	"automotive-workshop-api/internal/features/service-order/usecase"
)

const (
	defaultPage     = 1
	defaultPageSize = 20
	maxPageSize     = 100
)

// trackingTokenHeader is the header the customer-facing quote decision
// endpoints read the tracking token from — same header/constant name
// service-order-tracking already established for its own endpoint
// (requirements.md §0 item 2), duplicated here rather than imported since
// importing that feature's package would introduce the cross-feature
// coupling CLAUDE.md §9.2 forbids.
const trackingTokenHeader = "X-Tracking-Token"

// externalNotificationSecretHeader carries the shared secret the external
// quote-decision channel authenticates with
// (specs/service-order-quote-decision/design.md §5.1).
const externalNotificationSecretHeader = "X-External-Notification-Secret"

// externalStatusSecretHeader carries the shared secret the external
// status-update tool authenticates with — a different secret from the
// quote-decision one (specs/service-order-status-update/design.md §3).
const externalStatusSecretHeader = "X-External-Status-Secret" // #nosec G101 -- HTTP header name, not a credential

// RegisterRoutes registers every Service Order Opening/Diagnosis/Quote
// endpoint on mux, using the same Go 1.22 method-aware ServeMux pattern as
// every other feature (see specs/service-order-opening/design.md §1.5).
// requireAuth wraps every route, including order creation (RNF02 /
// specs/auth/design.md §7's "every non-public route requires auth"
// convention — see CLAUDE.md §17, the open decision this closes).
// requireAuth may be nil, same nil-safe convention as
// internal/features/product.RegisterRoutes, for callers that deliberately
// need an unauthenticated router (e.g. isolated fixture setup in tests).
func RegisterRoutes(mux *http.ServeMux, useCases UseCases, requireAuth func(http.Handler) http.Handler) {
	handler := &serviceOrderHandler{useCases: useCases}

	wrap := func(h http.HandlerFunc) http.Handler {
		if requireAuth != nil {
			return requireAuth(h)
		}
		return h
	}

	mux.Handle("POST /api/v1/service-orders", wrap(handler.create))
	mux.Handle("POST /api/v1/service-orders/{id}/diagnosis", wrap(handler.startDiagnosis))
	mux.Handle("PUT /api/v1/service-orders/{id}/quote", wrap(handler.composeQuote))
	mux.Handle("GET /api/v1/service-orders/{id}/quote", wrap(handler.getQuote))

	// Added by specs/service-order-quote-decision/. Sending is a mechanic
	// action (requireAuth-wrapped, same as diagnosis/compose above); approve/
	// reject are customer-facing under the same unauthenticated
	// /acompanhamento namespace specs/service-order-tracking/ already
	// established, validating their own tracking token instead of the
	// administrative JWT (RF12).
	mux.Handle("POST /api/v1/service-orders/{id}/quote/send", wrap(handler.sendQuote))
	mux.HandleFunc("POST /api/v1/acompanhamento/{codigo}/orcamento/aprovar", handler.approveQuote)
	mux.HandleFunc("POST /api/v1/acompanhamento/{codigo}/orcamento/reprovar", handler.rejectQuote)

	// Added by specs/service-order-quote-decision/ §9 (Fase 2): an external
	// system relays the customer's decision. Deliberately NOT wrapped in
	// requireAuth — the caller is not a logged-in administrator; the handler
	// authenticates it with X-External-Notification-Secret instead, before
	// touching the order (design.md §5.1/§5.3).
	mux.HandleFunc("POST /api/v1/service-orders/{id}/quote/decision", handler.decideQuoteExternal)

	// Added by specs/service-order-status-update/ (Fase 2): an external tool
	// (the n8n workflow in tools/n8n/) triggers IN_DIAGNOSIS/COMPLETED/
	// DELIVERED. Not wrapped in requireAuth for the same reason as the route
	// above; the handler checks X-External-Status-Secret first (design.md §3).
	mux.HandleFunc("POST /api/v1/service-orders/{id}/status", handler.updateStatusExternal)

	// Added by specs/service-order-query/ (RNF02: every route here requires
	// a valid JWT, unlike the create route above). {id} accepts either a
	// UUID or a sequential code (design.md §1.2) — a separate
	// GET .../code/{code} route is not implementable under this prefix, see
	// that section for why.
	mux.Handle("GET /api/v1/service-orders", wrap(handler.list))
	mux.Handle("GET /api/v1/service-orders/{id}", wrap(handler.get))

	// Added by specs/service-order-execution/ — all four requireAuth-wrapped,
	// per specs/auth/design.md §7's "every new route requires auth" convention.
	mux.Handle("POST /api/v1/service-orders/{id}/executions", wrap(handler.startExecution))
	mux.Handle("POST /api/v1/service-orders/{id}/executions/{executionId}/finish", wrap(handler.finishExecution))
	mux.Handle("POST /api/v1/service-orders/{id}/finalize", wrap(handler.finalizeOrder))
	mux.Handle("POST /api/v1/service-orders/{id}/deliver", wrap(handler.deliverOrder))

	// Added by specs/service-order-metrics/ (requireAuth-wrapped, RNF02). A
	// literal path segment ("metrics"), so it cannot conflict with the
	// {id}-shaped patterns above (design.md §1.2).
	mux.Handle("GET /api/v1/service-orders/metrics/average-execution-time", wrap(handler.averageExecutionTime))

	// Added by specs/service-order-stock-usage/ — all three requireAuth-wrapped,
	// per specs/auth/design.md §7's "every new route requires auth" convention.
	mux.Handle("POST /api/v1/service-orders/{id}/stock-movements", wrap(handler.registerStockUsage))
	mux.Handle("GET /api/v1/service-orders/{id}/stock-movements", wrap(handler.listStockMovements))
	mux.Handle("POST /api/v1/service-orders/{id}/stock-movements/{movementId}/reversal", wrap(handler.reverseStockMovement))
}

// UseCases are the application use cases the HTTP routes call, built and
// composed in cmd/api/main.go (specs/service-order-architecture-refactor/
// design.md §9.3).
type UseCases struct {
	Opening    *usecase.Opening
	Lifecycle  *usecase.Lifecycle
	Quotes     *usecase.Quotes
	Executions *usecase.Executions
	Queries    *usecase.Queries
	StockUsage *usecase.StockUsage
	Metrics    *usecase.Metrics
}

type serviceOrderHandler struct {
	useCases UseCases
}
