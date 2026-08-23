# Tasks — Service Order Execution, Finalization, and Delivery

Each task references the `design.md` section it implements.

1. **Schema** (design.md §1.3, §1.4, §4) — update `docs/schema.sql`: redefine
   `audit_services` (drop `event`/`occurred_at`, add `started_at`/`ended_at`), drop the
   `audit_event` enum, add `'delivery'` to `history_event`. Update `docs/entities.md`'s
   `AuditServices` section to match. Recreate the local Docker volume to apply it.
2. **Domain — `ServiceOrder`** (design.md §2.1) — add `StatusEmExecucao`/`StatusFinalizada`/
   `StatusEntregue` consts and `finalize()`/`deliver()` methods to `model.go`.
3. **Domain — `ServiceExecution`** (design.md §2.2) — new `execution_model.go`:
   `ServiceExecution` type, `NewServiceExecution`, `finish()`.
4. **Errors** (design.md §3) — add `ErrServiceExecutionNotFound`,
   `ErrServiceExecutionAlreadyFinished`, `ErrServiceExecutionEndBeforeStart`,
   `ErrExecutionsNotCompleted` to `errors.go`.
5. **Repository interfaces** (design.md §1.1, §2.6) — extend `ServiceOrderRepository` with
   `StartExecution`/`FinishExecution`/`FinalizeOrder`/`DeliverOrder`, and
   `serviceOrderLookups` with `findServiceExecutionByID`/
   `findServiceExecutionsByServiceOrderID`, in `repository.go`.
6. **Repository implementation** (design.md §2.6) — new `execution_repository.go`
   implementing the five methods above against `pgx`, transactional where noted.
7. **Service layer** (design.md §2.3, §2.4) — new `execution_service.go`:
   `StartExecution`, `FinishExecution`, `FinalizeOrder` (required-executions check),
   `DeliverOrder`.
8. **DTOs** (design.md §2.5) — new `execution_dto.go`: `StartExecutionRequest` (+
   `Validate`), `FinishExecutionRequest`, `ServiceExecutionResponse` (+ `toServiceExecutionResponse`).
9. **HTTP support** (design.md §2.5, §3) — add `decodeOptionalJSON[T]` and the four new
   error mappings to `httpsupport.go`.
10. **Handlers + routes** (design.md §1.2) — add `startExecution`/`finishExecution`/
    `finalizeOrder`/`deliverOrder` handlers and their four `requireAuth`-wrapped routes to
    `handler.go`.
11. **Fakes** (design.md §5) — extend `fake_repository_test.go` with in-memory execution
    storage backing the five new repository methods.
12. **Unit tests** — `execution_model_test.go` (finish() guards), `model_test.go` additions
    (finalize()/deliver() guards), `execution_service_test.go` (BR2/BR5/BR6/BR7).
13. **Integration tests** (design.md §5) — add a `moveServiceOrderToEmExecucao` helper and
    HTTP-level tests to `internal/handlers_test/service_order_test.go` covering the full
    acceptance checklist in `requirements.md` §7.
14. **Architecture doc** (design.md §4) — append the new addendum block to
    `specs/architecture.md`.
15. **Verification** — `go build ./...`, `go vet ./...`, `go test ./...` all green; walk
    `requirements.md` §7's checklist and confirm every item is covered by a test.
