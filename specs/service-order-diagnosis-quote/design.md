# Design — Service Order Diagnosis and Quote Composition

Satisfies: `requirements.md` (all sections). Extends `specs/service-order-opening/design.md`
rather than reopening its decisions (package layout, error envelope, transaction shape,
testing conventions).

## 1. Architecture decisions

### 1.1 Same package, not a new feature

`Quote`/`ComposeQuote`/`StartDiagnosis` operate on the same `ServiceOrder` aggregate
created by `specs/service-order-opening/` — the card itself says "Agregado
OrdemDeServico". Putting them in a separate `internal/features/quote/` package would force
that package to either duplicate `ServiceOrder`/status logic or import
`internal/features/service-order` directly, which breaks the "no direct coupling between
features" rule (`CLAUDE.md` §9.2). This feature therefore adds new files to the existing
`internal/features/service-order/` package:

```
internal/features/service-order/
├── model.go              → + Status consts, Quote/QuoteItem types, domain validation
├── quote_dto.go           → + diagnosis/quote request+response DTOs
├── errors.go              → + new sentinel errors
├── quote_repository.go    → + StartDiagnosis/SaveQuote/FindQuote + product/service lookups
├── quote_service.go       → + StartDiagnosis/ComposeQuote use cases
├── handler.go             → + 3 new routes
├── httpsupport.go         → + new error mappings
├── quote_model_test.go
├── quote_service_test.go
├── fake_repository_test.go → extended with the new fake methods
```

New use-case logic goes in `quote_*.go` files (not appended to the existing
`dto.go`/`repository.go`/`service.go`) purely for readability — same package, same public
API surface conventions as the rest of `internal/features/service-order/`.

### 1.2 Domain layer

- New `Status` consts: `StatusInDiagnosis = "IN_DIAGNOSIS"`,
  `StatusAwaitingApproval = "AWAITING_APPROVAL"` (added next to the existing
  `StatusReceived` in `model.go`).
- `ServiceOrder` gains two domain methods (no exported setter for `Status` — same
  invariant as before, transitions only happen through these):
  - `(*ServiceOrder) startDiagnosis() error` — requires `Status == StatusReceived`,
    returns `ErrInvalidStatusTransition` otherwise; sets `Status = StatusInDiagnosis`.
  - `(*ServiceOrder) markAwaitingApproval() error` — requires `Status !=
    StatusReceived`, returns `ErrInvalidStatusTransition` otherwise; sets `Status =
    StatusAwaitingApproval` (no-op transition if already there — recomposition case,
    requirements.md §3.9).
- New types:
  ```go
  type QuoteItemKind string
  const (
      QuoteItemProduct QuoteItemKind = "PRODUCT"
      QuoteItemService QuoteItemKind = "SERVICE"
  )

  // QuoteItemInput is what the service layer receives before resolving the
  // catalog reference — mirrors CreateInput's "raw strings in, resolved refs
  // out" shape from the existing Create use case.
  type QuoteItemInput struct {
      Kind      QuoteItemKind
      ProductID string // set when Kind == QuoteItemProduct
      ServiceID string // set when Kind == QuoteItemService
      Quantity  int
  }

  // QuoteItem is a resolved, priced line — the snapshot that gets persisted.
  type QuoteItem struct {
      Kind        QuoteItemKind
      ProductID   uuid.UUID // zero value when Kind == QuoteItemService
      ServiceID   uuid.UUID // zero value when Kind == QuoteItemProduct
      Description string
      Quantity    int
      UnitPrice   float64 // see §1.3 on money representation
      Total       float64
  }

  type Quote struct {
      ID            uuid.UUID
      Code          int64
      ServiceOrderID uuid.UUID
      TotalAmount   float64
      Status        QuoteStatus // "PENDING" | "APPROVED" | "REJECTED"
      Items         []QuoteItem
      GeneratedAt   time.Time
      RespondedAt   *time.Time
      CreatedAt     time.Time
      UpdatedAt     time.Time
  }
  ```
