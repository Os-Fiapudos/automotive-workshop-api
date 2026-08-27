# Service Order Tracking — Design

Satisfies `requirements.md`. Follows the vertical-slice pattern already used by every
implemented feature (CLAUDE.md §9) and reuses the two established cross-cutting
conventions closest to this feature's own shape: the `apierror` envelope (this feature most
resembles `service-order`, which uses `apierror` — CLAUDE.md §8) and direct
parameterized-SQL cross-table reads for data owned by another feature's table (the pattern
`service-order`'s repository already uses for `vehicles`/`customers`, documented in
`specs/service-order-opening/design.md` §1.4).

## 1. Components (traceable to the source spec's component list)

| Source spec component | This design |
| --- | --- |
| Endpoint `GET /api/v1/acompanhamento/{codigo}` | `internal/features/service-order-tracking` handler, route registered in `cmd/api/main.go` |
| "Módulo lógico de acompanhamento" | `service-order-tracking`'s `TrackingService` (service.go) |
| "Projeção de leitura reduzida" | `trackingRead` model + `trackingResponse` DTO (§3, §4) — never the admin `serviceorder.Response` |
| "Validador do token de acompanhamento" | `internal/shared/trackingtoken` (generic crypto helpers) + `TrackingService.Get`'s hash-and-match check (§2) |

## 2. New shared package: `internal/shared/trackingtoken`

Genuinely generic (crypto/rand + sha256, no business rule), same bar as
`internal/shared/token` (JWT) — belongs in `internal/shared/`, not duplicated per feature
(CLAUDE.md §9.3).

```go
package trackingtoken

// Generate returns a new opaque, high-entropy token: 32 random bytes
// (crypto/rand), hex-encoded (64 characters). requirements.md §3.2.
func Generate() (string, error)

// Hash returns the SHA-256 hash of raw, hex-encoded (64 characters), for
// storage/lookup. requirements.md §0 item 4 — only this value is ever
// persisted.
func Hash(raw string) string
```

## 3. Data model

New table, owned schema-wise by this feature, added to `docs/schema.sql` /
`docs/entities.md` per CLAUDE.md §10/§14:

```sql
CREATE TABLE IF NOT EXISTS service_order_tracking_tokens (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    service_order_id  UUID NOT NULL UNIQUE REFERENCES service_orders (id) ON DELETE CASCADE,
    token_hash        TEXT NOT NULL,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    revoked_at        TIMESTAMPTZ
);

CREATE UNIQUE INDEX IF NOT EXISTS ux_service_order_tracking_tokens_token_hash
    ON service_order_tracking_tokens (token_hash);
```

- `service_order_id UNIQUE`: one token per order, same 1:1-via-FK pattern already used by
  `quotes.service_order_id` (schema.sql:241) — matches requirements.md §3.2 ("bound
  exclusively to one service order") and the chosen issuance rule (one auto-generated at
  creation, no reissue endpoint in scope).
- `token_hash`, never the raw token (requirements.md §0 item 4). Unique-indexed since it is
  the lookup key.
- `revoked_at` nullable: `NULL` = active, non-`NULL` = revoked (requirements.md §3.3 /
  AUTO-1 — the column exists, no endpoint sets it yet in this slice).
- No `updated_at`: this row is set once at creation and, when a future revoke action
  exists, changes exactly one column with its own meaningful timestamp — same reasoning
  `docs/schema.sql`'s comment gives for omitting `updated_at` on event-trail tables
  (CLAUDE.md §8).
- Not added to the `set_updated_at` trigger loop, for the same reason.

## 4. Read model / public DTO (the "reduced read projection")

Package-local to `service-order-tracking`, never imported by or exported to
`internal/features/service-order`:

```go
// model.go
type trackingRead struct {
    Code       int64
    Status     string
    Vehicle    trackingVehicle
    Milestones []trackingMilestone
}

type trackingVehicle struct {
    LicensePlate string
    Brand        string
    Model        string
    Year         int
    Color        string
}

type trackingMilestone struct {
    Event          string
    PreviousStatus string
    NewStatus      string
    OccurredAt     time.Time
}
```

