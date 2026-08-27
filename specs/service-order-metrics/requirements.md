# Requirements — Average Service Execution Time Metric

Status: **Approved for implementation**
Feature folder: `internal/features/service-order/` (extends the existing package — reads
the same `audit_services`/`ServiceExecution` data `specs/service-order-execution/` already
writes, not a new feature package; see `design.md` §1.1 for why).

## 1. Context

Source: ticket "FP-22 - Métrica de tempo médio de execução" (RF09, the "monitor average
execution time" requirement, RNF02, RNF04).

`specs/service-order-execution/` already persists one `audit_services` row per execution of
a service within a service order, with its own `started_at`/`ended_at` (`ended_at` is `NULL`
while the execution is still in progress). No feature reads that data back analytically yet
— this feature adds exactly that: a read-only endpoint reporting, per service, how many
executions completed and their average duration.

## 2. User story

> As a workshop manager,
> I want to query the average execution time of services,
> so that I can track the workshop's operational efficiency.

## 3. Business rules

1. Average duration is `ended_at - started_at` for each qualifying execution.
2. Only **completed** executions (`ended_at IS NOT NULL`) participate in the average.
3. Executions still in progress (`ended_at IS NULL`) are excluded — never treated as zero or
   as an error.
4. The response is grouped by service: one entry per `Service` that has at least one
   qualifying execution, each carrying its own execution count and average duration.
5. Duration is reported in **minutes**, as a floating-point number
   (`averageDurationMinutes`), not seconds — an explicit choice recorded in `design.md` §1.4
   since the ticket left the unit open.
6. When no execution qualifies (no data at all, or a filter that matches nothing), the
   response is a valid `200` with an empty list — never an error and never a `404`.
7. Two optional, combinable (AND) filters are accepted:
   - `serviceId` — restrict the result to a single service (still returned in the same
     grouped-list shape, with zero or one entry).
   - `startDate`/`endDate` — restrict to executions whose `started_at` falls in that
     (inclusive) range.
8. The endpoint is administrative and requires a valid JWT (RNF02) — this project has no
   role/permission model (`CLAUDE.md` §13), so "administrative" means any caller holding a
   valid token issued by `POST /api/v1/auth/login`, the same meaning it has for every other
   JWT-protected route in this project.
9. The response envelope follows RNF04 — the same `apierror`-based error shape and plain-JSON
   success shape `service-order`'s other read endpoints (`GET /api/v1/service-orders`) already
   use (`CLAUDE.md` §8).

## 4. Scope

In scope:
- `GET /api/v1/service-orders/metrics/average-execution-time` — the grouped-by-service
  metric, with the `serviceId`/`startDate`/`endDate` filters.
- OpenAPI documentation of the endpoint.

Out of scope (explicitly not implemented by this feature):
- Any write operation — this feature is read-only.
- Metrics beyond average execution time (e.g. median, percentiles, cost) — not requested by
  the ticket.
- Filtering by service order, customer, or vehicle — the ticket's filters are service and
  date range only.
- Pagination — the result set is bounded by the number of distinct services in the catalog,
  not by execution volume (each execution collapses into its service's aggregate row), so no
  feature in this project's precedent (`customer`/`vehicle`/`product`/`service-order` listing)
  applies pagination to an aggregated result; revisit only if the catalog grows large enough
  to matter.

## 5. Endpoint covered (contract details in `design.md`)

- `GET /api/v1/service-orders/metrics/average-execution-time?serviceId=&startDate=&endDate=`

Requires `Authorization: Bearer <token>` (RNF02).

## 6. Acceptance criteria

1. `GET .../metrics/average-execution-time` with no filters returns one entry per service
   that has at least one completed execution, each with `executionCount` and
   `averageDurationMinutes`.
2. An execution still in progress (`ended_at` null) is excluded from both the count and the
   average for its service.
3. `?serviceId=<id>` restricts the result to that service (or an empty list if it has no
   completed executions).
4. `?startDate=&endDate=` restricts to executions started within that inclusive range.
5. Filters combine (AND) when more than one is supplied.
6. No data (empty database, or a filter matching nothing) returns `200` with `{"services":
   []}` — never an error.
7. The response documents `averageDurationMinutes` as minutes (design.md/OpenAPI).
8. The route returns `401` without a valid bearer token.
9. `docs/openapi.yaml` documents the endpoint, its filters, and response schema.

## 7. Open questions / discoveries resolved before implementation

- **Route path**: the ticket's suggested Portuguese path
  (`/api/v1/metricas/tempo-medio-servicos`) was not used. Confirmed with the requester:
  English, under the existing `/api/v1/service-orders` prefix
  (`GET /api/v1/service-orders/metrics/average-execution-time`) — same reasoning
  `specs/service-order-query/requirements.md` §7 already recorded for reusing that prefix
  instead of opening a second, parallel URL space for the same resource.
- **Duration unit**: the ticket left it open ("preferably seconds or minutes"). Confirmed
  with the requester: minutes.
- **Package placement**: confirmed with the requester to extend
  `internal/features/service-order/` rather than a new `internal/features/metrics/` package,
  since the data this feature reads (`audit_services`) already lives there
  (`execution_repository.go`) — see `design.md` §1.1.
