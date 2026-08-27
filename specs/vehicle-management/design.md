# Design — Vehicle Management

Satisfies: `requirements.md` (all sections). Reuses every architecture-wide convention fixed
by `specs/customer-management/design.md` (router, persistence, error envelope choice — see
§1.4) rather than reopening them; this document only makes the decisions that are specific to
Vehicle Management.

## 1. Architecture decisions

### 1.1 Slice granularity: one package per feature, not one per use case

Same decision, same rationale, as `specs/customer-management/design.md` §1.1 — the
originating prompt's `Modules/Atendimento/Veiculos/{Cadastrar,Consultar,Atualizar,Inativar}`
sketch is not followed literally, for the same reason the analogous `Clientes/Criar/`,
`Clientes/Consultar/`, ... sketch wasn't: four use cases sharing one model and one repository
don't justify four parallel folder trees. `internal/features/customer/` is the concrete,
already-implemented precedent for this project's actual convention.

```
internal/features/vehicle/
├── doc.go              → package comment
├── model.go             → Vehicle domain type + invariants (status transitions, etc.)
├── plate.go              → license plate Normalize/Validate (legacy + Mercosul)
├── dto.go                → request/response DTOs for the HTTP contract
├── repository.go        → VehicleRepository interface + Postgres implementation
├── service.go            → VehicleService: one method per use case (Create, Get, ...)
├── handler.go             → http.HandlerFunc per endpoint + RegisterRoutes(mux, service)
├── httpsupport.go         → JSON decode/encode, path/query parsing, service-error → HTTP mapping
├── errors.go              → feature-level sentinel errors (ErrNotFound, ErrDuplicatePlate, ...)
├── model_test.go, plate_test.go, service_test.go
```

Package name is `vehicle` (singular), mirroring `customer` (singular) even though the table
and routes are plural — consistent with the existing precedent, not a new convention.

### 1.2 Plate validation stays feature-local, not `internal/shared/`

Unlike CPF/CNPJ (`internal/shared/document`), which `customer-management/design.md` §1.5
justified as a "generic Brazilian tax-document algorithm" — reusable by any future feature
that needs to identify a person or company — a license plate identifies only a `Vehicle`.
No other entity in `docs/entities.md` has a plate-like field, and there is exactly one
consumer today. Per `CLAUDE.md` §9.3 ("`internal/shared/` is only for what is truly generic")
and the precedent already set by `customer-management/design.md` §1.5's pagination-parsing
call ("extract to `shared` the moment a second feature needs the same parsing"), plate
normalization/validation lives in `internal/features/vehicle/plate.go`, not
`internal/shared/`. Move it if a second feature ever needs it — not before.

Unlike CPF/CNPJ, there is no official check-digit algorithm for Brazilian plates, so
`plate.go` is structural-only:

- `Normalize(raw string) string` — strips formatting characters (spaces, `-`) and uppercases
  letters, same shape as `document.Normalize`.
- `Validate(normalized string) error` — a normalized plate is valid if it is exactly 7
  characters matching `[A-Z]{3}[0-9][A-Z0-9][0-9]{2}`. This single pattern accepts both
  formats at once: position 5 (0-indexed 4) is a plain digit for a legacy plate
  (`AAA9999`) and a letter for a Mercosul plate (`AAA9A99`) — the character class
  `[A-Z0-9]` covers both without needing two separate patterns or a format-detection step,
  unlike CPF vs. CNPJ (which differ by length, not by a single position).
- A single exported `plate.New(raw string) (Plate, error)` (or, kept as free functions if a
  wrapper struct proves unnecessary — decided during implementation, not a business rule)
  composes normalize → validate, mirroring `document.New`'s shape without inventing a second,
  incompatible validation idiom in the same codebase.

### 1.3 Application layer — customer-existence/active check without cross-feature coupling

