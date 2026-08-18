# Design — Service Order Opening

Satisfies: `requirements.md` (all sections). This is the second feature implemented in the
project; it follows the conventions already fixed by `specs/customer-management/design.md`
(routing, error envelope, package layout, testing) instead of reopening them.

## 1. Architecture decisions

### 1.1 Slice granularity

Same decision as `specs/customer-management/design.md` §1.1: one Go package per feature,
organized internally by responsibility file, not one folder per use case. This feature has
a single use case (create), so the package is intentionally small:

```
internal/features/service-order/
├── doc.go              → package comment
├── model.go             → ServiceOrder domain type + Status + invariants
├── dto.go                → CreateRequest / Response DTOs
├── repository.go        → ServiceOrderRepository interface + Postgres implementation
├── service.go            → ServiceOrderService.Create (the only use case)
├── handler.go             → http.HandlerFunc + RegisterRoutes(mux, service)
├── httpsupport.go         → JSON decode/encode, service-error → HTTP mapping
├── errors.go              → feature-level sentinel errors
├── model_test.go
├── service_test.go
```

### 1.2 Domain layer

- `ServiceOrder` (`model.go`) is the aggregate: `ID`, `Code`, `CustomerID`, `VehicleID`,
  `OpenedAt`, `Status`, `Notes`, `RequestedServiceIDs []uuid.UUID`, `CreatedAt`,
  `UpdatedAt`.
- `Status` is a string type holding the Portuguese enum values already fixed by
  `docs/entities.md`/`docs/schema.sql` (`RECEBIDA`, `EM_DIAGNOSTICO`, ...) — this feature
  only ever produces `RECEBIDA`; it does not implement any transition.
- `NewServiceOrder(customerID, vehicleID uuid.UUID, notes string, requestedServiceIDs
  []uuid.UUID) (*ServiceOrder, error)` is the only constructor; it always sets
  `Status = RECEBIDA`. There is no setter for `Status` and no other constructor — mirrors
  `customer.NewCustomer` always producing `StatusActive` with no way to construct
  otherwise.
- Validation this constructor performs: `requestedServiceIDs` must be non-empty is **not**
  required by any rule in `requirements.md` (a service order can be opened with zero
  initially-requested services, e.g. "just look at it" cases) — so an empty slice is
  valid. What the constructor does enforce: `customerID`/`vehicleID` are non-nil UUIDs.
  Existence/active/ownership checks are **not** the aggregate's job — they need repository
  access, so they live in `ServiceOrderService.Create` (§1.3), consistent with how
  `customer.Service.Create`'s document-uniqueness check lives in the service, not
  `NewCustomer`.

### 1.3 Application layer

- `ServiceOrderService.Create(ctx, input CreateInput) (*ServiceOrder, error)` is the one
  use case:
  1. Resolve the customer — by `CustomerID` or by normalized `CustomerDocument`
     (`internal/shared/document.Normalize`, reused, not duplicated).
  2. Return `ErrCustomerNotFound` / `ErrCustomerInactive` if not found / not `ACTIVE`.
  3. Resolve the vehicle — by `VehicleID` or by `LicensePlate`.
  4. Return `ErrVehicleNotFound` / `ErrVehicleInactive` if not found / not `ACTIVE`.
  5. Return `ErrVehicleNotOwnedByCustomer` if the vehicle's `customer_id` does not match
     the resolved customer.
  6. Validate every `RequestedServiceIDs` entry exists in `services`; return
     `ErrRequestedServiceNotFound` (naming the missing id) otherwise.
  7. Build the aggregate via `NewServiceOrder`.
  8. Call `repository.Create`, which performs the transactional insert (order + requested
     services + first history event, §3.2).
- No CQRS split — one service method, same as `customer.Service`.

### 1.4 Persistence

- `ServiceOrderRepository` is implemented against `pgx v5` (`pgxpool.Pool`), injected from
  `cmd/api/main.go`, same as `customer.CustomerRepository`.