```go
// dto.go
type trackingResponse struct {
    Code       int64                 `json:"code"`
    Status     string                `json:"status"`
    Vehicle    trackingVehicleDTO    `json:"vehicle"`
    Milestones []trackingMilestoneDTO `json:"milestones"`
}
```

Deliberately excluded (requirements.md §3.4/§3.5, AC6–AC8): order `id`, `customerId`,
`vehicleId`/vehicle `id`/`code`/`status`, `notes`, `requestedServices`,
`createdAt`/`updatedAt`, and `ServiceOrderHistory.description`. This is why AC9 requires a
separate Go type from `serviceorder.Response` — reusing/embedding it would leak exactly
these fields.

## 5. Token issuance (touches `internal/features/service-order`)

Per requirements.md §0 item 1, `POST /api/v1/service-orders` now also issues the tracking
token, in the same transaction that creates the order and its `creation` history row
(`internal/features/service-order/repository.go` `Create`, RNF07-style atomicity — the
order must never exist without a tracking token). Cross-feature coupling is avoided the
same way `service-order` already avoids it for `vehicles`/`customers` (§0 header): no Go
import of `service-order-tracking` from `service-order` (or vice versa). Concretely:

- `ServiceOrderRepository.Create`'s signature changes from
  `Create(ctx, order) error` to `Create(ctx, order) (trackingToken string, err error)`.
- Inside the same `tx`, after the existing `service_order_history` insert:
  ```go
  rawToken, err := trackingtoken.Generate()
  // ...
  tx.Exec(ctx,
      `INSERT INTO service_order_tracking_tokens (service_order_id, token_hash) VALUES ($1, $2)`,
      order.ID, trackingtoken.Hash(rawToken))
  ```
  Both features depend on the same generic `internal/shared/trackingtoken` package —
  no feature-to-feature dependency is introduced, matching the `customerRef`/`vehicleRef`
  precedent of touching another feature's table via plain SQL, not its Go package.
- `serviceorder.CreateResult` gains a `TrackingToken string` field; `serviceorder.Response`
  (dto.go) gains a `"trackingToken"` JSON field, returned **only** in the `201` creation
  response (never on any other admin route, and never logged — RNF08).
- `specs/service-order-opening/design.md` is updated with a short cross-reference note
  (its own response contract table gains this field); its requirements are not
  renumbered/rewritten, since the new field doesn't change any of its existing rules.

## 6. `service-order-tracking` package layout

```
internal/features/service-order-tracking/
  doc.go
  model.go       — trackingRead, trackingVehicle, trackingMilestone (§4)
  dto.go         — trackingResponse + toTrackingResponse (§4)
  errors.go      — ErrOrderNotFound, ErrTokenInvalid
  repository.go  — TrackingRepository interface + Postgres impl (§7)
  service.go     — TrackingService.Get(ctx, code int64, rawToken string) (*trackingRead, error)
  handler.go     — RegisterRoutes, GET handler
  httpsupport.go — writeServiceError (apierror mapping, §8)
```

Package name: `servicetracking` (Go identifier; the directory keeps the
`service-order-tracking` kebab-case name for consistency with the sibling `service-order*`
spec folders, same divergence the existing `service-order` directory / `serviceorder`
package already has).

## 7. Repository

```go
type TrackingRepository interface {
    FindByCodeAndTokenHash(ctx context.Context, code int64, tokenHash string) (*trackingRead, error)
}
```

