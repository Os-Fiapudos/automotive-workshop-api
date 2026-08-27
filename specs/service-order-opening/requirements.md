# Requirements — Service Order Opening

Status: **Approved for implementation**
Feature folder: `internal/features/service-order/`

## 1. Context

This is the second business feature implemented in `automotive-workshop-api`, after
`internal/features/customer/` (see `specs/customer-management/`). `ServiceOrder` exists
today only as domain/schema documentation (`docs/entities.md`, `docs/schema.sql`) — no Go
code or persistence layer exists for it yet.

This feature depends on `Customer` (already implemented, `internal/features/customer/`)
being `ACTIVE` to open an order, per the future-integration invariant recorded in
`specs/customer-management/requirements.md` §7.1. It also depends on `Vehicle` existing and
being active and owned by the customer — `Vehicle` has no Go feature at all yet (a
teammate is building vehicle management separately, not yet branched). This feature does
**not** implement vehicle management; it only adds the minimal schema/read capability it
needs to validate a vehicle at order-opening time (see §7.2).

## 2. User story

> As a front-desk attendant,
> I want to create a service order,
> so that I can register a vehicle's service from the moment it is received.

## 3. Business rules

1. The customer must exist and be `ACTIVE`. An unknown or `INACTIVE` customer cannot open
   an order.
2. The customer is identified by a normalized CPF/CNPJ (reusing
   `internal/shared/document`) or by its id — exactly one of the two must be provided.
3. The vehicle must exist, be `ACTIVE`, and belong to the customer identified in the
   request. A vehicle belonging to a different customer, an inactive vehicle, or an unknown
   vehicle must all be rejected.
4. The vehicle is identified by its license plate or by its id — exactly one of the two
   must be provided.
5. The order receives a unique, human-readable `code`, assigned by the database
   (`service_orders.code`, already `GENERATED ALWAYS AS IDENTITY` in `docs/schema.sql`) —
   nothing new needed here.
6. The order's initial status is always `RECEIVED`. The API consumer cannot choose or
   override it: a `status` field present in the request body, if any, is ignored — it does
   not fail the request, it simply has no effect on the created order.
7. The services initially requested (`requestedServiceIds`) represent the customer's
   initial demand only. They are not priced and do not constitute the definitive quote
   (`Quote`/`RF05`, out of scope — see `specs/service-order-opening` future work, not yet
   specified). Each requested service id must reference an existing `Service` in the
   catalog; an unknown id rejects the whole request.
