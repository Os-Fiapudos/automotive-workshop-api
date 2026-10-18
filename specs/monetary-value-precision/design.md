# Design — Monetary Value Precision

Satisfies: `requirements.md` (all sections).

## 1. `internal/shared/money` (new package)

```go
package money

// Cents is an exact monetary amount, in the smallest unit of the currency (cents for
// BRL). Never convert a Cents value through float64 for arithmetic — only MarshalJSON
// and String (display) format it, and both do so via integer division/modulo, never
// float division.
type Cents int64

func (c Cents) MarshalJSON() ([]byte, error) {
    // Formats as a plain JSON number with exactly two decimal places, e.g. 1990 -> "19.90",
    // via c/100 and c%100 (both integer ops) and fmt.Sprintf("%d.%02d", ...) — never a
    // float64 division. Negative values format as -D.DD (quote/price columns are all
    // CHECK (... >= 0) in docs/schema.sql, so this path is defensive, not expected).
}

func (c *Cents) UnmarshalJSON(data []byte) error {
    // Parses a JSON number (e.g. 19.90) via its decimal text representation
    // (json.Number / strconv on the raw token, not float64), rejecting more than two
    // decimal places (ErrTooManyDecimalPlaces) rather than silently truncating.
}

// ToNumeric/FromNumeric convert between Cents and pgx's pgtype.Numeric, the type pgx v5
// uses for an unconstrained NUMERIC column. Both operate on pgtype.Numeric's integer
// Int (*big.Int) and base-10 Exp fields directly — no float64 anywhere in either
// direction.
func ToNumeric(c Cents) pgtype.Numeric
func FromNumeric(n pgtype.Numeric) (Cents, error)
```

`doc.go` documents the one rule this whole package exists to enforce: no `float64` touches
a `Cents` value, ever, in either direction. This is "truly generic" code reused identically
by three features (`CLAUDE.md` §9.3), unlike a feature-specific business rule — the same bar
`internal/shared/document` and `internal/shared/apierror` already met for their own
features.

## 2. Repository boundary (one pattern, applied in three features)

Every `SELECT`/`INSERT`/`UPDATE` touching a money column changes from scanning/binding a
bare `float64` field to scanning/binding a `pgtype.Numeric` local variable, converted
immediately through `money.FromNumeric`/`money.ToNumeric`:

```go
// Before (service-catalog/repository.go, illustrative):
err := row.Scan(&service.ID, &service.Price, ...)  // Price float64, implicit float scan

// After:
var priceNumeric pgtype.Numeric
err := row.Scan(&service.ID, &priceNumeric, ...)
if err == nil {
    service.Price, err = money.FromNumeric(priceNumeric)
}
```

```go
// Before (product/repository.go INSERT, illustrative):
tx.Exec(ctx, `INSERT INTO products (..., unit_price) VALUES (..., $1)`, product.UnitPrice)

// After:
tx.Exec(ctx, `INSERT INTO products (..., unit_price) VALUES (..., $1)`, money.ToNumeric(product.UnitPrice))
```

This touches every repository method already listed per feature in `requirements.md` §4 —
no new query is added or removed, each existing query's money-column scan/bind site is
updated in place.

## 3. Domain/model changes, per feature

- **`service-catalog/model.go`**: `Service.Price float64` → `Service.Price money.Cents`.
  `NewService`'s price-validation (`price >= 0`) stays — `Cents` is a signed `int64`, so the
  same `price < 0` check applies, just against an integer instead of a float comparison
  (removing the one place float comparison for money existed in this feature, a minor
  correctness improvement alongside the main fix).
- **`product/model.go`**: `Product.UnitPrice float64` → `Product.UnitPrice money.Cents`,
  same treatment for `NewProduct`'s `unitPrice < 0` check.
