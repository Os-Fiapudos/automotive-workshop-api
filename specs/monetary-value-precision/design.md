# Design — Monetary Value Precision

Satisfies: `requirements.md` (all sections).

> **Revised 2026-10-20** to the F2-US13 story (`requirements.md` §0). Changes against the
> first version: the `pgtype.Numeric` helpers moved from `internal/shared/money` to
> `internal/shared/database` (§1, §2); `MarshalJSON` emits the shortest exact decimal
> instead of a fixed two-decimal form (§1); parsing, the rejection policy and bounded
> arithmetic are specified (§1, §3, §4); §4's claim that no request DTO decodes a price is
> corrected.

## 1. `internal/shared/money` (new package, standard library only)

```go
package money

// Cents is an exact monetary amount, in the smallest unit of the currency (cents for BRL).
type Cents int64

// MaxAmount is the largest magnitude a NUMERIC(12,2) column holds (9999999999.99).
const MaxAmount Cents = 999_999_999_999

var ErrInvalidAmount        // matched (errors.Is) by the two below
var ErrTooManyDecimalPlaces // "must have at most two decimal places"   (BR7)
var ErrOutOfRange           // "must not exceed 9999999999.99"          (BR8)
var ErrInvalidQuantity      // "quantity must be greater than zero"     (BR8)

func Parse(text string) (Cents, error)        // JSON number syntax -> Cents, from decimal text
func (c Cents) MarshalJSON() ([]byte, error)  // shortest exact decimal: 1990 -> 19.9
func (c *Cents) UnmarshalJSON(data []byte) error
func (c Cents) String() string                // always two places: "19.90", for display/logs
func (c Cents) Validate() error               // |c| <= MaxAmount
func (c Cents) MulQuantity(quantity int) (Cents, error)
func (c Cents) Add(other Cents) (Cents, error)
func Sum(values ...Cents) (Cents, error)
```

- **`Parse`/`UnmarshalJSON`** read the number's decimal text — sign, integer digits,
  fraction digits, optional exponent (`1e2` was accepted by the old `float64` fields, so it
  still is) — and build the cents value with string and integer operations only. Trailing
  zeros are not precision (`19.900` is `1990`); more than two significant decimal places is
  `ErrTooManyDecimalPlaces`, never rounded (BR7); a magnitude above `MaxAmount` is
  `ErrOutOfRange` (BR8), checked on the digit count before `strconv.ParseInt`, so no
  overflow. A non-number token (string, bool, object, array) returns a
  `*json.UnmarshalTypeError`, the same error a `float64` field produced, so those requests
  keep their previous `400 INVALID_BODY` answer. `null` leaves the value untouched, like
  `encoding/json` does for a number.
- **`MarshalJSON`** writes the whole part, then `.` and the cents with trailing zeros
  trimmed (`1990` → `19.9`, `10000` → `100`, `10` → `0.1`). `encoding/json` formats a
  `float64` with the shortest representation that round-trips; for any value with at most
  two decimals and at most 12 significant digits (all of `NUMERIC(12,2)`) that is exactly
  this text, so the wire format is unchanged byte for byte (BR4). A unit test compares both
  encodings over a value set that includes the limit.
- **Arithmetic** bounds every operand to `MaxAmount` before operating:
  `MulQuantity` rejects `quantity <= 0`, then checks `quantity <= MaxAmount / |c|` before
  multiplying; `Add` checks both operands and the result. With both operands bounded by
  `MaxAmount` (≈ 10^12), neither a product within the bound nor a sum can overflow `int64`.

`doc.go` states the one rule the package exists to enforce: no `float64` touches a `Cents`
value, in either direction — and that the Postgres conversion deliberately lives outside it.
This is "truly generic" code reused identically by three features (`CLAUDE.md` §9.3).

## 2. Postgres adapter — `internal/shared/database/money.go` (revised)

The conversion to and from `NUMERIC` lives in the shared Postgres adapter package, which
already owns the `pgx` pool, so the domain models that hold a `money.Cents` do not depend on
`pgx` transitively (story checklist; `requirements.md` §0 item 1):

```go
// NumericFromCents builds the bind value: Numeric{Int: c, Exp: -2}.
func NumericFromCents(c money.Cents) pgtype.Numeric

// CentsFromNumeric reads Int × 10^Exp as cents with math/big — no float64. Rejects NULL,
// NaN/±Inf, more than two significant decimal places, and |value| > MaxAmount.
func CentsFromNumeric(n pgtype.Numeric) (money.Cents, error)

// ScanCents returns a pgtype.NumericScanner that writes through CentsFromNumeric into dst,
// so a scan site stays a one-liner.
func ScanCents(dst *money.Cents) pgtype.NumericScanner
```

