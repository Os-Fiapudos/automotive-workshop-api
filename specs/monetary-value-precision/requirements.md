# Requirements — Monetary Value Precision (`float64` → exact cents)

Status: **Implemented** (2026-10-20 — revised to the FP story, see §0; planning id F2-US13,
depends on FP-29)
Feature folders touched: `internal/features/service-order/`, `internal/features/
service-catalog/`, `internal/features/product/`, plus a new `internal/shared/money/` and the
NUMERIC adapter in `internal/shared/database/`.

## 0. Revision note (2026-10-20)

The first version of this spec (PR #30) predates the Jira story for F2-US13, whose acceptance
checklist is stricter in four places. This revision changes the original where they apply
(each is called out in place):

1. **`pgtype.Numeric` helpers live in the Postgres adapter, not in `money`.** The story
   requires them "no adapter PostgreSQL, separados do tipo de domínio/shared money para não
   violar Clean Architecture por dependência transitiva". The original design put
   `ToNumeric`/`FromNumeric` in `internal/shared/money`, which would make every domain type
   holding a `Cents` transitively depend on `pgx`. `money` now imports only the standard
   library; the conversion is `internal/shared/database`'s `NumericFromCents`/
   `CentsFromNumeric`/`ScanCents` (BR3, `design.md` §1–§2).
2. **Explicit policy for more than two decimal places: reject (new BR7).** Before this
   feature a client could send `"price": 19.999`; Postgres silently rounded it to `20.00`.
3. **Arithmetic validates quantity, overflow and the `NUMERIC(12,2)` limit (new BR8).**
   Before, a price above `9999999999.99` reached Postgres and failed as a 500.
4. **The JSON wire format is kept byte for byte (BR4 corrected).** The original said
   `Cents` marshals as `19.90`; the old `float64` fields marshal as `19.9`. Both are the same
   JSON number, but the story requires "formato e valores públicos permanecem compatíveis",
   and `tasks.md` §5 already asked for byte-for-byte unchanged bodies — so `Cents` marshals to
   the shortest exact decimal, which is exactly what `float64` produced for every
   `NUMERIC(12,2)` value.

It also corrects one factual error in the original: `design.md` §4 said no request DTO
decodes a price from client input. Four do — service-catalog's create/update (`price`) and
product's create/update (`unitPrice`) — and all four now decode straight into `money.Cents`
(scope, §4).

## 1. Context

Source: `GAPS-FASE-2.md` §5 item 8 (flagged for `service-order/model.go`) and this repo's
own Fase 2 planning conversation (2026-10-12), which found the same issue is broader: every
feature holding a monetary value uses Go's `float64` for it —
`service-order/model.go`'s `QuoteItem.UnitPrice`/`.Total`, `Quote.TotalAmount`, and
`calculateTotal`; `service-catalog/model.go`'s `Service.Price`; `product/model.go`'s
`Product.UnitPrice`. `float64` cannot represent every value `NUMERIC(12,2)` can (e.g.
`0.1 + 0.2 != 0.3` in IEEE-754 binary floating point), so repeated arithmetic over money —
`calculateTotal` summing quote items being the clearest case — can accumulate rounding
error that a currency value must never have.

**Important finding that simplifies this feature's scope**: `docs/schema.sql` already
stores every money column as `NUMERIC(12, 2)` (`products.unit_price`, `services.price`,
`quotes.total_amount`, `quote_products.applied_unit_price`/`applied_total_price`,
`quote_services.applied_price`/`applied_total_price`) — exact, not floating point. **The
imprecision is introduced entirely on the Go side**, when `pgx` scans a `NUMERIC` into a
`float64` Go field and when Go code does arithmetic on that `float64`. No schema change is
required by this feature — see `design.md` §1.

## 2. User story

> As a workshop owner,
> I want every monetary value the system stores, displays, or sums to be exact,
> so that a quote total, a product price, or a service price can never be off by a fraction
> of a cent due to floating-point rounding.

## 3. Business rules

1. No Go type in `service-order`, `service-catalog`, or `product` represents a monetary
   value as `float64` (or any other binary floating-point type) after this feature.
2. Monetary values are represented internally as an integer count of the currency's
   smallest unit ("cents" for BRL) — `type Cents int64` (`internal/shared/money`) — so that
   addition, subtraction, and multiplication by an integer quantity are always exact,
   standard integer arithmetic.
3. The database schema is unchanged (`NUMERIC(12,2)`, §1) — this feature only changes how
   Go code reads and writes those columns, converting explicitly at the boundary rather than
   letting `pgx`'s default `float64` scan happen implicitly. **Revised 2026-10-20 (§0
   item 1):** that conversion lives in the Postgres adapter (`internal/shared/database`), and
   `internal/shared/money` depends only on the standard library.