- **`service-order/model.go`**: `QuoteItem.UnitPrice`/`.Total` → `money.Cents`;
  `Quote.TotalAmount` → `money.Cents`; `calculateTotal`:

  ```go
  // Before:
  func calculateTotal(items []QuoteItem) float64 {
      var total float64
      for _, item := range items {
          total += item.Total
      }
      return total
  }

  // After:
  func calculateTotal(items []QuoteItem) money.Cents {
      var total money.Cents
      for _, item := range items {
          total += item.Total
      }
      return total
  }
  ```

  Every `item.Total` itself is computed as `item.UnitPrice * money.Cents(item.Quantity)` —
  exact integer multiplication, replacing whatever `float64` multiplication currently
  computes it in `quote_service.go`'s item-building step.

## 4. API layer

No DTO field is renamed or reshaped (`requirements.md` §4/AC7) — `unitPrice`, `price`,
`total`, `totalAmount` all stay `json:"..."`-tagged exactly as today; only their Go type
changes from `float64` to `money.Cents`, which `MarshalJSON`/`UnmarshalJSON` (§1) keep
wire-compatible. Request DTOs that currently decode a price as `float64` (none exist today
— every price-bearing write path reads from the catalog server-side, not from client input,
per `specs/service-catalog/design.md`/`specs/product-management/`) are unaffected; if a
future feature ever accepts a price from a client, it should decode into `money.Cents`
directly rather than `float64`, but no such path exists to migrate today.

## 5. Interaction with the architecture refactor

If `specs/service-order-architecture-refactor/` has already landed when this feature is
implemented, `QuoteItem`/`Quote` live in `domain/quote.go` and the repository changes land
in `adapter/postgres/`; `internal/shared/money` is still imported by both (a shared package
import is not a cross-feature import, `CLAUDE.md` §9.2, and the domain layer depending on a
`internal/shared/` value type is the same kind of dependency `domain.ServiceOrder` already
has on `github.com/google/uuid`). If this feature lands first, the same changes simply apply
to the current flat `internal/features/service-order/model.go`/`repository.go` instead —
nothing in this design depends on refactor order.

## 6. Testing strategy

- **Unit** (`internal/shared/money/money_test.go`, new): `MarshalJSON`/`UnmarshalJSON`
  round-trip for values including `0.1`, `19.90`, `1234567.89`, `0`, and the `NUMERIC(12,2)`
  max magnitude; `UnmarshalJSON` rejects a third decimal place; `ToNumeric`/`FromNumeric`
  round-trip for the same value set.
- **Unit** (per feature, extending existing `*_test.go`): `calculateTotal` over a set of
  items specifically chosen because their prices are known to produce a rounding error
  under naive IEEE-754 `float64` summation (`requirements.md` AC4); `NewService`/
  `NewProduct`'s negative-price rejection, now asserted against `money.Cents`.
- **Integration** (one new test per feature, or extending an existing one): write a known
  `Cents` value through the repository, read it back via a real Postgres `NUMERIC(12,2)`
  column, assert exact equality (`requirements.md` AC3) — this is the test that actually
  proves the `pgtype.Numeric` conversion is lossless against the real column type, not just
  against Go's own round-trip.
- **Regression**: every existing integration test in `internal/handlers_test/` that asserts
  a JSON response body containing a price/total field continues to pass unchanged (proves
  AC7 — the wire format is untouched).

## 6a. Documentation

`CLAUDE.md` §3's `internal/shared/` listing (`database`, `token`, `middleware`, `httpx`,
`apierror`, `document`, `config`) gains `money (Cents value type, JSON + Postgres NUMERIC
conversion)` — missed in this plan's first pass across every Fase 2 spec; added here since
this is the feature that introduces the package. `specs/architecture.md`'s own
cross-cutting-code section gets the same addition, consistent with how
`specs/customer-management/` and `specs/auth/` already registered the shared packages they
introduced.

## 7. Traceability

| Requirement | Design section |
| --- | --- |
| BR1, BR2 (no float64, `Cents int64`) | §1, §3 |
| BR3 (schema unchanged, explicit boundary conversion) | §2 |
| BR4 (JSON wire format unchanged) | §1, §4 |
| BR5 (`calculateTotal` all-integer) | §3 |
| BR6 (existing specs' ACs unaffected) | §4, §6 |
| AC1–AC7 | §1, §2, §3, §6 |
