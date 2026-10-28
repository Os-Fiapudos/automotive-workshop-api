// Package usecase is the application layer of the service-order feature: one
// struct per group of use cases (Opening, Lifecycle, Quotes, Executions,
// Queries, StockUsage, Metrics), the raw inputs and read models they exchange
// with the adapters, and the ports (ports.go) they depend on.
//
// Ports are declared here, by their consumer, and kept narrow; concrete
// implementations live in adapter/postgres and adapter/notifier and are
// composed in cmd/api/main.go. This package depends only on domain, the
// standard library, github.com/google/uuid, infrastructure-free shared helpers
// (internal/shared/document, internal/shared/trackingtoken) and the
// internal/shared/money value type — never on net/http, encoding/json, pgx,
// internal/shared/apierror or any adapter.
// internal/features/service-order/architecture_test.go enforces this,
// transitively (specs/service-order-architecture-refactor/).
package usecase
