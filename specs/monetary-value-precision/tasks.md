# Tasks — Monetary Value Precision

Each task references the `design.md` section it implements. Revised 2026-10-20 with the
F2-US13 story (`requirements.md` §0); all tasks done on that date.

## 1. Shared packages (`design.md` §1, §2)

- [x] Create `internal/shared/money/`: `Cents` type, `MaxAmount`, `Parse`,
      `MarshalJSON`/`UnmarshalJSON`, `String`, `Validate`, `MulQuantity`/`Add`/`Sum` —
      standard library only.
- [x] `internal/shared/database/money.go`: `NumericFromCents`/`CentsFromNumeric`/
      `ScanCents` (`github.com/jackc/pgx/v5/pgtype`, part of the existing `pgx/v5`
      dependency — no `go.mod` change).
- [x] `money_test.go` (both packages): JSON round-trip (incl. known float64-lossy values),
      byte equality with the old `float64` encoding, decimal-place and range rejection,
      arithmetic limits, `NUMERIC` conversion round-trip and rejections.

## 2. `service-catalog` (`design.md` §3, §4)

- [x] `model.go`/`service.go`: `Price` → `money.Cents`; validation unchanged.
- [x] `repository.go`: drop the `::double precision` cast; scan/bind through
      `database.ScanCents`/`NumericFromCents`.
- [x] `handler.go`: request/response `price` → `money.Cents`; `decodeBody` maps
      `money.ErrInvalidAmount` to a `price` validation detail.
- [x] Update `*_test.go` (values in cents); add rejection-policy and wire-format tests.

## 3. `product` (`design.md` §3, §4)

- [x] `model.go`/`service.go`: `UnitPrice` → `money.Cents`.
- [x] `repository.go`: every scan/bind of `unit_price` converted.
- [x] `dto.go`/`handler.go`/`httpsupport.go`: `unitPrice` → `money.Cents`; `decodeJSON`
      maps `money.ErrInvalidAmount` to a `unitPrice` validation detail.
- [x] Update `*_test.go`; add the `decodeJSON` policy test.

## 4. `service-order` (`design.md` §3, §5)

- [x] `model.go`: `QuoteItem.UnitPrice`/`.Total`, `Quote.TotalAmount` → `money.Cents`;
      `priceQuoteItems` (new) and `calculateTotal` over `Cents` with limit checks.
- [x] `quote_service.go`: item totals computed by `priceQuoteItems` after
      `validateQuoteItems`, not by `float64` multiplication in `resolveQuoteItems`.
- [x] `errors.go`/`httpsupport.go`: `ErrQuoteAmountOutOfRange` → `400 VALIDATION_ERROR`.
- [x] `repository.go`/`quote_repository.go`: every scan/bind of `unit_price`/`price`/
      `total_amount`/`applied_unit_price`/`applied_total_price`/`applied_price` converted.
- [x] `quote_notifier.go`/`resend_notifier.go`: `TotalAmount` → `money.Cents`, formatted
      via `String()`.
- [x] Update `*_test.go`; add the rounding-error-prone `calculateTotal` cases (AC4), item
      totals with quantities, and the limit cases.

## 5. Integration tests (`design.md` §6)

- [x] `internal/handlers_test/money_test.go`: adapter vs. a temporary `NUMERIC(12,2)`
      column; service-catalog and product repository round-trips; quote flow end to end.
- [x] Existing `internal/handlers_test/` suite passes against a real Postgres
      (`product_test.go`/`service_order_test.go` updated to cents and exact equality).

## 6. Verification

- [x] `go build ./...`, `go vet ./...`, `go test ./...` pass, without `DATABASE_URL` and
      against a real Postgres (no skipped tests); `scripts/coverage.sh` passes.
- [x] `docs/openapi.yaml` — types unchanged (`number`); request money fields document the
      limit and rejection policy, response fields the exact representation.
- [x] `CLAUDE.md` §3 and `specs/architecture.md` — `internal/shared/money` added,
      `internal/shared/database` notes the NUMERIC conversion.
- [x] Re-check `requirements.md` §5 AC1–AC10.
