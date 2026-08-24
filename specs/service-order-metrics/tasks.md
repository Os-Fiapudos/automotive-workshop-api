# Tasks — Average Service Execution Time Metric

Each task references the `design.md` section it implements.

## 1. Spec traceability check

- [x] `requirements.md` and `design.md` written and internally consistent; the three open
      decisions (route path, duration unit, package placement, `requirements.md` §7) resolved
      with the requester before writing any code.

## 2. Persistence (`design.md` §1.6/§1.7)

- [x] Add `findAverageExecutionTimeByService` to `serviceOrderLookups` (`repository.go`).
- [x] Implement it on `PostgresServiceOrderRepository` (`metrics_repository.go`), the
      aggregate query from `design.md` §1.6.

## 3. Application layer (`design.md` §1.8)

- [x] `MetricsFilter`, `ServiceMetric` types (`metrics_service.go`).
- [x] `ServiceOrderService.AverageExecutionTime`.

## 4. API layer (`design.md` §1.9)

- [x] `metrics_dto.go`: `serviceMetricResponse`, `AverageExecutionTimeResponse`,
      `toAverageExecutionTimeResponse` (empty slice, never `nil`).
- [x] `handler.go`: `averageExecutionTime` method; `serviceId`/`startDate`/`endDate` query
      parsing and validation, mirroring `parseListFilter`'s pattern.

## 5. Wiring (`design.md` §1.2)

- [x] Register `GET /api/v1/service-orders/metrics/average-execution-time` in
      `RegisterRoutes`, wrapped in `requireAuth`.
- [x] Verify the `ServeMux` registration actually succeeds (existing
      `RegisterRoutes`-against-a-real-mux test already covers this — extend rather than
      duplicate it).

## 6. Tests (`design.md` §2)

- [x] Extend `fake_repository_test.go` with `findAverageExecutionTimeByService`.
- [x] `metrics_service_test.go`: multiple completed executions average correctly;
      in-progress execution excluded; no data → empty list; `serviceId` filter; date-range
      filter; combined filters.
- [x] `internal/handlers_test/service_order_metrics_test.go`: seeds executions through the
      real execution endpoints, asserts the metric response; empty-database case; `401`
      without a token.

## 7. Documentation (`design.md` §3)

- [x] `docs/openapi.yaml`: new path, query parameters, response schema, `bearerAuth`.
- [x] `specs/architecture.md`: addendum note recording the new endpoint.

## 8. Verification

- [x] `go build ./...`, `go vet ./...`, `go test ./...` all pass.
- [x] Every `requirements.md` acceptance criterion (§6) re-checked against the implemented
      behavior.
- [x] No change outside this feature's scope (no touching `product`/`customer`/`vehicle`/
      `servicecatalog` code, no change to any existing `service-order` route's behavior).
