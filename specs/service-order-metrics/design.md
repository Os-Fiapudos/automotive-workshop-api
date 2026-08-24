# Design — Average Service Execution Time Metric

Satisfies: `requirements.md` (all sections). Extends `specs/service-order-execution/design.md`
rather than reopening its decisions (the `audit_services` shape, package layout, error
envelope, testing conventions).

## 1. Architecture decisions

### 1.1 Same package, not a new feature

This feature adds one read-only handler to `internal/features/service-order/` (Go package
`serviceorder`), the package `specs/service-order-execution/` already extended with
`ServiceExecution`/`audit_services` access. It reads exactly that data (`ended_at IS NOT
NULL` executions, joined to `services` for the name), so putting it in a new
`internal/features/metrics/` package would just re-declare a second read path to the same
table — same reasoning `specs/service-order-query/design.md` §1.1 already gives for its own
listing/detail endpoints. Confirmed with the requester (`requirements.md` §7).

New files, following the existing `query_*.go`/`execution_*.go` split-by-use-case-group
convention:
- `metrics_dto.go` — request filter parsing result + response DTOs.
- `metrics_service.go` — `MetricsFilter` and `ServiceOrderService.AverageExecutionTime`.
- `metrics_repository.go` — the Postgres aggregate query.
- `handler.go` gets one new method (`averageExecutionTime`) and one new registration in
  `RegisterRoutes`.

### 1.2 Route

```
GET /api/v1/service-orders/metrics/average-execution-time?serviceId=&startDate=&endDate=
```

Confirmed with the requester (`requirements.md` §7): English, under the existing
`/api/v1/service-orders` prefix, not the ticket's suggested
`/api/v1/metricas/tempo-medio-servicos`.

No `ServeMux` pattern conflict risk here unlike `service-order-query`'s `{id}` vs.
`{id}/quote` case (`specs/architecture.md` decision 18): `metrics/average-execution-time` is
a literal path with no wildcard segment, so it cannot collide with `{id}`-shaped patterns —
Go's `http.ServeMux` treats a literal segment as more specific than a wildcard at the same
position. Still verified by actually registering the route on a real `*http.ServeMux`
(`tasks.md` §6), per that same decision's lesson: `go build`/`go vet`/`go test` don't catch
this class of bug, only a real registration call does.

Wrapped in `requireAuth`, like every other route this package added after
`service-order-opening` (`specs/auth/design.md` §7, `requirements.md` BR8).

### 1.3 Filters

- `serviceId` — a UUID; malformed value → `400` (`apierror.Validation`), same convention
  `service-order-query`'s `parseListFilter` already uses for its own filters.
- `startDate`/`endDate` — parsed as `RFC3339` date-times, same format/validation convention
  `service-order-query`'s `createdFrom`/`createdTo` already established (`design.md` §1.3 of
  that spec) — reusing an existing precedent rather than inventing a bare-date (`YYYY-MM-DD`)
  format for this endpoint alone.
- All three combine with AND (`requirements.md` BR7).

### 1.4 Duration unit: minutes

Confirmed with the requester (`requirements.md` §7): `averageDurationMinutes`, a `float64`.
Computed in SQL as `EXTRACT(EPOCH FROM (ended_at - started_at)) / 60.0` — seconds divided by
60, not `DATE_PART('minute', ...)` (which would discard the hour component and truncate to
whole minutes instead of averaging fractional durations correctly).

### 1.5 Response shape

```json
{
  "services": [
    {
      "serviceId": "…",
      "serviceCode": 12,
      "serviceName": "Troca de óleo",
      "executionCount": 8,
      "averageDurationMinutes": 42.5
    }
  ]
}
```

A plain object wrapping a `services` array — not the `data`/`page`/`pageSize`/`total`/
`totalPages` envelope `service-order-query`'s listing uses, since this endpoint is
deliberately unpaginated (`requirements.md` §4). Empty list (`"services": []`), never `null`
and never a `404`, when nothing qualifies (BR6/AC6) — same "empty list is not an error"
convention `service-order-query`'s listing already follows for a filter matching nothing.

### 1.6 Query