4. The JSON wire format of every affected API field is unchanged — a client reading
   `"unitPrice": 19.90` before this feature reads the exact same shape after it; this is an
   internal-representation fix, not a breaking API change. `Cents` marshals/unmarshals
   JSON via decimal-string-safe integer formatting (never through a `float64`
   intermediate), so the two decimal places a currency value already has are preserved
   exactly. **Corrected 2026-10-20 (§0 item 4):** the output is the shortest exact decimal
   (`1990` → `19.9`, `10000` → `100`, `10` → `0.1`), byte-identical to what the `float64`
   fields produced for every `NUMERIC(12,2)` value — not a fixed `19.90`.
5. `calculateTotal` (and any other money-summing logic touched by this feature) operates
   entirely in `Cents` (`int64` arithmetic) — no floating-point step anywhere in the sum.
6. This is a mechanical, behavior-preserving type change for every currently-passing
   acceptance criterion in `specs/service-catalog/`, `specs/service-order-diagnosis-quote/`,
   and the product-management specs that reference price/`unitPrice`/`total` — none of
   those specs' business rules change; only the Go representation does.
7. **New 2026-10-20 (§0 item 2) — more than two decimal places are rejected, never rounded
   or truncated.** A monetary request field (`price`, `unitPrice`) whose value has more than
   two significant decimal places (`19.999`, `0.001`; trailing zeros such as `19.900` are
   fine) is answered with `400 VALIDATION_ERROR` and a detail on that field. Rounding was
   rejected because it would silently store a value the client did not send. A non-number
   value (e.g. the string `"19.90"`) keeps its previous answer, `400 INVALID_BODY`.
8. **New 2026-10-20 (§0 item 3) — every amount stays within `NUMERIC(12,2)`.** A request
   value above `9999999999.99` in magnitude is rejected the same way as BR7. Item totals
   (`unit price × quantity`) require a strictly positive quantity and, like the quote total
   (sum of item totals), are checked against `9999999999.99` before anything is computed
   past it — no `int64` overflow and no database overflow error; composing such a quote
   answers `400 VALIDATION_ERROR` ("quote amount exceeds the maximum of 9999999999.99").

## 4. Scope

In scope:
- `internal/shared/money`: new package, `type Cents int64`, JSON marshal/unmarshal, parsing,
  bounded arithmetic (`design.md` §1). Standard library only.
- `internal/shared/database`: Postgres `NUMERIC` ↔ `Cents` conversion helpers (§0 item 1,
  `design.md` §2).
- service-catalog and product request DTOs (`price`, `unitPrice`) decode into `money.Cents`
  and map a rejected amount to a field-level validation error (BR7/BR8).
- `service-order`: `QuoteItem.UnitPrice`/`.Total` → `Cents`; `Quote.TotalAmount` → `Cents`;
  `calculateTotal` rewritten over `Cents`.
- `service-catalog`: `Service.Price` → `Cents`.
- `product`: `Product.UnitPrice` → `Cents`.
- Every repository method in these three features that scans or binds one of these columns,
  updated to convert through `internal/shared/money`'s helpers instead of a bare
  `float64` field.
