# Design — Service Order Stock Usage (FP-19)

Satisfies: `requirements.md` (all sections). Extends `specs/service-order-execution/design.md`
and `specs/service-order-quote-decision/design.md` rather than reopening their decisions
(package layout, error envelope, transaction shape, testing conventions).

## 0. Why a shared `stock_movements` table, not a new feature package

`internal/features/product/` already has a `StockMovement` domain type and a working
`ApplyStockAdjustment` (ENTRY/EXIT, active check, quantity > 0, non-negative balance) behind
`POST /api/v1/produtos/{id}/estoque/ajustes` — but it was a stub end to end before this
feature: `PostgresProductRepository.AdjustStock` only ran `UPDATE products SET
current_stock = ...`, and `GET /produtos/{id}/movimentacoes` always returned an empty array.
No movement was ever persisted.

Rather than let `service-order`'s new usage-deduction movements and `product`'s pre-existing
(but never-persisted) manual-adjustment movements become two different "stock movement"
concepts in the same domain, both write to one `stock_movements` table (`docs/schema.sql`),
distinguished by a nullable `service_order_id`. This also fixes `product`'s stub as a
byproduct (§6 below) — a small, explicitly-scoped touch to `product/repository.go`'s
`AdjustStock`, not a broader refactor of that feature.