- Domain-level validation function `validateQuoteItems(items []QuoteItem) error`: at least
  one item (`ErrEmptyQuote`), every `Quantity > 0` (`ErrInvalidQuantity`) — called by the
  service layer after resolving+pricing items, before persisting (requirements.md §3.3,
  §3.6). Total calculation (`sum(item.Total)`) is a plain function next to it,
  `calculateTotal(items []QuoteItem) decimal`, the single place RF06 is implemented —
  nowhere else in the codebase computes a quote total.

### 1.3 Money representation

`internal/features/product` already established the project's money convention:
`Product.UnitPrice float64`, read/written directly against `NUMERIC(12,2)` columns via
pgx's default numeric↔float64 conversion (`internal/features/product/repository.go`).
This feature reuses that convention rather than inventing a second one — `QuoteItem.
UnitPrice`/`Total` and `Quote.TotalAmount` are `float64`, computed with ordinary Go
arithmetic (`quantity` is a small integer, so `float64(quantity) * unitPrice` carries no
practical precision risk at this scale, consistent with how `product` already accepts
this trade-off). RF06 ("calculated exclusively by the back end") is enforced by never
giving the request DTO a total field to populate, not by a special numeric type.

### 1.4 Application layer

Two new use cases on `ServiceOrderService` (same struct as the existing `Create`, now with
a slightly larger `lookups`/`repository` interface — see §3.2):

- `StartDiagnosis(ctx, serviceOrderID uuid.UUID) (*ServiceOrder, error)`:
  1. Load the order (`findServiceOrderByID`) — `ErrServiceOrderNotFound` if missing.
  2. `order.startDiagnosis()` — surfaces `ErrInvalidStatusTransition`.
  3. `repository.StartDiagnosis(ctx, order)` — persists the status change + history row
     transactionally (§3.3).
- `ComposeQuote(ctx, serviceOrderID uuid.UUID, inputs []QuoteItemInput) (*Quote, error)`:
  1. Load the order — `ErrServiceOrderNotFound` if missing.
  2. Reject if `order.Status == StatusReceived` (`ErrDiagnosisNotStarted`) — requirements.md
     §3.2.
  3. Load the existing quote for this order, if any (`repository.FindQuoteByServiceOrderID`
     — a quote row always exists once a first composition succeeds, see §3.1's
     `ON CONFLICT` upsert). If it exists and `Status != PENDING`, return
     `ErrQuoteAlreadyDecided` (requirements.md §3.9/§3.12... actually §3.9).
  4. Validate `inputs` is non-empty (`ErrEmptyQuote`) and every `Quantity > 0`
     (`ErrInvalidQuantity`) — cheap checks before touching the database.
  5. Resolve+price every item: for `QuoteItemProduct`, `lookups.findActiveProductByID`
     (`ErrProductNotFound`/`ErrProductInactive`); for `QuoteItemService`,
     `lookups.findServiceByID` (`ErrServiceNotFound` — no active check, requirements.md
     §3.12). Build `QuoteItem{Description, UnitPrice}` from the catalog row at this exact
     moment (the snapshot).
  6. `calculateTotal(items)`.
  7. `order.markAwaitingApproval()`.
  8. `repository.SaveQuote(ctx, order, items, total)` — transactional upsert (§3.3).
- `GetQuote(ctx, serviceOrderID uuid.UUID) (*Quote, error)`: `findServiceOrderByID` (404 if
  missing) then `FindQuoteByServiceOrderID` — `ErrQuoteNotFound` if diagnosis/composition
  hasn't happened yet (a real, expected state per `docs/seed.sql`'s order 4 example, not an
  error condition the client did something wrong to reach — mapped to 404 all the same,
  consistent with "the resource you asked for doesn't exist yet").

### 1.5 Persistence