- Existing tests in all three features updated for the new type; no test behavior/assertion
  about monetary *values* changes, only the Go type asserted against (values expressed in
  cents, and float tolerances such as `assert.InDelta` replaced by exact equality).

Out of scope:
- Any schema change (§1) — `docs/schema.sql`/`docs/entities.md`'s `NUMERIC(12,2)` columns
  are correct already.
- Any change to the JSON field names or the numeric *values* returned by any endpoint (§3
  item 4) — this is purely an internal precision fix.
- Multi-currency support, currency codes, or anything beyond BRL — out of this project's
  scope entirely, unaffected by this feature.
- `internal/features/service-order-architecture-refactor`'s package layout — if that
  refactor and this feature are both in flight, this feature's changes apply to whichever
  layout exists at the time (flat or layered); see `design.md`'s note.

## 5. Acceptance criteria

1. No `float64` (or `float32`) field remains for a monetary value in `service-order`,
   `service-catalog`, or `product`'s domain/model types.
2. `internal/shared/money.Cents` round-trips through JSON exactly (`19.90` → `Cents(1990)`
   → `19.9` — the same text the `float64` field produced, BR4 — not `19.899999999999999` or
   similar) — covered by a unit test with at least
   one value that is known to lose precision under naive `float64` round-tripping (e.g.
   `0.1`, `19.90`, `1234567.89`).
3. `internal/shared/money.Cents` round-trips through a real Postgres `NUMERIC(12,2)` column
   exactly (write, read back, compare) — an integration test against the real database.
4. `calculateTotal`, given quote items whose prices/quantities are known to produce a
   floating-point rounding error under naive `float64` summation, produces the exact,
   correct integer-cents total.
5. Every existing acceptance criterion in `specs/service-catalog/requirements.md`
   referencing `price`, and in `specs/service-order-diagnosis-quote/requirements.md`
   referencing `unitPrice`/`total`/`totalAmount`, still passes unchanged.
6. `go build ./...`, `go vet ./...`, `go test ./...` pass.
7. No JSON response field is renamed, removed, or reshaped by this feature (verified by
   running the existing integration test suite, which asserts exact response bodies in
   several places, and by a manual `docs/openapi.yaml` diff showing no schema change beyond
   internal type — the YAML's type was already `number`, unchanged).
8. **New 2026-10-20.** A `price`/`unitPrice` with more than two decimal places, or above
   `9999999999.99`, is answered `400 VALIDATION_ERROR` with a detail on that field (BR7/BR8)
   — unit-tested per feature.
9. **New 2026-10-20.** `MulQuantity`/`Add`/`calculateTotal` reject a non-positive quantity
   and any result beyond `9999999999.99` without overflowing; a quote whose item or total
   exceeds it is rejected before anything is persisted (BR8) — unit-tested, including
   totals with quantities (`3 × 19.90 + 7 × 0.10 = 60.40`).
10. **New 2026-10-20.** `internal/shared/money` imports no package outside the standard
    library (`go list -deps`); `pgtype` appears only in `internal/shared/database` (§0
    item 1).

## 6. Open questions resolved before design

- **Why `Cents int64` and not `shopspring/decimal` or another arbitrary-precision library?**
  A new external dependency needs explicit alignment (`CLAUDE.md` §12) and this project has
  none yet for decimal arithmetic. BRL (like most currencies this project will ever need)
  has exactly two decimal places and values well within `int64`'s range even at the
  `NUMERIC(12,2)`'s maximum magnitude, so a plain integer-cents representation is sufficient
  and adds zero new dependencies — the simpler choice per `CLAUDE.md` §12's "bar for adding
  a new [dependency] stays... justified need." If a future requirement needs arbitrary
  precision or multiple currencies, that is a new decision, not assumed here.