Single SQL query joining `service_orders`, `vehicles`, and
`service_order_tracking_tokens`, resolving the order by `code` first so a `404` (unknown
code) can be distinguished from a `401` (order exists, token doesn't match/is revoked) per
requirements.md §0 item 3 / AC3–AC5:

1. `SELECT id, status FROM service_orders WHERE code = $1` → no row: `ErrOrderNotFound`
   (404).
2. `SELECT 1 FROM service_order_tracking_tokens WHERE service_order_id = $1 AND token_hash = $2 AND revoked_at IS NULL`
   → no row: `ErrTokenInvalid` (401). This single condition covers a missing/garbled token
   (won't hash-match anything), a token issued for a different order (right hash, wrong
   `service_order_id`, satisfying AC5), and a revoked token (AC4).
3. On match, load the vehicle (`vehicles` table, same plain-column read pattern as
   `service-order`'s `findVehicleByID`) and the milestone trail
   (`SELECT event, previous_status, new_status, occurred_at FROM service_order_history WHERE service_order_id = $1 ORDER BY occurred_at`).

All three reads happen outside a transaction (read-only, no invariant to protect across
them — same reasoning as `service-order`'s post-commit `findServicesByIDs`).

## 8. Handler / error mapping

```go
mux.HandleFunc("GET /api/v1/acompanhamento/{codigo}", handler.get)
```

Registered directly (not wrapped in `requireAuth`) — requirements.md §4/AC10. `{codigo}`
is parsed as `int64`; a non-numeric value is treated the same as an unknown code (`404`),
since `service_orders.code` is always numeric and a malformed one can never match a real
order (same reasoning `service-order`'s own `parseUUIDs` already applies to malformed
ids).

Token comes from the `X-Tracking-Token` header (requirements.md §0 item 2); empty/missing
→ same `ErrTokenInvalid` path as a wrong token (AC2).

`writeServiceError` mapping, using `apierror` (the envelope `service-order` — this
feature's closest sibling — already uses):

| Error | Status | Code |
| --- | --- | --- |
| `ErrOrderNotFound` | 404 | `NOT_FOUND` (`apierror.NotFound`) |
| `ErrTokenInvalid` | 401 | `INVALID_TRACKING_TOKEN` |
| anything else | 500 | `INTERNAL_ERROR` (`apierror.Internal`) |

`apierror` currently has no 401 constructor (every existing `apierror`-using feature sits
behind `middleware.RequireAuth`, which returns 401 via `httpx` before a handler is ever
reached — CLAUDE.md §8). This feature is the first `apierror` consumer that produces its
own 401, so `apierror` gains one generic addition, same shape as the existing `Conflict`:

```go
func Unauthorized(code, message string) *Error {
    return &Error{Status: http.StatusUnauthorized, Code: code, Message: message}
}
```

## 9. Logging (RNF08)

The project has no request/response body-logging middleware today (verified: `main.go` and
`internal/shared/middleware` log nothing but fatal startup errors). No new logging is added
by this feature, so the token is never logged by construction; this is called out
explicitly because RNF08 is one of the two listed non-functional requirements, not because
any existing code path needed changing.

## 10. Tests

- `internal/shared/trackingtoken/trackingtoken_test.go`: `Generate` returns 64-hex-char,
  non-repeating values; `Hash` is deterministic and injective enough for distinct inputs to
  differ.
- `internal/features/service-order-tracking/*_test.go`: unit tests for `TrackingService.Get`
  against a fake repository — unknown code (404 path), wrong/empty token, revoked token,
  cross-order token, valid token (AC1–AC5), and that the returned read model excludes
  `description` (AC6).
- `internal/features/service-order/*_test.go`: existing `Create` tests extended to assert
  `CreateResult.TrackingToken` is non-empty and that the persisted hash matches
  `trackingtoken.Hash(result.TrackingToken)`.
- `internal/handlers_test/service_order_tracking_test.go`: integration test (skips without
  `DATABASE_URL`, same convention as the other three integration files) covering AC1–AC5,
  AC7, AC8, AC10 end-to-end: create two orders via the real `POST /api/v1/service-orders`,
  extract each `trackingToken`, then exercise `GET /api/v1/acompanhamento/{codigo}` for
  valid access, no token, wrong token, order B's token against order A's code, and an
  unknown code — asserting both status codes and that CPF/CNPJ/phone/e-mail/customer name
  never appear in the response body.