Extends `serviceOrderLookups`/`ServiceOrderRepository` (still both implemented by the same
`PostgresServiceOrderRepository`, same split rationale as `specs/service-order-opening/
design.md` §3.2 — keeps `quote_service_test.go`'s fake small):

```go
type serviceOrderLookups interface {
    // ... existing methods ...
    findServiceOrderByID(ctx context.Context, id uuid.UUID) (*ServiceOrder, error)
    findActiveProductByID(ctx context.Context, id uuid.UUID) (*productRef, error)
    findServiceByID(ctx context.Context, id uuid.UUID) (*serviceRef, error) // reused, already exists
}

type ServiceOrderRepository interface {
    Create(ctx context.Context, order *ServiceOrder) error
    StartDiagnosis(ctx context.Context, order *ServiceOrder) error
    SaveQuote(ctx context.Context, order *ServiceOrder, items []QuoteItem, total float64) (*Quote, error)
    FindQuoteByServiceOrderID(ctx context.Context, serviceOrderID uuid.UUID) (*Quote, error)
}
```

`productRef` is a new package-local projection (mirrors `customerRef`/`vehicleRef` from
`specs/service-order-opening/design.md` §1.4): `{ID, Code, Name, Description, UnitPrice
float64, Active bool}`.

### 1.6 Transaction shapes (RNF07)

Both new writes follow the exact `pool.Begin` / `defer tx.Rollback` / `tx.Commit` shape
`specs/service-order-opening/design.md` §3.3 established as the project's multi-table
transaction pattern:

**`StartDiagnosis`**: `UPDATE service_orders SET status = 'IN_DIAGNOSIS' WHERE id = $1
AND status = 'RECEIVED' RETURNING updated_at` (the `AND status = 'RECEIVED'` guard closes a
race with a concurrent transition; zero rows affected is treated the same as the
pre-checked `ErrInvalidStatusTransition`) + insert into `service_order_history`
(`event = 'diagnosis_started'`).

**`SaveQuote`**: upserts the `quotes` row (`INSERT ... ON CONFLICT (service_order_id) DO
UPDATE SET total_amount = EXCLUDED.total_amount, updated_at = now()`, relying on `quotes`'s
existing `UNIQUE (service_order_id)`), deletes all existing rows in `quote_products`/
`quote_services` for that quote id, re-inserts the new item set (delete-then-insert is
simpler and safe here — a `PUT` is a full replace, not a diff — and item counts are small),
`UPDATE service_orders SET status = 'AWAITING_APPROVAL' WHERE id = $1` (only if not
already that status, but idempotent either way), and inserts into `service_order_history`
(`event = 'quote_composed'`). All in one transaction; any failure rolls back the quote,
its items, and the order's status transition together.

> **Erratum (`specs/service-order-quote-decision/`)**: the `UPDATE service_orders` statement
> described above no longer exists. That feature's own requirements attributed the
> `IN_DIAGNOSIS → AWAITING_APPROVAL` transition to an explicit "send the quote to the
> customer" step this design did not have, and moved it to a new `SendQuote` repository
> method instead — see `specs/service-order-quote-decision/design.md` §1.2/§1.5 for the
> resolution and its traceability. This paragraph is left as originally written, per
> `specs/README.md`'s "a specification should not be changed just to make the code fit" rule:
> it was correct for what this feature implemented at the time; the later feature is the one
> that changed the requirement, and records that change in its own spec rather than rewriting
> this one.

### 1.7 API layer

Reuses `internal/shared/apierror`, same as the rest of the feature. New status mapping,
appended to `writeServiceError`:

| Situation | Status | `error.code` |
| --- | --- | --- |
| Service order not found | 404 | `NOT_FOUND` |
| Invalid status transition (diagnosis on non-RECEIVED) | 409 | `INVALID_STATUS_TRANSITION` |
| Diagnosis not started (compose on RECEIVED) | 409 | `DIAGNOSIS_NOT_STARTED` |
| Quote already decided (APPROVED/REJECTED) | 409 | `QUOTE_ALREADY_DECIDED` |
| Quote not found (GET before any composition) | 404 | `NOT_FOUND` |
| Empty item list | 400 | `VALIDATION_ERROR` |
| Invalid quantity (<= 0) | 400 | `VALIDATION_ERROR` |
| Product not found | 404 | `NOT_FOUND` |
| Product inactive | 409 | `PRODUCT_INACTIVE` |
| Service not found | 404 | `NOT_FOUND` |

Same 404-vs-409 reasoning as `specs/service-order-opening/design.md` §1.5: "not found" is
404, a well-formed reference to a real resource that's in a conflicting state is 409.

New routes, wrapped in `requireAuth` (requirements.md §7.4 — a deviation from this
package's existing, still-unauthenticated `POST /api/v1/service-orders`, so
`RegisterRoutes`'s signature changes to accept it, mirroring `product.RegisterRoutes`):

```go
func RegisterRoutes(mux *http.ServeMux, service *ServiceOrderService, requireAuth func(http.Handler) http.Handler) {
    handler := &serviceOrderHandler{service: service}
    wrap := func(h http.HandlerFunc) http.Handler {
        if requireAuth != nil {
            return requireAuth(h)
        }
        return h
    }

    mux.HandleFunc("POST /api/v1/service-orders", handler.create) // unchanged, still unauthenticated
    mux.Handle("POST /api/v1/service-orders/{id}/diagnosis", wrap(handler.startDiagnosis))
    mux.Handle("PUT /api/v1/service-orders/{id}/quote", wrap(handler.composeQuote))
    mux.Handle("GET /api/v1/service-orders/{id}/quote", wrap(handler.getQuote))
}
```

`cmd/api/main.go`'s call becomes
`serviceorder.RegisterRoutes(router, serviceOrderService, requireAuth)`, matching
`product.RegisterRoutes`'s existing three-argument shape — `POST /api/v1/service-orders`
itself stays unauthenticated (nil-safe `wrap` isn't applied to it), so this change does not
silently resolve the open "should service-order-opening require auth" decision noted in
`CLAUDE.md` §1/§17.

## 2. Domain model additions

### 2.1 `quote_products` / `quote_services` (`docs/entities.md`, `docs/schema.sql`)

Add `applied_description` (snapshot, mirrors `applied_unit_price`/`applied_price`) to
both; add `quantity` to `quote_services` (did not exist — only `quote_products` had it).
Add `applied_total_price` to both (persist the per-item total, not just recompute it on
read — the checklist explicitly requires each item to "register" a total, and storing it
avoids the read path ever silently disagreeing with what was actually charged).

```sql
ALTER TABLE quote_products
    ADD COLUMN IF NOT EXISTS applied_description TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS applied_total_price NUMERIC(12, 2) NOT NULL DEFAULT 0
        CHECK (applied_total_price >= 0);
ALTER TABLE quote_products ALTER COLUMN applied_description DROP DEFAULT;
ALTER TABLE quote_products ALTER COLUMN applied_total_price DROP DEFAULT;

ALTER TABLE quote_services
    ADD COLUMN IF NOT EXISTS applied_description TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS quantity INTEGER NOT NULL DEFAULT 1 CHECK (quantity > 0),
    ADD COLUMN IF NOT EXISTS applied_total_price NUMERIC(12, 2) NOT NULL DEFAULT 0
        CHECK (applied_total_price >= 0);
ALTER TABLE quote_services ALTER COLUMN applied_description DROP DEFAULT;
ALTER TABLE quote_services ALTER COLUMN applied_total_price DROP DEFAULT;
```

(`DEFAULT` + `DROP DEFAULT` is only meaningful for an `ALTER TABLE` against a database
that already has rows — this project's schema is only ever applied to a fresh volume
(`CLAUDE.md` §14), so in `docs/schema.sql` itself these columns are added directly as
`NOT NULL` in the `CREATE TABLE` statements, no `DEFAULT`/backfill needed. The `ALTER
TABLE` form above is shown for clarity of exactly what changed; the actual edit is to the
`CREATE TABLE quote_products`/`quote_services` blocks in place.)

### 2.2 `history_event` enum

```sql
DO $$ BEGIN
    ALTER TYPE history_event ADD VALUE IF NOT EXISTS 'diagnosis_started';
EXCEPTION WHEN duplicate_object THEN NULL; END $$;
DO $$ BEGIN
    ALTER TYPE history_event ADD VALUE IF NOT EXISTS 'quote_composed';
EXCEPTION WHEN duplicate_object THEN NULL; END $$;
```

Again, since the schema is only ever applied fresh, the actual edit in `docs/schema.sql`
is to the original `CREATE TYPE history_event AS ENUM (...)` literal, adding the two new
values there directly — the `ALTER TYPE` form is Postgres's incremental-migration syntax,
not what this greenfield schema file uses.

### 2.3 `docs/entities.md`

- `Quote` section: item shape gains `description`/`totalValue` per item (already had
  `quantity`/unit price implicitly via `products`/`services` arrays — clarify the array
  entries carry these four fields now, not just id+price).
- `ServiceOrderHistory.event` enum values gain `diagnosis_started`, `quote_composed`.

### 2.4 `docs/seed.sql`

Existing `quote_products`/`quote_services` inserts (order 1, 2, 3, 5) need
`applied_description`/`applied_total_price` (and `quantity` for `quote_services`) added —
description copied from the matching `products`/`services` seed row, total computed as
`quantity * applied_price`. No new orders added; this is purely filling in the new
columns for existing rows so the seed stays internally consistent.

## 3. Testing strategy

- **Unit tests** (`quote_model_test.go`): `startDiagnosis`/`markAwaitingApproval`
  transition rules (valid/invalid source status, idempotent re-entry into
  `AWAITING_APPROVAL`); `validateQuoteItems`/`calculateTotal` (empty list, invalid
  quantity, correct sum in cents, no floating-point drift across many items).
- **Unit tests** (`quote_service_test.go`, extending `fake_repository_test.go`): success
  path (diagnosis → compose → order shows `AWAITING_APPROVAL`), compose before
  diagnosis rejected, compose on unknown order rejected, unknown/inactive product
  rejected, unknown service rejected, empty items rejected, invalid quantity rejected,
  recompose while `PENDING` replaces items and recalculates total, recompose after
  `APPROVED`/`REJECTED` rejected (fake needs a way to seed a decided quote — a small
  test-only setter), a later catalog price/name change does not affect an
  already-fetched `QuoteItem` (proven by construction: the fake's lookup returns a
  snapshot struct, not a live reference — same reasoning as `customerRef`/`vehicleRef` in
  the existing feature).
- **Integration tests** (`internal/handlers_test/`, extending the service-order test file
  or a new `service_order_quote_test.go`): full flow `RECEIVED` →
  `POST .../diagnosis` → `IN_DIAGNOSIS` → `PUT .../quote` → `AWAITING_APPROVAL`,
  asserting `GET .../quote` matches; total computed server-side even when the request body
  sends a divergent value (the request DTO doesn't even have a total field to send —
  same enforcement-by-omission approach as `CreateRequest`'s missing `status` field);
  `products.current_stock` unchanged after composing; two `service_order_history` rows
  (one per transition) with correct `previous_status`/`new_status`; changing a product's
  price/name after composing a quote does not change the already-persisted item
  (`applied_description`/`applied_unit_price` stay put); rejecting diagnosis-start on a
  non-`RECEIVED` order; rejecting composition on a `RECEIVED` order.

## 4. Traceability

Every decision above satisfies a specific `requirements.md` item; `tasks.md` breaks this
design into ordered implementation steps, each referencing the section here it implements.
