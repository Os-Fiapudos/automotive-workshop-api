# Tasks — Monetary Value Precision

Each task references the `design.md` section it implements.

## 1. Shared package (`design.md` §1)

- [ ] Create `internal/shared/money/`: `Cents` type, `MarshalJSON`/`UnmarshalJSON`,
      `ToNumeric`/`FromNumeric` (requires `github.com/jackc/pgx/v5/pgtype`, already an
      indirect dependency of this project's existing `pgx/v5` — confirm no `go.mod` change
      needed beyond `go mod tidy`, `CLAUDE.md` §12).
- [ ] `money_test.go`: JSON round-trip (incl. known float64-lossy values), decimal-place
      rejection, `ToNumeric`/`FromNumeric` round-trip.

## 2. `service-catalog` (`design.md` §3)

- [ ] `model.go`: `Service.Price` → `money.Cents`; `NewService`'s validation updated.
- [ ] `repository.go`: every scan/bind of `price` converted through `pgtype.Numeric` +
      `money.FromNumeric`/`ToNumeric` (`design.md` §2).
- [ ] Update `*_test.go` for the new type (no assertion value changes, only the type
      asserted against).

## 3. `product` (`design.md` §3)

- [ ] `model.go`: `Product.UnitPrice` → `money.Cents`; `NewProduct`'s validation updated.
- [ ] `repository.go`: every scan/bind of `unit_price` converted (`design.md` §2).
- [ ] Update `*_test.go` for the new type.

## 4. `service-order` (`design.md` §3, §5)

- [ ] `model.go` (or `domain/quote.go`, if the architecture refactor has landed):
      `QuoteItem.UnitPrice`/`.Total`, `Quote.TotalAmount` → `money.Cents`; `calculateTotal`
      rewritten over `Cents`.
- [ ] `quote_service.go` (or `usecase/quote_compose.go`): item-total computation
      (`UnitPrice * Cents(Quantity)`) over integers, not `float64`.
- [ ] `repository.go`/`quote_repository.go` (or `adapter/postgres/*.go`): every scan/bind
      of `unit_price`/`total`/`total_amount`/`applied_unit_price`/`applied_total_price`/
      `applied_price` converted (`design.md` §2).
- [ ] Update `*_test.go` for the new type; add the rounding-error-prone `calculateTotal`
      case (`requirements.md` AC4).

## 5. Integration tests (`design.md` §6)

- [ ] One round-trip test per feature (write `Cents` → real Postgres `NUMERIC(12,2)` →
      read back → assert exact equality).
- [ ] Full regression run of `internal/handlers_test/` confirming every existing
      price/total-bearing response body is byte-for-byte unchanged.

## 6. Verification

- [ ] `go build ./...`, `go vet ./...`, `go test ./...` pass, without `DATABASE_URL` and
      against a real Postgres.
- [ ] `docs/openapi.yaml` diffed — confirm no schema/type change beyond what was already
      documented (`number`, unchanged).
- [ ] `CLAUDE.md` §3 and `specs/architecture.md` — add `internal/shared/money` to the
      shared-package listing (`design.md` §6a).
- [ ] Re-check `requirements.md` §5 AC1–AC7.