`requirements.md` BR1 requires checking, at vehicle creation, that the referenced customer
exists and is `ACTIVE`. `CLAUDE.md` §9's "no feature imports another feature's package
directly" rule (already respected between `auth` and `customer`) still applies here.

**Decision**: `VehicleService` depends on a small interface it declares itself, mirroring how
`auth.Service` depends on `UserFinder`/`TokenIssuer` rather than concrete types
(`specs/auth/design.md` §3):

```go
// in internal/features/vehicle/service.go
type CustomerLookup interface {
    // IsActiveCustomer reports whether customerID refers to an existing
    // customer and, if so, whether that customer is currently ACTIVE.
    IsActiveCustomer(ctx context.Context, customerID uuid.UUID) (found bool, active bool, err error)
}
```

`cmd/api/main.go` — the only place that already imports both `customer` and (now) `vehicle`
— supplies the concrete implementation, a small adapter wrapping the existing
`*customer.CustomerService`:

```go
type customerLookupAdapter struct{ service *customer.CustomerService }

func (adapter customerLookupAdapter) IsActiveCustomer(ctx context.Context, id uuid.UUID) (bool, bool, error) {
    found, err := adapter.service.Get(ctx, id)
    if errors.Is(err, customer.ErrNotFound) {
        return false, false, nil
    }
    if err != nil {
        return false, false, err
    }
    return true, found.IsActive(), nil
}
```

This keeps `internal/features/vehicle/` free of any import of `internal/features/customer/`
— the composition happens only in `main.go`, exactly like every other cross-feature wiring in
this project.

### 1.4 Error envelope: reuse `internal/shared/apierror`

