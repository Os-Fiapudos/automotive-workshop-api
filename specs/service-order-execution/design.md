# Design — Service Order Execution, Finalization, and Delivery

Satisfies: `requirements.md` (all sections). Extends `specs/service-order-diagnosis-quote/design.md`
and `specs/service-order-query/design.md` rather than reopening their decisions (package
layout, error envelope, transaction shape, testing conventions).

## 1. Architecture decisions

### 1.1 Same package, not a new feature

Same reasoning `service-order-diagnosis-quote/design.md` §1.1 already gives: this feature
operates on the same `ServiceOrder` aggregate `service-order-opening` created, so it adds
new files to the existing `internal/features/service-order/` package instead of a new
`internal/features/service-order-execution/` one. New use-case logic goes in `execution_*.go`
files, mirroring the existing `quote_*.go`/`query_*.go` split:

```
internal/features/service-order/
├── model.go                → + Status consts (IN_PROGRESS/COMPLETED/DELIVERED), finalize()/
│                                deliver() transition methods
├── execution_model.go       → ServiceExecution type + constructor + finish()
├── execution_dto.go         → start/finish execution request+response DTOs
├── errors.go                → + new sentinel errors
├── execution_repository.go  → + StartExecution/FinishExecution/FinalizeOrder/DeliverOrder +
│                                findServiceExecutionByID/findServiceExecutionsByServiceOrderID
├── execution_service.go     → + StartExecution/FinishExecution/FinalizeOrder/DeliverOrder use cases
├── handler.go                → + 4 new routes/handlers
├── httpsupport.go            → + new error mappings + decodeOptionalJSON helper
├── repository.go             → + interface method additions
├── execution_model_test.go
├── execution_service_test.go
├── fake_repository_test.go   → extended with the new fake methods
```

### 1.2 Endpoints and routing

```
POST /api/v1/service-orders/{id}/executions                       → start execution
POST /api/v1/service-orders/{id}/executions/{executionId}/finish  → finish execution
POST /api/v1/service-orders/{id}/finalize                          → finalize order
POST /api/v1/service-orders/{id}/deliver                           → deliver order
```

All four registered in `RegisterRoutes`, wrapped in `requireAuth` — every route
`service-order-diagnosis-quote`/`service-order-query` added was wrapped, and
`specs/auth/design.md` §7's "every new route requires auth unless explicitly public"
convention applies here too. Named in English (`executions`/`finish`/`finalize`/`deliver`),
matching `requirements.md` §5's naming decision.

### 1.3 `ServiceExecution` persistence: restructuring `audit_services`

`docs/schema.sql`'s `audit_services` table today is an append-only event log (`event`:
`start`|`end`, one row per event, no link between a service's start row and its end row).
That shape cannot support this feature's contract: `POST .../executions` must return a
stable `executionId` the client then calls `POST .../executions/{executionId}/finish`
with, and finishing must validate that specific execution's own end-time against its own
start-time (BR4) — neither is expressible by matching loose event rows.

`audit_services` has no Go feature reading or writing it yet (`CLAUDE.md` §1), so this is
its first real implementation, not a change to working code. This design restructures it
to one row per execution, with its own start and end columns, keeping the table name (a
schema rename felt like unnecessary churn given the table has never been implemented) but
dropping the `event`/`occurred_at` columns and the now-unused `audit_event` enum:

```sql
CREATE TABLE IF NOT EXISTS audit_services (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    service_order_id  UUID NOT NULL REFERENCES service_orders (id) ON DELETE CASCADE,
    service_id        UUID NOT NULL REFERENCES services (id) ON DELETE RESTRICT,
    started_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    ended_at          TIMESTAMPTZ
);
```