Every `SELECT`/`INSERT`/`UPDATE` touching a money column changes in place:

```go
// Before (service-catalog/repository.go):
const serviceColumns = "..., price::double precision, ..."   // float64 via cast
row.Scan(&s.ID, ..., &s.Price, ...)

// After:
const serviceColumns = "..., price, ..."
row.Scan(&s.ID, ..., database.ScanCents(&s.Price), ...)

// Binding (product/repository.go INSERT):
tx.Exec(ctx, `INSERT INTO products (..., unit_price) VALUES (..., $1)`,
    database.NumericFromCents(product.UnitPrice))
```

No query is added or removed. Sites: service-catalog `scanService`/`Create`/`Update`;
product `Create`/`Update`/`List`/`scanProduct`; service-order `findActiveProductByID`,
`findServiceByID`, `SaveQuote` (header and both item tables) and
`FindQuoteByServiceOrderID` (header and both item tables).

## 3. Domain/model changes, per feature

- **`service-catalog`**: `Service.Price`, `NewService.Price`, `*Changes.Price` →
  `money.Cents`. The `price < 0` checks stay, now an integer comparison.
- **`product`**: `Product.UnitPrice`, `NewProduct`/`ProductService.Create`'s `unitPrice`,
  `UpdateDetails`/`UpdateInput`'s `*UnitPrice` → `money.Cents`; same `< 0` checks.
- **`service-order`**: `QuoteItem.UnitPrice`/`.Total`, `Quote.TotalAmount`,
  `productRef.UnitPrice`, `serviceRef.Price`, `QuoteNotification.TotalAmount` and
  `SaveQuote`'s `total` → `money.Cents`. Item totals move out of `resolveQuoteItems` (which
  now only copies the catalog unit price) into a domain step that runs after
  `validateQuoteItems`, so the existing error precedence (catalog lookups first, then
  empty/quantity checks) is unchanged:

  ```go
  // ComposeQuote:
  items, err := service.resolveQuoteItems(ctx, inputs)
  validateQuoteItems(items)          // ErrEmptyQuote / ErrInvalidQuantity, as before
  priceQuoteItems(items)             // Total = UnitPrice.MulQuantity(Quantity)
  total, err := calculateTotal(items) // money.Cents sum via Add

  func calculateTotal(items []QuoteItem) (money.Cents, error) {
      var total money.Cents
      for _, item := range items {
          var err error
          if total, err = total.Add(item.Total); err != nil {
              return 0, quoteAmountError(err)
          }
      }
      return total, nil
  }
  ```

  `quoteAmountError` maps `money.ErrInvalidQuantity` to the existing `ErrInvalidQuantity`
  and a range error to the new `ErrQuoteAmountOutOfRange`, answered
  `400 VALIDATION_ERROR` "quote amount exceeds the maximum of 9999999999.99" (BR8) — before
  `SaveQuote`, so nothing is persisted.
- **`resend_notifier.go`** formats the e-mail total with `TotalAmount.String()` (then `.` →
  `,`) instead of `fmt.Sprintf("%.2f", float64)` — same text, no float.

`service-order` keeps its flat layout: the Clean Architecture refactor
(`specs/service-order-architecture-refactor/`, FP-29) has not landed on `develop` at
implementation time, and §5 below makes this design independent of that order.

## 4. API layer (corrected)

No DTO field is renamed or reshaped (`requirements.md` AC7) — `unitPrice`, `price`, `total`,
`totalAmount` keep their `json:"..."` tags; only their Go type changes to `money.Cents`.

**Correction to the first version**: request DTOs *do* decode a price from client input —
service-catalog `createRequest`/`updateRequest.Price` and product
`CreateRequest`/`UpdateRequest.UnitPrice`. They now decode into `*money.Cents` directly
(never `float64`), and each feature's body decoder maps an error matching
`money.ErrInvalidAmount` to a field-level validation error, everything else keeping its
previous answer:

| Input | Before | After |
| --- | --- | --- |
| `19.90`, `100`, `1e2` | accepted | accepted, same value |
| `19.999` | accepted, Postgres rounds to `20.00` | `400 VALIDATION_ERROR`, detail on the field (BR7) |
| `10000000000` | `500` (numeric field overflow) | `400 VALIDATION_ERROR`, detail on the field (BR8) |
| `"19.90"` (string) | `400 INVALID_BODY` | `400 INVALID_BODY` (unchanged) |