`CLAUDE.md` §8 leaves the `httpx` vs. `apierror` choice open project-wide, but is explicit
that a new feature should "use whichever of the two the feature it most resembles already
uses" rather than add a third shape. Vehicle Management is, by every structural measure
(registry entity linked to a customer, create/retrieve/update/logical-deactivate, a
uniqueness constraint with a dedicated conflict code, a `details`-bearing validation error),
the same shape of feature as Customer Management — not Auth. **Decision: `apierror`**, same
HTTP-status conventions (400 for every validation failure, structural or business; no 422 —
`customer-management/design.md` §1.5's rationale applies unchanged).

New status/code mapping this feature adds on top of the existing `apierror` constructors:

| Situation | Status | `error.code` |
| --- | --- | --- |
| Malformed JSON body | 400 | `INVALID_BODY` |
| Missing/invalid field, invalid plate format, year out of range | 400 | `VALIDATION_ERROR` |
| Vehicle not found (by id or plate) | 404 | `NOT_FOUND` |
| Referenced customer does not exist | 404 | `CUSTOMER_NOT_FOUND` |
| Referenced customer exists but is `INACTIVE` | 409 | `CUSTOMER_INACTIVE` |
| Plate already belongs to another vehicle | 409 | `DUPLICATE_LICENSE_PLATE` |

`CUSTOMER_NOT_FOUND` is a distinct code from the vehicle's own `NOT_FOUND` so a client can
tell "the vehicle id you asked for doesn't exist" apart from "the customerId you're trying to
link doesn't exist" — same reasoning `customer-management` used to split `DUPLICATE_DOCUMENT`
from `DUPLICATE_EMAIL` instead of a single ambiguous conflict code.

`CUSTOMER_INACTIVE` is `409`, not `400`: unlike a malformed field, the request is
well-formed and the customer id is syntactically valid — the failure is a **state**
conflict (this customer, right now, is not eligible), the same category `409` already covers
for a duplicate plate. This mirrors the future Service-Order-vs-inactive-vehicle rule this
feature itself sets a precedent for (§7.1).

### 1.5 API layer — routing and JWT

- Routes registered via `vehicle.RegisterRoutes(mux, service, requireAuth)` on the same
  `*http.ServeMux` `main.go` already builds, same method-aware pattern style as `customer`
  (`"POST /api/v1/vehicles"`, `"GET /api/v1/vehicles/{id}"`, ...).
  **Implementation-time correction**: the customer-scoped listing was originally specified as
  the nested `GET /api/v1/customers/{customerId}/vehicles`, registered by `vehicle` itself
  (it returns `Vehicle` resources, this feature's own responsibility) — not by `customer`,
  consistent with "no feature imports another feature," the URL prefix being just a path
  string, not a Go dependency. **That route panics `http.ServeMux` at process startup**: Go
  1.22's mux requires that of any two patterns matching an overlapping set of paths, one be
  strictly more specific than the other, and `/api/v1/customers/{customerId}/vehicles` is
  genuinely ambiguous against customer's own already-shipped
  `GET /api/v1/customers/document/{document}` — both would match a path like
  `/api/v1/customers/document/vehicles` (as a document lookup for `"vehicles"`, or as a
  vehicle listing for customer `"document"`), and neither pattern is a subset of the other.
  This was caught by actually starting the process (`go run ./cmd/api` panics immediately),
  not by `go build`/`go vet`/`go test`, none of which start a real `*http.ServeMux`.
  **Shipped instead**: `GET /api/v1/vehicles/customer/{customerId}` — same 3-segment shape as
  the already-working `GET /api/v1/vehicles/plate/{plate}` (a literal second segment
  disambiguating from the `{id}` 2-segment route), fully contained within `vehicle`'s own
  path prefix, so it can never collide with any route another feature registers. Fixing this
  by renaming/restructuring `customer`'s pre-existing `document` route instead was not an
  option — that route is out of scope for this feature and already shipped.
- **All seven routes are wrapped in `middleware.RequireAuth`** (`requirements.md` §6/RNF02),
  reusing the existing `auth` feature's token verification exactly as `GET /api/v1/auth/me`
  already does — no new JWT/middleware code. `vehicle.RegisterRoutes` takes the
  `requireAuth` middleware as a plain `func(http.Handler) http.Handler` parameter (the same
  value `middleware.RequireAuth(tokens)` already produces for auth's own route), so this
  package never needs to import `internal/shared/middleware` or hold a token manager itself;
  `cmd/api/main.go` passes it through unchanged.
- **Pagination**: same `page`/`pageSize` convention as `customer` (`design.md` §1.5:
  default `1`/`20`, max `pageSize` `100`, out-of-range values clamped, not rejected), applied
  to both `GET /api/v1/vehicles` and `GET /api/v1/vehicles/customer/{customerId}`. The
  originating prompt does not mention pagination explicitly, but RNF04 ("contratos REST
  consistentes") is; introducing a second, unpaginated list contract in the same codebase
  right next to the paginated one `customer` already established would itself be the
  inconsistency RNF04 warns against. Parsing/response-building is shared by `list` and
  `listByCustomer` via `vehicle/httpsupport.go`'s `parsePagination`/`toListResponse` (two
  callers within the feature, so extraction stays local — same "not yet extracted to
  `shared`" reasoning as `customer/httpsupport.go`'s own pagination helpers; the trigger for
  moving to `shared` remains a second *feature* needing the same parsing, per
  `customer-management/design.md` §1.5, not reached yet).
- **Response envelope**: same as `customer` — the resource (or the `{data, page, pageSize,
  total, totalPages}` page envelope) directly, no extra wrapper.

## 2. Domain model

### 2.1 Vehicle (updated)

`docs/entities.md`'s `Vehicle` entity does not yet have a `status` field, which this
feature's requirements need (`requirements.md` BR6-BR8). Per `CLAUDE.md` §10, this is added
to `docs/entities.md`, `docs/schema.sql`, and the Go code together — the same treatment
`customer-management` gave `Customer.status`/`documentType`:

| Field | Type | Description |
| --- | --- | --- |
| id | uuid | Unchanged. |
| code | number | Unchanged. |
| licensePlate | string | Unchanged, but now always stored normalized (7 uppercase alphanumeric characters, legacy or Mercosul format). |
| brand | string | Unchanged. |
| model | string | Unchanged. |
| year | number | Unchanged, now constrained to 1950..currentYear+1. |
| color | string | Unchanged. |
| customerId | uuid | Unchanged. Immutable after creation (requirements.md §3.2). |
| status | string | **New.** `ACTIVE` or `INACTIVE`. Starts `ACTIVE`; see BR6-BR7. |
| createdAt | string | Unchanged. |
| updatedAt | string | Unchanged. |

### 2.2 Invariants

- A `Vehicle` cannot exist with a structurally invalid or unnormalized plate — enforced by
  `Vehicle`'s constructor, the only path that sets a vehicle's plate (plate is immutable
  after creation, so there is no `ChangePlate` method, unlike `Customer.ChangeDocument`).
- A `Vehicle` always starts `ACTIVE`; there is no constructor path that creates one
  `INACTIVE`.
- `Deactivate()` is idempotent-safe at the domain level (calling it twice does not error);
  the service layer surfaces `404` only if the vehicle doesn't exist at all, exactly mirroring
  `Customer.Deactivate()` (`customer-management/design.md` §2.2).
- There is no `Activate()` method (requirements.md BR7).
- There is no method that changes `CustomerID` (requirements.md §3.2/§7.2 — out of scope).

## 3. Persistence design

### 3.1 Schema changes (`docs/schema.sql`)

```sql
DO $$ BEGIN
    CREATE TYPE vehicle_status AS ENUM ('ACTIVE', 'INACTIVE');
EXCEPTION WHEN duplicate_object THEN NULL; END $$;
```

plus one new column on the existing `vehicles` table: `status vehicle_status NOT NULL
DEFAULT 'ACTIVE'`. Applied the same way `customer_status`/`customer_document_type` were —
directly in the `CREATE TABLE vehicles` statement (schema is only ever applied against a
fresh Docker volume, `customer-management/design.md` §1.4) — not as a separate `ALTER TABLE`.
`docs/seed.sql`'s existing `vehicles` insert is updated to supply `status` explicitly.

The existing `ux_vehicles_license_plate` unique index (`docs/schema.sql`, already present)
already satisfies `requirements.md` BR4 at the database level; no new index is required for
plate uniqueness. The existing `vehicles.customer_id REFERENCES customers(id) ON DELETE
RESTRICT` foreign key already guarantees referential integrity for `customerId`; it does
**not**, by itself, enforce "customer must be `ACTIVE`" (Postgres FKs don't see other
columns) — that half of BR1 is an application-level check via `CustomerLookup` (§1.3).

### 3.2 Repository interface

```go
type VehicleRepository interface {
    Create(ctx context.Context, vehicle *Vehicle) error
    FindByID(ctx context.Context, id uuid.UUID) (*Vehicle, error)
    FindByPlate(ctx context.Context, normalizedPlate string) (*Vehicle, error)
    // ExistsByPlate reports whether normalizedPlate already belongs to a
    // vehicle other than excludeID (nil to check against every vehicle).
    ExistsByPlate(ctx context.Context, normalizedPlate string, excludeID *uuid.UUID) (bool, error)
    List(ctx context.Context, page, pageSize int) ([]*Vehicle, int, error)
    ListByCustomer(ctx context.Context, customerID uuid.UUID, page, pageSize int) ([]*Vehicle, int, error)
    Update(ctx context.Context, vehicle *Vehicle) error
}
```

- `ExistsByPlate` mirrors `CustomerRepository.ExistsByDocument`'s shape exactly (application
  pre-check for a clean `409` in the common case; the database's unique index, mapped via
  `pgconn.PgError.ConstraintName == "ux_vehicles_license_plate"`, is still the final guard
  against a race between two concurrent requests — same two-layer defense as
  `customer-management/design.md` §1.4).
- `Update` persists the full row (brand, model, year, color, status) — the service merges
  the partial `PATCH` input into the loaded `Vehicle` before calling `Update`; plate and
  customer id are never part of what `Update` changes, since neither is ever mutated after
  creation (requirements.md §3.2).
- `ListByCustomer` is a distinct method, not `List` with an optional filter parameter,
  because `GET /api/v1/vehicles/customer/{customerId}` needs to `404` when the customer
  itself doesn't exist (a check the plain paginated `List` has no notion of) — see §4.5.

## 4. API contract

Base path: `/api/v1/vehicles` (every endpoint, including the customer-scoped listing — see
§1.5's implementation-time routing correction). All request/
response bodies are JSON. Every route below requires `Authorization: Bearer <token>`
(`design.md` §1.5); a missing/invalid/expired token gets the existing `401` response from
`middleware.RequireAuth`, unchanged.

### 4.1 `POST /api/v1/vehicles`

Request:
```json
{ "licensePlate": "ABC1D23", "brand": "Fiat", "model": "Uno", "year": 2018, "color": "White", "customerId": "a0000000-...-000000000001" }
```
- All six fields required.
- `licensePlate` normalized then validated (legacy or Mercosul, `plate.go`).
- `year` must be within 1950..currentYear+1.
- `customerId` must reference an existing, `ACTIVE` customer.
- `201 Created`, `Location: /api/v1/vehicles/{id}`, body = full vehicle (status `ACTIVE`).
- `400` — missing/invalid field, invalid plate, year out of range.
- `404` — `customerId` does not reference an existing customer (`CUSTOMER_NOT_FOUND`).
- `409` — the customer exists but is `INACTIVE` (`CUSTOMER_INACTIVE`), or the plate already
  belongs to another vehicle (`DUPLICATE_LICENSE_PLATE`).

### 4.2 `GET /api/v1/vehicles`

Query params: `page`, `pageSize` (§1.5). Includes active and inactive vehicles (no implicit
filter — mirrors `requirements.md` BR8).
- `200 OK`, body = `{ "data": [Vehicle...], "page", "pageSize", "total", "totalPages" }`.

### 4.3 `GET /api/v1/vehicles/{id}`

- `200 OK`, body = `Vehicle`. `404` if not found.

### 4.4 `GET /api/v1/vehicles/plate/{plate}`

- `{plate}` is normalized (formatting stripped, uppercased) before lookup, so both a raw and
  a formatted path segment resolve the same vehicle — mirrors
  `GET /api/v1/customers/document/{document}`.
- `200 OK`, body = `Vehicle`. `404` if not found.

### 4.5 `GET /api/v1/vehicles/customer/{customerId}`

Query params: `page`, `pageSize` (§1.5).
- `404` (`CUSTOMER_NOT_FOUND`) if `customerId` does not reference an existing customer — an
  inactive customer's vehicles are still listed (read paths never hide inactive records,
  BR8/`customer-management` BR3.8's same principle applied here).
- `200 OK`, body = the same paginated envelope as §4.2, scoped to that customer. An existing
  customer with no vehicles returns `200` with an empty `data` array, not `404`.

### 4.6 `PATCH /api/v1/vehicles/{id}`

Request (all fields optional, partial update — requirements.md §3.2/BR9):
```json
{ "brand": "Fiat", "model": "Uno Mille", "year": 2019, "color": "Red" }
```
- Only `brand`, `model`, `year`, and `color` are accepted; `licensePlate` and `customerId`
  are not part of this request's shape at all (not merely ignored — omitted from the DTO), so
  a client cannot even attempt to change them through this endpoint.
- `200 OK`, body = updated `Vehicle`. `400` invalid year. `404` not found.

### 4.7 `DELETE /api/v1/vehicles/{id}`

- Logical deactivation, not physical delete (requirements.md BR7). Since this feature
  exposes no physical-delete endpoint at all, "a vehicle linked to a service order can never
  be physically deleted" (the originating prompt's business rule) holds trivially for every
  vehicle, regardless of service-order linkage — there is nothing in this feature's contract
  that could physically delete a row either way.
- `200 OK`, body = updated `Vehicle` (status `INACTIVE`). `404` if not found.
- Idempotent: deactivating an already-inactive vehicle returns `200` with no change made.

### 4.8 `Vehicle` response shape

```json
{
  "id": "b0000000-...-000000000001",
  "code": 1,
  "licensePlate": "ABC1D23",
  "brand": "Fiat",
  "model": "Uno",
  "year": 2018,
  "color": "White",
  "customerId": "a0000000-...-000000000001",
  "status": "ACTIVE",
  "createdAt": "2026-08-17T12:00:00Z",
  "updatedAt": "2026-08-17T12:00:00Z"
}
```

## 5. Validation

- `internal/features/vehicle/plate.go` (see §1.2) — `Normalize`, `Validate`, composed into a
  constructor used by `model.go`, never a regex-only shortcut applied ad hoc in the handler.
- Field-level request validation (required fields present, non-empty; year within range) is a
  `Validate() []apierror.Detail` method on `CreateRequest`, called by `vehicle/handler.go`
  before invoking the service — same colocation convention as
  `customer.CreateRequest.Validate()` (`customer-management/design.md` §5).
- The customer-exists/customer-active check (BR1) is **not** field-level validation — it
  depends on external state (the customer's current status), so it lives in `service.go`
  via `CustomerLookup` (§1.3), the same layering `customer-management` uses for document
  uniqueness (a request-shape check can't answer it alone either).

## 6. Testing strategy

- **Unit tests**, stdlib `testing` + `testify` (matches `customer`, the feature this one most
  resembles — `requirements.md` §8's "reuse what the feature it most resembles already uses"
  logic applied to the test-library choice too, not just the error envelope):
  - `internal/features/vehicle/plate_test.go`: table-driven valid/invalid/formatted/
    normalized cases for both legacy and Mercosul formats.
  - `internal/features/vehicle/model_test.go`: vehicle starts `ACTIVE`; `Deactivate()`
    transitions and is idempotent; no `Activate()` exists; no plate/customer-id mutator
    exists.
  - `internal/features/vehicle/service_test.go`: an in-memory fake `VehicleRepository` (plain
    map-backed struct, no mocking framework — same style as `customer`'s
    `fake_repository_test.go`) plus a fake `CustomerLookup` drive: create with an active
    customer (success), create against a nonexistent customer (404), create against an
    inactive customer (409), duplicate plate (409), invalid plate (400), year out of range
    (400), get by id/plate, list, list-by-customer against a nonexistent customer (404),
    update brand/model/year/color, deactivate (idempotent), not-found cases.
- **Integration tests**, `internal/handlers_test/vehicle_test.go` (real `*http.ServeMux` +
  real `pgxpool.Pool`, `t.Skip` when `DATABASE_URL` is unreachable — same pattern as
  `customer_test.go`):
  - A request without a valid bearer token gets `401` on every route (RNF02).
  - Full CRUD flow over real HTTP, with a valid token: create (active customer), get by id,
    get by plate, list, list-by-customer, update, deactivate.
  - Invalid plate (both a garbage string and a plate matching neither format), year outside
    1950..currentYear+1, duplicate plate on create, customer not found, customer inactive.
  - Pagination on both list endpoints.
  - Logical deactivation is not a physical delete: the row is still retrievable by id/plate/
    listing after `DELETE`, with `status: "INACTIVE"`.
  - A concurrent-duplicate-plate race is caught by the database constraint, not just the
    application pre-check (mirrors `customer_test.go`'s equivalent case for `document`).
  - Each test creates its own customer(s) (a valid, active customer is a precondition for
    most vehicle tests) and vehicles with randomly generated valid plates, cleaning up its own
    rows afterward — same independence approach as `customer_test.go`.

## 7. Traceability

Every decision above satisfies a specific `requirements.md` item; `tasks.md` breaks this
design into ordered implementation steps, each referencing the section here it implements.