`docs/entities.md`'s `AuditServices` section is updated to match (§4 below), and the Go
type is named `ServiceExecution` (English; the ticket's `ExecucaoServico`), per
`requirements.md` §6.

### 1.4 `service_order_history` — one new event value

`COMPLETED` reuses the existing `completion` event value (already documented as "OS
completion" in `docs/schema.sql`'s `history_event` comment, unused by any feature so far).
`DELIVERED` has no existing value, so `history_event` gains one more: `'delivery'`.
Starting/finishing an execution does **not** write a `service_order_history` row —
`requirements.md` BR8 explains why (the order's status does not change).

### 2. Domain layer

### 2.1 `ServiceOrder` additions (`model.go`)

```go
const (
    StatusInProgress Status = "IN_PROGRESS"
    StatusCompleted Status = "COMPLETED"
    StatusDelivered   Status = "DELIVERED"
)
```

Two new transition methods, same shape as `startDiagnosis`/`markAwaitingApproval`:

```go
func (order *ServiceOrder) finalize() error {
    if order.Status != StatusInProgress {
        return ErrInvalidStatusTransition
    }
    order.Status = StatusCompleted
    return nil
}

func (order *ServiceOrder) deliver() error {
    if order.Status != StatusCompleted {
        return ErrInvalidStatusTransition
    }
    order.Status = StatusDelivered
    return nil
}
```

Starting/finishing an execution requires `order.Status == StatusInProgress` too (BR2, BR6)
but does not itself change `Status` — that guard lives in the service layer
(`execution_service.go`), checked with a plain equality, not a new aggregate method, since
there is no state to transition.

`ErrInvalidStatusTransition` is reused (not a new sentinel) for every "order is not in the
right status for this operation" case across this feature — `finalize`, `deliver`,
starting an execution, and finishing an execution — matching how `startDiagnosis` already
uses it and keeping `httpsupport.go`'s mapping (`Conflict` → HTTP 409) as the single answer
to the checklist's "invalid transition returns 409 or 422".

### 2.2 `ServiceExecution` (`execution_model.go`)

```go
type ServiceExecution struct {
    ID             uuid.UUID
    ServiceOrderID uuid.UUID
    ServiceID      uuid.UUID
    StartedAt      time.Time
    EndedAt        *time.Time
}

func NewServiceExecution(serviceOrderID, serviceID uuid.UUID) (*ServiceExecution, error) {
    if serviceOrderID == uuid.Nil || serviceID == uuid.Nil {
        return nil, ErrInvalidAggregate
    }
    return &ServiceExecution{ID: uuid.New(), ServiceOrderID: serviceOrderID, ServiceID: serviceID}, nil
}

func (execution *ServiceExecution) finish(endedAt *time.Time) error {
    if execution.EndedAt != nil {
        return ErrServiceExecutionAlreadyFinished
    }
    if endedAt != nil && endedAt.Before(execution.StartedAt) {
        return ErrServiceExecutionEndBeforeStart
    }
    execution.EndedAt = endedAt
    return nil
}
```

`StartedAt` is always server-authored (DB `now()`, same convention as `ServiceOrder.OpenedAt`/
`Quote.GeneratedAt` — the client never supplies a date the server would otherwise generate).
`EndedAt` accepts an optional client-supplied value (§2.5) — the only way BR4's "end before
start" rejection is a reachable, testable case.

**Both default timestamps come from the same clock, deliberately.** An earlier version of
this design had the service layer default a nil `endedAt` to Go's `time.Now()`, compared
against a `StartedAt` that was authored by the database's `now()` at insert time. Those are
two different clocks, and they measurably drift — confirmed locally, where the Postgres
container's clock ran about a second ahead of the host running the Go process — which made
BR4's check spuriously fire on a same-request start-then-finish with no real delay between
them. The same risk exists in any real deployment where the API process and the database
run on different machines. The fix: `finish(endedAt *time.Time)` only ever compares an
**explicit, caller-supplied** `endedAt` against `StartedAt`; a `nil` is passed straight
through, and `execution_repository.go`'s `FinishExecution` resolves it with
`COALESCE($2, now())` — the database's own clock, the same one that produced `StartedAt` —
so the two values being compared are, by construction, never drawn from different clocks
when the server is the one choosing the end time.

### 2.3 Required-executions rule (BR5) — `execution_service.go`

"Required executions" is defined as: the distinct set of service ids that appear as
`QuoteItemService` line items of the order's approved quote. `FinalizeOrder` loads the
quote, collects that set, loads every `ServiceExecution` for the order, and requires each
required service id to have at least one execution with `EndedAt != nil`. An order with no
service line items in its quote (products only) has no required executions and can be
finalized as soon as it is `IN_PROGRESS`.

### 2.4 Service-layer flow

```go
func (service *ServiceOrderService) StartExecution(ctx, serviceOrderID, serviceID uuid.UUID) (*ServiceExecution, error)
func (service *ServiceOrderService) FinishExecution(ctx, serviceOrderID, executionID uuid.UUID, endedAt *time.Time) (*ServiceExecution, error)
func (service *ServiceOrderService) FinalizeOrder(ctx, serviceOrderID uuid.UUID) (*ServiceOrder, error)
func (service *ServiceOrderService) DeliverOrder(ctx, serviceOrderID uuid.UUID) (*ServiceOrder, error)
```

`StartExecution`/`FinishExecution` both re-load the order and check
`order.Status == StatusInProgress` before touching the execution (BR2/BR6).
`StartExecution` also validates the given service id exists in the catalog, reusing
`findServiceByID` (no new lookup) — same existence-only check
`service-order-diagnosis-quote` already applies to quote service items; this feature does
not additionally require the service to be one of the quote's line items (not stated by
any acceptance criterion — see `requirements.md` §2.1's "don't invent" note).

### 2.5 Request/response shapes (`execution_dto.go`)

```jsonc
// POST /api/v1/service-orders/{id}/executions
{ "serviceId": "<uuid>" }
// → 201, Location: /api/v1/service-orders/{id}/executions/{executionId}
{ "id": "...", "serviceOrderId": "...", "serviceId": "...", "startedAt": "...", "endedAt": null }

// POST /api/v1/service-orders/{id}/executions/{executionId}/finish
{ "endedAt": "<RFC3339, optional>" }   // omitted/empty body → server uses now()
// → 200
{ "id": "...", "serviceOrderId": "...", "serviceId": "...", "startedAt": "...", "endedAt": "..." }

// POST /api/v1/service-orders/{id}/finalize
// (no body) → 200 { "id": "...", "status": "COMPLETED" }

// POST /api/v1/service-orders/{id}/deliver
// (no body) → 200 { "id": "...", "status": "DELIVERED" }
```

`finalize`/`deliver` reuse the existing `serviceOrderStatusResponse` shape
`startDiagnosis` already returns — no new response type needed.

The finish-execution and finalize/deliver handlers accept an empty request body (the
existing `decodeJSON[T]` helper errors on a zero-length body, since `json.Decode` returns
`io.EOF`). `httpsupport.go` gets a `decodeOptionalJSON[T]` helper — identical to
`decodeJSON[T]` except an `io.EOF` is treated as "no body sent" rather than an error — used
by the finish-execution handler for its optional `endedAt` field.

### 2.6 Repository (`execution_repository.go`)

```go
StartExecution(ctx, execution *ServiceExecution) error   // single INSERT, no transaction needed
FinishExecution(ctx, execution *ServiceExecution) error  // single UPDATE ... SET ended_at = COALESCE($2, now()) WHERE id = $1 AND ended_at IS NULL RETURNING ended_at
FinalizeOrder(ctx, order *ServiceOrder) error             // transactional: UPDATE service_orders + INSERT service_order_history
DeliverOrder(ctx, order *ServiceOrder) error              // transactional: UPDATE service_orders + INSERT service_order_history
```

`FinalizeOrder`/`DeliverOrder` follow `StartDiagnosis`'s exact shape (RNF07): `Begin`,
`defer Rollback`, `UPDATE service_orders SET status = $2 WHERE id = $1 AND status = '<expected>'`
(closing the same concurrent-transition race `StartDiagnosis` already closes — zero rows
affected is treated as `ErrInvalidStatusTransition`), `INSERT INTO service_order_history`,
`Commit`. `FinishExecution`'s `ended_at IS NULL` guard closes the equivalent race for
finishing the same execution twice concurrently, mapped to
`ErrServiceExecutionAlreadyFinished` on zero rows affected.

## 3. Error handling (`errors.go` additions + `httpsupport.go` mapping)

| Sentinel error | HTTP mapping |
| --- | --- |
| `ErrServiceExecutionNotFound` | 404 `NotFound` |
| `ErrServiceExecutionAlreadyFinished` | 409 `Conflict("SERVICE_EXECUTION_ALREADY_FINISHED", ...)` |
| `ErrServiceExecutionEndBeforeStart` | 400 `Validation(...)` — input validation, same bucket as `ErrInvalidQuantity`/`ErrEmptyQuote`, not a state-transition conflict |
| `ErrExecutionsNotCompleted` | 409 `Conflict("EXECUTIONS_NOT_COMPLETED", ...)` |
| `ErrInvalidStatusTransition` (reused) | 409 `Conflict("INVALID_STATUS_TRANSITION", ...)` — already mapped |

## 4. Docs kept in sync (CLAUDE.md §10/§14)

- `docs/entities.md`: `AuditServices` section rewritten to the started-at/ended-at shape
  (§1.3 above); a short note added since its field list changes from the current
  event/occurred_at shape.
- `docs/schema.sql`: `audit_services` table redefined, `audit_event` enum removed,
  `'delivery'` added to `history_event`.
- `specs/architecture.md`: new addendum block appended (matching the existing
  `service-order-tracking`/`service-order-query` addendum style), documenting the four new
  routes and the quote-approval gap from `requirements.md` §2.1.

## 5. Testing (requirements.md §7's last checklist item)

- Unit: `execution_model_test.go` — `finish()`'s already-finished/end-before-start guards;
  `model_test.go` additions — `finalize()`/`deliver()`'s status guards.
- Unit (service layer, fake repository): `execution_service_test.go` — BR2 (must be
  `IN_PROGRESS`), BR5 (required executions), BR6 (finalized order rejects new/finishing
  executions), BR7 (must be `COMPLETED` to deliver).
- Integration (`internal/handlers_test/service_order_test.go`): full HTTP round trip
  against a real Postgres, using a new `moveServiceOrderToInProgress` SQL test helper (same
  "insert the missing precondition directly via SQL" pattern `insertVehicle`/`insertProduct`
  already use for gaps the API can't fill) to reach `IN_PROGRESS`, since — per
  `requirements.md` §2.1 — no endpoint can produce that state yet.