The quote request carries no money (the server prices it, RF06), so no service-order request
DTO changes.

**OpenAPI**: types stay `number`/`format: double` (no incompatible change). Request
`price`/`unitPrice` gain `maximum: 9999999999.99` and a description of the two-decimal
rejection policy; response money fields gain a description of the exact representation.

## 5. Interaction with the architecture refactor

If `specs/service-order-architecture-refactor/` has already landed when this feature is
implemented, `QuoteItem`/`Quote` live in `domain/quote.go` and the repository changes land
in `adapter/postgres/`; `internal/shared/money` is still imported by both (a shared package
import is not a cross-feature import, `CLAUDE.md` §9.2, and — now that `money` imports only
the standard library — the domain layer depending on it adds no infrastructure dependency,
satisfying that refactor's BR1). The `database.ScanCents`/`NumericFromCents` calls belong
to `adapter/postgres/` only. If this feature lands first (as it did, 2026-10-20), the same
changes apply to the flat `internal/features/service-order/` files instead.

## 6. Testing strategy

- **Unit** (`internal/shared/money/money_test.go`): `Parse` over plain, exponent, trailing
  zero and negative forms; rejection of a third decimal place, of out-of-range values and of
  malformed text; JSON round-trip for `0`, `0.1`, `0.3`, `19.9`, `1234567.89`, `100`, the
  limit and a negative value; `MarshalJSON` byte-equal to `json.Marshal(float64)` over a
  value set including the limit; non-number tokens reported as `*json.UnmarshalTypeError`;
  `MulQuantity`/`Add`/`Sum` exact results, quantity rejection, limit and would-be-overflow
  rejection.
- **Unit** (`internal/shared/database/money_test.go`): `NumericFromCents`/
  `CentsFromNumeric` round-trip; normalization of other exponents (`19.9`, `19.900`, `1e2`);
  rejection of NULL, NaN, infinity, three decimals, beyond-limit and beyond-`int64` values;
  `ScanCents` leaving `dst` untouched on failure.
- **Unit** (per feature): `calculateTotal` over values that drift in `float64`
  (1000 × `0.10`, `0.1 + 0.2`; AC4), its limit; `priceQuoteItems` (`3 × 19.90 = 59.70`)
  and its limit/quantity rejection; `ComposeQuote` exact totals with quantities and its
  beyond-limit rejection with nothing persisted; the rejection policy at the catalog handler
  and product body decoder; the catalog response text (`19.90` → `"price":19.9`).
- **Integration** (`internal/handlers_test/money_test.go`, real Postgres): the adapter
  against a temporary `NUMERIC(12,2)` column (value back equal, `::text` equal to
  `Cents.String()`, one cent beyond the limit refused by the column); service-catalog and
  product repositories create/read/update round-trip for `0`, `0.10`, `19.90`,
  `1234567.89` and the limit; the quote flow end to end (`3 × 19.90 + 7 × 0.10`): stored
  columns `59.70`/`0.70`/`60.40` and the response text `"totalAmount":60.4`.
- **Regression**: every existing integration test in `internal/handlers_test/` keeps
  passing. `service_catalog_test.go` decodes responses into its own `float64` client struct
  (it never imports the feature's types), so it checks the public contract independently of
  the Go change.

## 6a. Documentation

`CLAUDE.md` §3's `internal/shared/` listing gains `money` (pure `Cents` value type) and the
`database` entry mentions the NUMERIC conversion; `specs/architecture.md`'s cross-cutting
code section gets the same addition, consistent with how earlier features registered the
shared packages they introduced.

## 7. Traceability

| Requirement | Design section |
| --- | --- |
| BR1, BR2 (no float64, `Cents int64`) | §1, §3 |
| BR3 (schema unchanged, explicit boundary conversion in the adapter) | §2 |
| BR4 (JSON wire format unchanged, byte for byte) | §1, §4 |
| BR5 (`calculateTotal` all-integer) | §3 |
| BR6 (existing specs' ACs unaffected) | §4, §6 |
| BR7 (reject more than two decimal places) | §1, §4 |
| BR8 (quantity, overflow, `NUMERIC(12,2)` limit) | §1, §3, §4 |
| AC1–AC10 | §1, §2, §3, §4, §6 |
