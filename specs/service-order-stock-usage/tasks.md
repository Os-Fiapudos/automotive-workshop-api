# Tasks — Service Order Stock Usage (FP-19)

Each task references the `design.md` section it implements.

1. **Schema** (design.md §0) — add `stock_movement_type` enum and `stock_movements` table
   to `docs/schema.sql`, plus indexes. Update `docs/entities.md` with the new `StockMovement`
   entity and `StockMovementType` enum. Recreate the local Docker volume to apply it.
2. **Domain** (design.md §2) — new `stockusage_model.go`: `StockMovementType`,
   `StockMovement`, `StockUsageItem`, `validateStockUsageItems`.
3. **Errors** (design.md §7) — add `ErrEmptyStockUsage`, `ErrInsufficientStock`,
   `ErrStockMovementNotFound`, `ErrStockMovementAlreadyReversed`,
   `ErrStockMovementNotReversible` to `errors.go` (reusing `ErrInvalidQuantity`/
   `ErrProductNotFound`/`ErrProductInactive`/`ErrInvalidStatusTransition`).
4. **Repository interface** (design.md §1.1, §4) — extend `ServiceOrderRepository` with
   `RegisterStockUsage`/`ReverseStockMovement`/`ListStockMovements` in `repository.go`.
5. **Repository implementation** (design.md §4) — new `stockusage_repository.go`
   implementing the three methods, transactional per §4.1/§4.2.
6. **Service layer** (design.md §3) — new `stockusage_service.go`: `RegisterStockUsage`,
   `ReverseStockMovement`, `ListStockMovements`.
7. **DTOs** (design.md §5) — new `stockusage_dto.go`: request/response types + `Validate`.
8. **HTTP support** (design.md §7) — add the five new error mappings to `httpsupport.go`.
9. **Handlers + routes** (design.md §1.2) — add the three handlers and their
   `requireAuth`-wrapped routes to `handler.go`.
10. **Fakes** (design.md §9) — extend `fake_repository_test.go` with in-memory stock/movement
    storage backing the three new repository methods, replicating the guarded-update
    disambiguation so unit tests can exercise it without a database.
11. **Unit tests** — `stockusage_service_test.go` (BR1, BR3, empty items).
12. **Product package touch** (design.md §6) — change `ProductRepository.AdjustStock`'s
    signature, wrap it in a transaction that also inserts into `stock_movements`, add
    `ListMovements`, wire `handler.listMovements` to call it. Update
    `internal/features/product/fake_repository_test.go` to match.
13. **Integration tests** (design.md §9) — add HTTP-level tests to
    `internal/handlers_test/service_order_test.go` covering the full acceptance checklist in
    `requirements.md` §6, reusing `insertProduct`/`productStock`/`moveServiceOrderToEmExecucao`.
14. **Architecture doc** (design.md §8) — append the new addendum block to
    `specs/architecture.md`.
15. **Verification** — `go build ./...`, `go vet ./...`, `go test ./...` all green; walk
    `requirements.md` §6's checklist and confirm every item is covered by a test.