```sql
SELECT s.id, s.code, s.name,
       COUNT(*) AS execution_count,
       AVG(EXTRACT(EPOCH FROM (a.ended_at - a.started_at)) / 60.0) AS average_duration_minutes
FROM audit_services a
JOIN services s ON s.id = a.service_id
WHERE a.ended_at IS NOT NULL
  AND ($1::uuid IS NULL OR a.service_id = $1)
  AND ($2::timestamptz IS NULL OR a.started_at >= $2)
  AND ($3::timestamptz IS NULL OR a.started_at <= $3)
GROUP BY s.id, s.code, s.name
ORDER BY s.name
```

`a.ended_at IS NOT NULL` is the one condition that satisfies both BR2 ("only completed
executions") and BR3 ("in-progress executions excluded") at once — there is no other state
an `audit_services` row can be in (`specs/service-order-execution/design.md` §1.3). No
`services` row without a qualifying execution appears in the result (`INNER JOIN`, not `LEFT
JOIN`) — matches BR4 ("one entry per service that **has** at least one qualifying
execution"), not "one entry per service in the catalog."

### 1.7 Persistence: `serviceOrderLookups` addition

```go
findAverageExecutionTimeByService(ctx context.Context, filter MetricsFilter) ([]*ServiceMetric, error)
```

Added to the existing `serviceOrderLookups` interface (`repository.go`) — a read-only
projection, same boundary `findRequestedServices`/`listServiceOrders` already sit behind, so
`metrics_service_test.go` can fake it exactly like `query_service_test.go` fakes its own
reads.

### 1.8 Application layer

```go
type MetricsFilter struct {
    ServiceID *uuid.UUID
    StartDate *time.Time
    EndDate   *time.Time
}

type ServiceMetric struct {
    ServiceID              uuid.UUID
    ServiceCode            int64
    ServiceName            string
    ExecutionCount         int
    AverageDurationMinutes float64
}

func (service *ServiceOrderService) AverageExecutionTime(ctx context.Context, filter MetricsFilter) ([]*ServiceMetric, error) {
    return service.lookups.findAverageExecutionTimeByService(ctx, filter)
}
```

A thin pass-through, same shape as `List`'s in `query_service.go` — no business logic beyond
what the query itself already encodes (BR1-BR4 are all satisfied in SQL, §1.6).

### 1.9 API layer

- `metrics_dto.go`: `serviceMetricResponse` + `AverageExecutionTimeResponse{Services
  []serviceMetricResponse}` and a `toAverageExecutionTimeResponse` builder — `Services` built
  with `make(..., 0, len(...))` so it marshals as `[]`, never `null`, on an empty result
  (BR6).
- `handler.go`: `averageExecutionTime` method; `serviceId` parsed with `uuid.Parse`,
  `startDate`/`endDate` with `time.Parse(time.RFC3339, ...)`, mirroring
  `parseListFilter`'s per-field `apierror.Detail` accumulation pattern exactly (reusing the
  existing validation-error shape rather than inventing a new one for this endpoint).

## 2. Testing strategy

Same two-layer convention every other use case in this package follows
(`specs/service-order-execution/design.md`'s testing section, `CLAUDE.md` §11):
- `metrics_service_test.go` — fake-repository unit tests: multiple completed executions of
  one service average correctly; an in-progress execution is excluded; no data at all →
  empty list; `serviceId` filter; date-range filter; combined filters.
- `internal/handlers_test/service_order_metrics_test.go` — real-database integration test,
  self-skipped without `DATABASE_URL`: seeds executions via the real
  `POST .../executions`/`.../executions/{id}/finish` endpoints (not direct SQL — exercising
  the same path production traffic uses, matching this package's existing integration-test
  convention), then asserts the metric endpoint's response; `401` without a token.

## 3. Documentation

`docs/openapi.yaml`: new path under the existing `service-orders` tag, its three query
parameters, response schema, `bearerAuth` requirement — same pattern
`service-order-query`'s own OpenAPI addition already followed.

`specs/architecture.md`: addendum note under its existing `service-order-execution` addendum
(§1 overview), recording this feature's endpoint the same way that one recorded its four
routes.