- **First explicit transaction in the project.** `auth` and `customer` only ever write to
  one table per call; this feature's `Create` writes to three tables
  (`service_orders`, `service_order_requested_services`, `service_order_history`) and
  RNF07 requires all-or-nothing. `Create` wraps the three inserts in
  `pool.Begin(ctx)` / `tx.Commit(ctx)`, with `defer tx.Rollback(ctx)` (a no-op after a
  successful commit, per pgx's documented behavior) as the safety net for any early
  return/panic. This is a new pattern for the codebase — call this out in the PR
  description, since every future feature that writes to more than one table should reuse
  this shape rather than reinvent it.
- Read-only helpers this feature needs — and that do **not** belong to any other feature's
  Go package (no cross-feature import, `CLAUDE.md` §9.2) — live in the same
  `repository.go`, as unexported methods on `PostgresServiceOrderRepository`, each a single
  parameterized query:
  - `findActiveCustomerByID(ctx, id uuid.UUID) (*customerRef, error)`
  - `findActiveCustomerByDocument(ctx, normalizedDocument string) (*customerRef, error)`
  - `findActiveVehicleByID(ctx, id uuid.UUID) (*vehicleRef, error)`
  - `findActiveVehicleByPlate(ctx, plate string) (*vehicleRef, error)`
  - `findMissingServiceIDs(ctx, ids []uuid.UUID) ([]uuid.UUID, error)`

  `customerRef`/`vehicleRef` are small private structs (`ID`, `Status`, and — for vehicle —
  `CustomerID`) local to this package; they are **not** the `customer.Customer` /
  future `vehicle.Vehicle` domain types, precisely to avoid importing another feature's
  package. These queries read `customers.status`/`vehicles.status` directly, which is safe
  because both are plain database columns documented in `docs/schema.sql`, not an internal
  detail of another Go package.
- `docs/schema.sql` changes (new `vehicles.status`, new `service_order_requested_services`
  table) are applied the same way `customer-management` did its schema change: directly in
  `CREATE TABLE`, since no shared/production volume exists yet
  (`docker compose down -v && docker compose up -d` to pick it up locally).

### 1.5 API layer

- Reuses `internal/shared/apierror` (no new error envelope).
- **HTTP status mapping**:

  | Situation | Status | `error.code` |
  | --- | --- | --- |
  | Malformed JSON body | 400 | `INVALID_BODY` |
  | Missing field, or both/neither of a customer or vehicle identifier pair given | 400 | `VALIDATION_ERROR` |
  | Customer not found | 404 | `CUSTOMER_NOT_FOUND` |
  | Customer inactive | 409 | `CUSTOMER_INACTIVE` |
  | Vehicle not found | 404 | `VEHICLE_NOT_FOUND` |
  | Vehicle inactive | 409 | `VEHICLE_INACTIVE` |
  | Vehicle belongs to a different customer | 409 | `VEHICLE_NOT_OWNED_BY_CUSTOMER` |
  | A requested service id does not exist | 404 | `SERVICE_NOT_FOUND` |

  **Decision: 409, not 400, for "inactive" and "not owned by customer."** These are not
  malformed input — the request is well-formed and the referenced ids are real — the
  request conflicts with the current state of those resources, which is exactly what 409
  already means for `customer-management` (`DUPLICATE_DOCUMENT`). "Not found" stays 404,
  same convention as `customer-management`.
- **Response envelope**: single resource, no wrapper, matching `customer-management`.

## 2. Domain model

### 2.1 `Vehicle.status` (new)

`docs/entities.md`'s `Vehicle` entity has no status field. This feature adds it — required
by `requirements.md` §3 — mirroring exactly what `customer-management/design.md` §2.1 did
for `Customer.status`:

| Field | Type | Description |
| --- | --- | --- |
| status | string | **New.** `ACTIVE` or `INACTIVE`. Starts `ACTIVE`. No behavior for transitioning it is implemented by this feature — that belongs to vehicle management (§7.2 of `requirements.md`); this feature only reads it. |

### 2.2 `ServiceOrder.requestedServices` (new)

`docs/entities.md`'s `ServiceOrder` entity lists `quote` but not the services initially
requested. Add:

| Field | Type | Description |
| --- | --- | --- |
| requestedServices | Service[] | Services initially requested when the order was opened — the customer's stated demand, not the definitive priced quote (see `Quote`, not yet built on top of this). |

### 2.3 Invariants

- A `ServiceOrder` cannot be constructed with any status other than `RECEBIDA`.
- A `ServiceOrder` cannot be constructed without a valid customer and vehicle id (existence
  and business-rule checks happen in the service layer, not the constructor — see §1.2).

## 3. Persistence design

### 3.1 Schema changes (`docs/schema.sql`)

```sql
DO $$ BEGIN
    CREATE TYPE vehicle_status AS ENUM ('ACTIVE', 'INACTIVE');
EXCEPTION WHEN duplicate_object THEN NULL; END $$;
```

`vehicles.status vehicle_status NOT NULL DEFAULT 'ACTIVE'` added to the `CREATE TABLE
vehicles` statement, plus a matching `COMMENT ON COLUMN`.

New join table, same shape as `quote_services` but without a price column (§7.1 of
`requirements.md` — requested services are not priced):

```sql
CREATE TABLE IF NOT EXISTS service_order_requested_services (
    service_order_id  UUID NOT NULL REFERENCES service_orders (id) ON DELETE CASCADE,
    service_id        UUID NOT NULL REFERENCES services (id) ON DELETE RESTRICT,
    PRIMARY KEY (service_order_id, service_id)
);

CREATE INDEX IF NOT EXISTS ix_service_order_requested_services_service_id
    ON service_order_requested_services (service_id);
```

`docs/seed.sql`'s existing `INSERT INTO vehicles` statements need the new `status` column
(all `ACTIVE`, matching the customers they belong to — except possibly one to exercise the
`INACTIVE` rejection path in integration tests, mirroring how `docs/seed.sql` already keeps
one `INACTIVE` customer, `a0000000-...0004`).

### 3.2 Repository interface

```go
type ServiceOrderRepository interface {
    Create(ctx context.Context, order *ServiceOrder) error
}
```

Kept minimal and use-case-shaped (only what `ServiceOrderService.Create` needs), consistent
with `CLAUDE.md` §26 (no speculative CRUD methods for reads this feature doesn't perform —
there is no "get order by id" endpoint in this card). The read-only customer/vehicle/service
lookups from §1.4 are not part of this exported interface; they are private implementation
details of `PostgresServiceOrderRepository`, since `ServiceOrderService`'s fake-repository
unit tests (§6) only need to fake the single `Create` call plus a small lookup interface —
see below.

To keep `service_test.go` able to fake customer/vehicle/service resolution without a real
database, `ServiceOrderService` actually depends on two interfaces, not one:

```go
// Defined in service.go, next to ServiceOrderService, mirroring
// CustomerRepository's placement in customer/repository.go.
type serviceOrderLookups interface {
    findActiveCustomerByID(ctx context.Context, id uuid.UUID) (*customerRef, error)
    findActiveCustomerByDocument(ctx context.Context, normalizedDocument string) (*customerRef, error)
    findActiveVehicleByID(ctx context.Context, id uuid.UUID) (*vehicleRef, error)
    findActiveVehicleByPlate(ctx context.Context, plate string) (*vehicleRef, error)
    findMissingServiceIDs(ctx context.Context, ids []uuid.UUID) ([]uuid.UUID, error)
}

type ServiceOrderRepository interface {
    Create(ctx context.Context, order *ServiceOrder) error
}
```

`PostgresServiceOrderRepository` implements both; `ServiceOrderService` takes both as two
constructor parameters (in practice the same concrete value from `main.go`, but the split
keeps `service_test.go`'s fake small and explicit about what each piece is for).

### 3.3 Transaction shape (`Create`)

```go
func (r *PostgresServiceOrderRepository) Create(ctx context.Context, order *ServiceOrder) error {
    tx, err := r.pool.Begin(ctx)
    if err != nil {
        return err
    }
    defer tx.Rollback(ctx) // no-op once Commit succeeds

    if err := tx.QueryRow(ctx,
        `INSERT INTO service_orders (id, customer_id, vehicle_id, notes)
         VALUES ($1, $2, $3, $4)
         RETURNING code, opened_at, created_at, updated_at`,
        order.ID, order.CustomerID, order.VehicleID, order.Notes,
    ).Scan(&order.Code, &order.OpenedAt, &order.CreatedAt, &order.UpdatedAt); err != nil {
        return err
    }

    for _, serviceID := range order.RequestedServiceIDs {
        if _, err := tx.Exec(ctx,
            `INSERT INTO service_order_requested_services (service_order_id, service_id) VALUES ($1, $2)`,
            order.ID, serviceID,
        ); err != nil {
            return err
        }
    }

    if _, err := tx.Exec(ctx,
        `INSERT INTO service_order_history (service_order_id, event, description, previous_status, new_status)
         VALUES ($1, 'creation', $2, 'RECEBIDA', 'RECEBIDA')`,
        order.ID, "Service order opened.",
    ); err != nil {
        return err
    }

    return tx.Commit(ctx)
}
```

Any error at any step rolls back all three writes together (RNF07). `status` itself is
never passed in on insert — the column's `DEFAULT 'RECEBIDA'` (already in `docs/schema.sql`)
is the single source of truth for the initial value, which is also why a `status` field in
the request body has nothing to bind to even if present (§8 of `requirements.md`).

### 3.4 Duplicate requested service ids

`requestedServiceIds` is not deduplicated by the service layer before being handed to
`Create` — no requirement asks for silent deduplication, and inventing one would be
exactly the kind of unrequested rule `CLAUDE.md` §17 warns against. In practice this means
a request listing the same service id twice fails the whole creation (via the
`service_order_requested_services` primary key, mid-transaction — see §3.3/§6), which is an
acceptable, honest `500`/`INTERNAL_ERROR` for a malformed-in-a-way-no-rule-covers request,
and doubles as the integration test that proves the transaction actually rolls back
(§5). If a future requirement wants duplicates silently ignored instead, that must be
specified explicitly, not assumed here.

## 4. API contract

### 4.1 `POST /api/v1/service-orders`

Request:
```json
{
  "customerId": "a0000000-0000-0000-0000-000000000001",
  "vehicleId": "b0000000-0000-0000-0000-000000000001",
  "requestedServiceIds": ["d0000000-0000-0000-0000-000000000001"],
  "notes": "Customer reported a light engine noise."
}
```
or, identifying customer/vehicle by document/plate instead of id:
```json
{
  "customerDocument": "123.456.789-09",
  "licensePlate": "ABC1D23",
  "requestedServiceIds": [],
  "notes": "Routine check."
}
```
- Exactly one of `customerId`/`customerDocument` must be present; exactly one of
  `vehicleId`/`licensePlate` must be present. `requestedServiceIds` and `notes` are
  optional (default: empty list / empty string).
- Any `status` field present in the body is ignored.
- `201 Created`, `Location: /api/v1/service-orders/{id}`, body = full order
  (status `RECEBIDA`).
- Error mapping per the table in §1.5.

### 4.2 `ServiceOrder` response shape

```json
{
  "id": "e7b1...uuid",
  "code": 1042,
  "customer": { "id": "a0000000-...-0001", "code": 1, "name": "João Pedro Silva" },
  "vehicle": { "id": "b0000000-...-0001", "code": 1, "licensePlate": "ABC1D23" },
  "openedAt": "2026-08-17T12:00:00Z",
  "status": "RECEBIDA",
  "notes": "Customer reported a light engine noise.",
  "requestedServices": [
    { "id": "d0000000-...-0001", "code": 1, "name": "Oil Change" }
  ],
  "createdAt": "2026-08-17T12:00:00Z",
  "updatedAt": "2026-08-17T12:00:00Z"
}
```

`customer`/`vehicle`/`requestedServices` are summarized (id, code, and the one or two
fields useful for display), not full `Customer`/`Vehicle`/`Service` payloads — this
endpoint owns its own response shape independent of those other features' DTOs, same
reasoning as the private `customerRef`/`vehicleRef` structs in §1.4. Building this summary
means `Create`/the handler needs the customer/vehicle names and the requested services'
names/codes; §3.2's `customerRef`/`vehicleRef` are extended with the display fields
(`Name`, `Code`) needed for this response, and requested-service display data is fetched
with one more read query after the transaction commits (a service-catalog lookup, not a
write, so it does not need to be inside the transaction).

## 5. Testing strategy

- **Unit tests** (stdlib `testing` + `testify`, same as `customer`):
  - `model_test.go`: `NewServiceOrder` always produces `RECEBIDA`; there is no way to
    construct any other status.
  - `service_test.go`: an in-memory fake implementing `ServiceOrderRepository` +
    `serviceOrderLookups` (plain struct backed by maps, no mocking framework) drives
    `Create` through: success (by id, by document/plate), customer not found/inactive,
    vehicle not found/inactive/not-owned, missing requested service, and confirms a
    `status` field is never read from the input at all (no field exists on `CreateInput`
    for it — enforced by the type, not by a runtime check).
- **Integration tests** (`internal/handlers_test/service_order_test.go`):
  - Real `*http.ServeMux` + real `pgxpool.Pool` via `DATABASE_URL`, `t.Skip` if unset, same
    as `customer_test.go`.
  - Full success flow using `docs/seed.sql` fixtures; each error case from §1.5's table;
    and the rollback case — `requestedServiceIds` is passed through to the transaction
    as-is, **without deduplication** (a deliberate choice, see §3.4): sending the same
    valid service id twice passes the pre-check (`findMissingServiceIDs`, both occurrences
    exist) but the second `INSERT` into `service_order_requested_services` then violates
    its `PRIMARY KEY (service_order_id, service_id)`, failing *after* the `service_orders`
    row has already been inserted in the same transaction. Assert the whole request fails
    (500, mapped generically since this isn't a user-facing business rule) and that
    `SELECT` on `service_orders` for that customer/vehicle afterwards shows no orphan row —
    this is the test that actually exercises RNF07's rollback guarantee end-to-end against
    real Postgres, not just the history-insert step in isolation.

## 6. Traceability

Every decision above satisfies a specific `requirements.md` item; `tasks.md` breaks this
design into ordered implementation steps, each referencing the section here it implements.