8. Creating the order must also record a `service_order_history` event
   (`event = 'creation'`, `previous_status = new_status = 'RECEIVED'` — see
   `docs/seed.sql`'s existing seed data for this exact convention already in use).
9. The service order row and its first history row must be created in the same database
   transaction (RNF07): if writing the history event fails, the service order creation
   must be rolled back — the API must never leave a service order without its creation
   history entry.

## 4. Scope

In scope for this feature:

- `ServiceOrder` domain aggregate: id, code, customer, vehicle, opened-at, status
  (always `RECEIVED` on creation), notes, requested services, timestamps.
- One use case: create a service order (`POST /api/v1/service-orders`).
- Validation: customer exists/active, vehicle exists/active/owned by customer, requested
  services exist.
- Transactional creation (order + requested services + first history event).
- `vehicles.status` (`ACTIVE`/`INACTIVE`) added to the schema — required by rule §3, but
  is not a vehicle-management feature; only what the create-order validation needs.
- Unit and integration tests covering the rules above.

Out of scope for this feature (see §7, "Future requirements"):

- Any other Service Order use case (diagnosis, quote composition/approval, execution,
  delivery, status transitions beyond the initial `RECEIVED`) — future cards (e.g. FP-16
  and beyond).
- Vehicle management (create/update/list/deactivate a vehicle) — a teammate's separate,
  not-yet-specified feature. This feature only reads `vehicles` for validation.
- Quote (`Orçamento`) — RF05/RF06, not implemented here; `requestedServiceIds` is not a
  quote.
- Authentication/authorization on the new endpoint (matches
  `specs/customer-management/requirements.md` §7.2 — still unauthenticated, project-wide
  "to be defined").

## 5. Endpoint covered (contract details in `design.md`)

```
POST /api/v1/service-orders
```

> **Naming note**: the originating task used `POST /api/v1/ordens-servico`. Per
> `CLAUDE.md` §8 (English domain language, with the single documented exception of
> `ServiceOrder.status` values) and the precedent already set by
> `specs/customer-management/requirements.md` §5 (`/api/v1/clientes` → `/api/v1/customers`),
> this feature uses `/api/v1/service-orders`. This is a deliberate, documented deviation
> from the literal task wording, not an invented requirement.

## 6. Acceptance criteria

```
[ ] Create an order for a valid, active customer and an active vehicle owned by that customer
[ ] Locate the customer by id
[ ] Locate the customer by normalized CPF/CNPJ
[ ] Reject when the customer does not exist
[ ] Reject when the customer is INACTIVE
[ ] Locate the vehicle by id
[ ] Locate the vehicle by license plate
[ ] Reject when the vehicle does not exist
[ ] Reject when the vehicle is INACTIVE
[ ] Reject when the vehicle belongs to a different customer than the one identified
[ ] The order always receives a unique code
[ ] The order's initial status is always RECEIVED
[ ] A status value sent in the request body is ignored, not applied, and does not fail the request
[ ] Requested services are persisted as the initial demand (no price)
[ ] Reject when a requested service id does not exist in the catalog
[ ] A creation history event is recorded (event=creation, previous=new=RECEIVED)
[ ] A failure writing the history event rolls back the whole order creation (no orphan order)
[ ] The response returns code, customer, vehicle, requested services, and status
[ ] Unit tests for the aggregate (always starts RECEIVED, no way to set another status)
[ ] Unit tests for service-layer validation (customer/vehicle/service rules) via an in-memory fake repository
[ ] Integration tests for the endpoint, including the transactional-rollback case
```

## 7. Future requirements (explicitly deferred, not implemented here)

### 7.1 Quote composition (RF05/RF06)

> The requested services recorded at order creation are not the definitive quote.

A future card (diagnosis + quote composition) will introduce `Orcamento`/`ItemOrcamento`
on top of the `ServiceOrder` created here, with automatic price calculation and status
transitions (`RECEIVED` → `IN_DIAGNOSIS` → ...). No code in this feature should
anticipate or stub that.

### 7.2 Vehicle management

> Full vehicle CRUD (create, update, list, deactivate) is a separate, not-yet-specified
> feature being built by a teammate.

This feature adds only `vehicles.status` to the schema (required by rule §3) and its own
read-only queries inside `internal/features/service-order/` to validate a vehicle at
order-creation time. It does not create `internal/features/vehicle/` or any public vehicle
endpoint. When vehicle management is specified, its authors should be aware
`vehicles.status` already exists (added here) to avoid a duplicate/conflicting schema
change.

### 7.3 JWT / administrative authentication

Same status as `specs/customer-management/requirements.md` §7.2 — not implemented by this
feature; a dedicated Security feature will add it project-wide later.

## 8. Open questions resolved before implementation

- **Status field in the request body**: ignored (not applied), rather than rejected with
  `400`. Chosen because RF04/RF08 only require the server to be the sole author of the
  initial status — silently ignoring an extraneous field is consistent with how optional/
  unrecognized JSON fields are already handled elsewhere in the project (no strict-decoding
  convention exists), and avoids inventing a new "unknown field" validation rule not asked
  for by any requirement.
- **Vehicle status**: added as a new schema column (`vehicles.status`), mirroring
  `customers.status` (`specs/customer-management/design.md` §3.1), since RF04 explicitly
  requires checking it and no other feature owns this column yet.