Each feature still only accesses the table through its own SQL — `service-order` does not
import `internal/features/product`, and vice versa, matching how `service-order` already
reads/writes `products` directly (`quote_repository.go`'s `productRef`/`findActiveProductByID`)
without importing that package, per CLAUDE.md §9.2's "no direct coupling between features".

## 1. Architecture decisions

### 1.1 Same package, not a new feature

This feature operates on the same `ServiceOrder` aggregate `service-order-opening` created
and reuses its `EM_EXECUCAO` status, so it adds new files to the existing
`internal/features/service-order/` package, mirroring the existing `execution_*.go`/
`quote_*.go` split:

```
internal/features/service-order/
├── errors.go                → + new sentinel errors
├── stockusage_model.go      → StockMovement type, StockUsageItem, validation
├── stockusage_dto.go        → request/response DTOs
├── stockusage_repository.go → RegisterStockUsage/ReverseStockMovement/ListStockMovements
├── stockusage_service.go    → RegisterStockUsage/ReverseStockMovement/ListStockMovements use cases
├── handler.go                → + 3 new routes/handlers
├── httpsupport.go            → + new error mappings
├── repository.go             → + interface method additions
├── stockusage_service_test.go
├── fake_repository_test.go   → extended with the new fake methods
```

### 1.2 Endpoints and routing

```
POST /api/v1/service-orders/{id}/stock-movements                       → register usage deduction(s)
GET  /api/v1/service-orders/{id}/stock-movements                       → list movements for the order
POST /api/v1/service-orders/{id}/stock-movements/{movementId}/reversal → reverse a movement
```

All three registered in `RegisterRoutes`, `requireAuth`-wrapped (`requirements.md` §4).

## 2. Domain layer (`stockusage_model.go`)

```go
type StockMovementType string

const (
    StockMovementEntry StockMovementType = "ENTRY"
    StockMovementExit  StockMovementType = "EXIT"
)

type StockMovement struct {
    ID                 uuid.UUID
    ProductID          uuid.UUID
    ServiceOrderID     *uuid.UUID
    Type               StockMovementType
    Quantity           int
    PreviousStock      int
    NewStock           int
    Reason             string
    ReversedMovementID *uuid.UUID
    OccurredAt         time.Time
}

// StockUsageItem is one raw line of a usage-deduction request.
type StockUsageItem struct {
    ProductID string
    Quantity  int
}

func validateStockUsageItems(items []StockUsageItem) error {
    if len(items) == 0 {
        return ErrEmptyStockUsage
    }
    for _, item := range items {
        if item.Quantity <= 0 {
            return ErrInvalidQuantity // reused from quote items — same "must be > 0" rule
        }
    }
    return nil
}
```

No new `ServiceOrder` transition or method: BR1's "must be `EM_EXECUCAO`" is a plain status
equality check in the service layer, same shape `StartExecution`/`FinishExecution` already
use (`execution_service.go`) — there is no state to transition, only a precondition to gate.

## 3. Service layer (`stockusage_service.go`)

```go
func (service *ServiceOrderService) RegisterStockUsage(ctx, serviceOrderID uuid.UUID, items []StockUsageItem) ([]*StockMovement, error) {
    order, err := service.lookups.findServiceOrderByID(ctx, serviceOrderID)
    if err != nil {
        return nil, err
    }
    if order.Status != StatusEmExecucao {
        return nil, ErrInvalidStatusTransition
    }
    if err := validateStockUsageItems(items); err != nil {
        return nil, err
    }
    return service.repository.RegisterStockUsage(ctx, order.ID, items)
}

func (service *ServiceOrderService) ReverseStockMovement(ctx, serviceOrderID, movementID uuid.UUID) (*StockMovement, error) {
    if _, err := service.lookups.findServiceOrderByID(ctx, serviceOrderID); err != nil {
        return nil, err
    }
    return service.repository.ReverseStockMovement(ctx, serviceOrderID, movementID)
}

func (service *ServiceOrderService) ListStockMovements(ctx, serviceOrderID uuid.UUID) ([]*StockMovement, error) {
    if _, err := service.lookups.findServiceOrderByID(ctx, serviceOrderID); err != nil {
        return nil, err
    }
    return service.repository.ListStockMovements(ctx, serviceOrderID)
}
```

Product existence/active/balance checks are **not** duplicated here — they are enforced
atomically by the repository's guarded `UPDATE` (§4), the same split `DecideQuote`/
`StartDiagnosis` already use for status-transition guards, avoiding a check-then-act race
between a service-layer pre-check and the actual write (BR8).

## 4. Repository (`stockusage_repository.go`)

```go
RegisterStockUsage(ctx, orderID uuid.UUID, items []StockUsageItem) ([]*StockMovement, error)
ReverseStockMovement(ctx, orderID, movementID uuid.UUID) (*StockMovement, error)
ListStockMovements(ctx, orderID uuid.UUID) ([]*StockMovement, error)
```

### 4.1 `RegisterStockUsage` — single `pgx.Tx` (RNF07/BR6/BR7)

Same `Begin` / `defer Rollback` / `Commit`-once-at-the-end shape `DecideQuote` established
(`quote_repository.go`):

1. `SELECT status FROM service_orders WHERE id = $1` inside the transaction to re-confirm
   `EM_EXECUCAO` (closes the same race `StartDiagnosis`'s status guard closes) — zero/mismatched
   rows → `ErrServiceOrderNotFound`/`ErrInvalidStatusTransition`.
2. For each item, the same guarded, atomic `UPDATE` `product.AdjustStock`
   (`internal/features/product/repository.go:246`) already established for exactly this
   race (BR8 — a concurrent transaction's own guarded `UPDATE` cannot observe a stale
   balance, since Postgres serializes row-level writes):
   ```sql
   UPDATE products
   SET current_stock = current_stock - $2, updated_at = now()
   WHERE id = $1 AND status = 'ACTIVE' AND current_stock >= $2
   RETURNING current_stock + $2, current_stock
   ```
   Zero rows affected → a follow-up `SELECT status, current_stock FROM products WHERE id =
   $1` (still inside the transaction) disambiguates `ErrProductNotFound` /
   `ErrProductInactive` / `ErrInsufficientStock`, same disambiguation shape
   `product/repository.go:255-268` already uses.
3. `INSERT INTO stock_movements (product_id, service_order_id, type, quantity,
   previous_stock, new_stock) VALUES ($1, $2, 'EXIT', $3, $4, $5) RETURNING id, occurred_at`
   for that item.
4. Any failure at any item — `Rollback` (deferred) undoes every deduction already made in
   this call, since `Commit` is only ever reached after every item succeeds (BR7).
5. `Commit` once, at the end.

### 4.2 `ReverseStockMovement` — single `pgx.Tx` (BR9)

1. Load the original movement, scoped to `orderID` (`WHERE id = $1 AND service_order_id =
   $2`) so a client cannot reverse another order's movement by guessing its id — zero rows
   → `ErrStockMovementNotFound`.
2. Confirm it is reversible: `type = 'EXIT'` (an `ENTRY` reversal is never itself reversed —
   `requirements.md` §2.1) and not already reversed — enforced via `NOT EXISTS (SELECT 1
   FROM stock_movements WHERE reversed_movement_id = $1)` — otherwise
   `ErrStockMovementAlreadyReversed`.
3. Re-credit the product with the same guarded-`UPDATE` pattern as §4.1 step 2, `ENTRY`
   direction (`current_stock + $2`, no lower-bound guard needed — an `ENTRY` cannot make the
   balance negative).
4. `INSERT` the inverse `ENTRY` movement with `reversed_movement_id` set to the original's
   id.
5. `Commit`.

### 4.3 `ListStockMovements`

A single `SELECT ... FROM stock_movements WHERE service_order_id = $1 ORDER BY occurred_at
DESC` — a read, no transaction needed (same reasoning `findServicesByIDs` already applies).

## 5. Request/response shapes (`stockusage_dto.go`)

```jsonc
// POST /api/v1/service-orders/{id}/stock-movements
{ "items": [{ "productId": "<uuid>", "quantity": 2 }] }
// → 201
{ "items": [{ "id": "...", "productId": "...", "serviceOrderId": "...", "type": "EXIT",
              "quantity": 2, "previousStock": 10, "newStock": 8, "occurredAt": "..." }] }

// GET /api/v1/service-orders/{id}/stock-movements
// → 200 { "items": [ ...same shape... ] }

// POST /api/v1/service-orders/{id}/stock-movements/{movementId}/reversal
// (no body) → 201
{ "id": "...", "productId": "...", "serviceOrderId": "...", "type": "ENTRY",
  "quantity": 2, "previousStock": 8, "newStock": 10, "reversedMovementId": "...", "occurredAt": "..." }
```

The list envelope is `{"items": [...]}` (CLAUDE.md §8's "reuse this shape for new list
endpoints" convention, first defined by the service catalog listing) — this is a genuinely
new top-level list endpoint, not an addition to the order detail response's existing
`data/page/pageSize/total/totalPages` shape, and a service order's movement count in one
`EM_EXECUCAO` window does not need pagination.

## 6. `product` package: minimal touch to persist its own movements

`internal/features/product/repository.go`'s `AdjustStock` changes from
`AdjustStock(ctx, id uuid.UUID, delta int) (*Product, error)` to
`AdjustStock(ctx, id uuid.UUID, movement *StockMovement) (*Product, error)`: the caller
(`service.go`'s `AdjustStock`, already building a `*StockMovement` via
`Product.ApplyStockAdjustment`) passes it through; the repository wraps the existing guarded
`UPDATE` and a new `INSERT INTO stock_movements (..., service_order_id) VALUES (..., NULL)`
in one transaction, filling `movement.PreviousStock`/`NewStock`/`OccurredAt` from the
`RETURNING` clauses. `ProductRepository` gains `ListMovements(ctx, productID uuid.UUID,
page, pageSize int) ([]*StockMovement, int, error)`, and `handler.listMovements` calls it
instead of returning its hardcoded empty array — the only behavior change to that feature,
no unrelated refactor.

## 7. Error handling (`errors.go` additions + `httpsupport.go` mapping)

| Sentinel error | HTTP mapping |
| --- | --- |
| `ErrEmptyStockUsage` | 400 `Validation("stock usage must include at least one item")` |
| `ErrInvalidQuantity` (reused) | 400 — already mapped |
| `ErrProductNotFound` (reused) | 404 — already mapped |
| `ErrProductInactive` (reused) | 409 — already mapped |
| `ErrInsufficientStock` | 409 `Conflict("INSUFFICIENT_STOCK", ...)` |
| `ErrStockMovementNotFound` | 404 `NotFound("stock movement not found")` |
| `ErrStockMovementAlreadyReversed` | 409 `Conflict("STOCK_MOVEMENT_ALREADY_REVERSED", ...)` |
| `ErrStockMovementNotReversible` | 409 `Conflict("STOCK_MOVEMENT_NOT_REVERSIBLE", ...)` |
| `ErrInvalidStatusTransition` (reused) | 409 — already mapped |

## 8. Docs kept in sync (CLAUDE.md §10/§14)

- `docs/entities.md`: new `StockMovement` entity + `StockMovementType` enum sections.
- `docs/schema.sql`: `stock_movement_type` enum, `stock_movements` table, indexes.
- `specs/architecture.md`: new addendum block appended.

## 9. Testing (requirements.md §7's last checklist item, RNF06)

- Unit (service layer, fake repository): `stockusage_service_test.go` — BR1 (must be
  `EM_EXECUCAO`), BR3 (quantity > 0), empty items rejected.
- Integration (`internal/handlers_test/service_order_test.go`), against a real Postgres,
  reusing `insertProduct`/`productStock`/`moveServiceOrderToEmExecucao`:
  - successful multi-item deduction decrements every product and links every movement to
    the order (BR5).
  - insufficient stock on one item rolls back every item in that request, verified via
    `productStock` on the earlier, "would-have-succeeded" item (BR7).
  - inactive/nonexistent product rejected.
  - concurrency: two goroutines racing `RegisterStockUsage` against the same product with
    stock enough for exactly one — assert exactly one succeeds (BR8).
  - reversal restores stock and links back to the original; reversing twice is rejected
    (BR9).
