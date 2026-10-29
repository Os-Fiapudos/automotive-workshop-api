// Package domain is the innermost layer of the service-order feature: the
// ServiceOrder aggregate and its status transitions, quotes, executions,
// stock movements, history entries and the sentinel errors every outer layer
// reports.
//
// It depends only on the standard library, github.com/google/uuid and the
// internal/shared/money value type (requirements.md §7.7) — never on
// net/http, encoding/json, pgx, internal/shared/apierror, the usecase package
// or any adapter. internal/features/service-order/architecture_test.go
// enforces this, transitively (specs/service-order-architecture-refactor/).
package domain
