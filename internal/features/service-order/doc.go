// Package serviceorder is the root of the service-order feature — opening a
// service order, diagnosis and quote, the customer's decision, execution,
// stock usage, queries, metrics and the external status update — organized in
// Clean Architecture layers inside this one vertical slice
// (specs/service-order-architecture-refactor/):
//
//	domain/            the ServiceOrder aggregate, its transitions and errors
//	usecase/           application use cases and the ports they declare
//	adapter/postgres/  persistence: implements the ports with pgx
//	adapter/httpapi/   HTTP: routes, DTOs, error-to-status mapping
//	adapter/notifier/  quote-sent e-mail: implements usecase.QuoteNotifier
//
// Dependencies only point inward (adapter -> usecase -> domain); the
// composition happens in cmd/api. architecture_test.go in this package
// enforces the rule transitively. The feature's behavior is specified in the
// specs/service-order-*/ folders.
package serviceorder
