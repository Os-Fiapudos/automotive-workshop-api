# Requirements — Vehicle Management

Status: **Draft — pending requester validation**
Feature folder (proposed): `internal/features/vehicle/`

## 1. Context

This feature is requested via a prompt written in Portuguese (user story, RF02/RNF02-04,
endpoints `/api/v1/veiculos`, module sketch `Modules/Atendimento/Veiculos/{Cadastrar,
Consultar,Atualizar,Inativar}`). Two implemented features already exist in this codebase —
`auth` and `customer` (`specs/customer-management/`) — and Vehicle Management resembles
Customer Management closely: a registry entity, owned by (linked to) a Customer, with
create/retrieve/update/logical-deactivate use cases. `docs/entities.md` and
`docs/schema.sql` already document/define the `Vehicle` entity and the `vehicles` table
(`id`, `code`, `license_plate`, `brand`, `model`, `year`, `color`, `customer_id`,
`created_at`, `updated_at`) — no `status` column exists yet, and no Go feature implements it
(`specs/architecture.md` §6, "Other entities... schema exists but still has no Go
repository").

## 2. User story

> As a front-desk attendant,
> I want to register vehicles linked to customers,
> so that I can correctly associate them with service orders.

## 3. Resolved ambiguities (confirmed with the requester before writing this document)

The originating prompt conflicts with this project's already-established conventions on four
points. Each was resolved with the requester directly (per `CLAUDE.md` §17, "when there is
ambiguity, ask before implementing") rather than assumed:

1. **Route/domain language.** The prompt gives Portuguese paths (`/api/v1/veiculos`,
   `/api/v1/clientes/{clienteId}/veiculos`). `CLAUDE.md` §8 and
   `specs/customer-management/requirements.md` §5 already establish — and deliberately
   applied to a Portuguese-origin task — that every domain identifier in this codebase is
   English, with the single documented exception of `ServiceOrder.status`. **Confirmed
   decision: English routes**, `/api/v1/vehicles` and (originally)
   `/api/v1/customers/{customerId}/vehicles`, consistent with that precedent. This is the
   same category of deliberate, documented deviation `customer-management` already made, not
   a new invented requirement. **Implementation-time correction**: the customer-scoped listing
   actually shipped as `GET /api/v1/vehicles/customer/{customerId}` instead — Go's
   `http.ServeMux` panics at startup on `/api/v1/customers/{customerId}/vehicles`, since it is
   genuinely ambiguous against customer's own already-shipped
   `GET /api/v1/customers/document/{document}` (neither pattern is more specific than the
   other). See `design.md` §1.5 for the full account; this was a technical routing conflict
   discovered during implementation, not a scope change.
2. **`PATCH` scope.** The prompt's own acceptance checklist (§7 below) names exactly four
   updatable fields — brand, model, year, color — and business rule 7 ("owner change must not
   compromise existing service orders") never appears as an endpoint or checklist item.
   **Confirmed decision: `PATCH` updates only brand, model, year, and color.** License plate
   and the owning customer are immutable after creation in this feature; rule 7 is recorded
   as a future note (§7.1) rather than implemented, since there is no capability in scope that
   reassigns a vehicle's owner.
3. **License plate format.** RNF03 requires plate validation without naming a standard.
   Brazil has two plate formats in circulation — legacy (`AAA-9999`) and Mercosul
   (`AAA9A99`, mandatory since 2018; legacy plates remain valid until replaced), the latter
   already used throughout `docs/seed.sql`'s existing vehicle rows (e.g. `ABC1D23`).
   **Confirmed decision: accept both**, mirroring the CPF/CNPJ dual-format precedent in
   `internal/shared/document`.
4. **Manufacturing year range.** Business rules require a "valid format and range" without
   stating bounds. **Confirmed decision: 1950 to (current year + 1)** — excludes implausible/
   mistyped years while allowing next-year models sold in advance.

## 4. Business rules

1. A vehicle must be linked to a customer that exists **and** is currently `ACTIVE`
   (`specs/customer-management`'s `Customer.status`). Creation against a nonexistent customer
   or an `INACTIVE` one is rejected.
2. The license plate is normalized (uppercased, formatting characters stripped) before
   validation, persistence, or lookup.
3. The normalized plate must match either the legacy (`AAA9999`) or Mercosul (`AAA9A99`)
   structural format (§3.3). A plate that matches neither is rejected.
4. The plate is unique across all vehicles, active or inactive.
5. The manufacturing year must be an integer between 1950 and the current year + 1,
   inclusive (§3.4).
6. A vehicle starts, at creation, in status `ACTIVE`. No other initial status is allowed.
7. A vehicle can be moved from `ACTIVE` to `INACTIVE` (deactivation). This is a **logical**
   status change — the record is never physically deleted, and its history is preserved.
   There is no endpoint that reactivates a vehicle (mirrors
   `specs/customer-management/requirements.md` §3.7's "no implicit reactivation" rule).
8. Inactive vehicles remain fully queryable (by id, by plate, in listings, in a customer's
   vehicle list). Inactivity only affects future eligibility for new service orders (§7.1) —
   it does not hide the record.
9. `PATCH` is a partial update of brand, model, year, and color only (§3.2); a field not sent
   in the request body remains unchanged. License plate and the owning customer are
   immutable through this feature's endpoints.

## 5. Scope

In scope for this feature:

- Vehicle domain model (plate, brand, model, year, color, owning customer, status).
- Create, retrieve (by id, by plate), list (paginated, all vehicles or scoped to one
  customer), partially update (brand/model/year/color), and logically deactivate a vehicle.
- License plate normalization and structural validation (legacy + Mercosul).
- Plate uniqueness, enforced both by the application and by the existing database constraint
  (`ux_vehicles_license_plate`, already in `docs/schema.sql`).
- Validating, at creation, that the referenced customer exists and is `ACTIVE` — reusing the
  already-implemented `customer` feature's status, without importing its Go package directly
  (see `design.md` §1.3 for how).
- Adding the `status` field to the `Vehicle` domain model, `docs/entities.md`, and
  `docs/schema.sql` (does not exist yet — same treatment `customer-management` gave
  `Customer.status`/`documentType`).
- REST contract for the seven endpoints below (JWT-protected — see RNF02 below), reusing the
  project's `apierror` envelope (see `design.md` §1.4 for why).
- OpenAPI documentation for the feature.
- Unit and integration tests covering the rules above.

Out of scope for this feature (see §7.1 for how it's recorded instead):

- Service Order (does not exist yet in the codebase) and its "reject an inactive vehicle"
  eligibility rule.
- Reassigning a vehicle's owner (customer) after creation.
- Updating a vehicle's license plate after creation.
- Reactivating a previously deactivated vehicle.
- Any authorization/role model beyond "a valid JWT is required" (no roles exist in this
  project yet, per `CLAUDE.md` §13).
- Any functionality not required to manage vehicles.

## 6. Endpoints covered (contract details in `design.md`)

```
POST   /api/v1/vehicles
GET    /api/v1/vehicles
GET    /api/v1/vehicles/{id}
GET    /api/v1/vehicles/plate/{plate}
GET    /api/v1/vehicles/customer/{customerId}   (see §3.1's implementation-time correction)
PATCH  /api/v1/vehicles/{id}
DELETE /api/v1/vehicles/{id}   (logical deactivation, not physical delete)
```

All seven routes require a valid JWT (RNF02) — see `design.md` §1.5. This differs from
Customer Management, whose six routes are still deliberately unauthenticated
(`specs/customer-management/requirements.md` §7.2); this feature's own requirements
explicitly demand JWT ("Todas as rotas administrativas exigem JWT"), and `auth`'s
`middleware.RequireAuth` already exists and is reused as-is. This does **not** retroactively
wrap the Customer Management routes — that remains the separate, still-open decision recorded
in `CLAUDE.md` §17.

## 7. Acceptance criteria

Traceability to the originating prompt's checklist (its section 7), each item mapped to
where it's satisfied:

```
[ ] Register a vehicle only for a customer that exists and is ACTIVE           (BR1, design.md §3.3)
[ ] Reject a vehicle for a nonexistent customer (404)                          (BR1)
[ ] Reject a vehicle for an INACTIVE customer (409)                            (BR1)
[ ] Normalize the license plate before validating/persisting/looking it up     (BR2)
[ ] Reject a structurally invalid plate (neither legacy nor Mercosul)          (BR3)
[ ] Reject a duplicate plate (409)                                             (BR4)
[ ] Reject a manufacturing year outside 1950..currentYear+1                    (BR5)
[ ] Retrieve a vehicle by id                                                   (design.md §4.3)
[ ] Retrieve a vehicle by plate                                                (design.md §4.4)
[ ] List a customer's vehicles                                                 (design.md §4.5)
[ ] Update brand, model, year, and color                                       (BR9, design.md §4.6)
[ ] Deactivation preserves historical relationships (logical, not physical)    (BR7, design.md §4.7)
[ ] An inactive vehicle cannot be used in a new service order                  (§7.1 — future note)
[ ] Every administrative route requires JWT                                    (§6, design.md §1.5)
[ ] Tests: plate validation, active-customer linkage, and full CRUD            (design.md §6)
```

## 8. Future requirements (explicitly deferred, not implemented here)

### 7.1 Service Order eligibility

> An inactive vehicle must not be assignable to a new Service Order.

Service Order does not exist in the codebase yet (same situation as
`specs/customer-management/requirements.md` §7.1's customer-eligibility note). When it is
specified and implemented, its "open service order" use case must validate that the
referenced vehicle's status is `ACTIVE` before allowing the order to be created. This is
recorded here as a **future integration invariant** on the Service Order feature, not
implemented by this feature. No code in `internal/features/vehicle/` should anticipate or
stub this check.

### 7.2 Owner reassignment

> Business rule "owner change must not compromise existing service orders" (from the
> originating prompt) has no corresponding endpoint or acceptance-criteria item in this
> prompt (§3.2 above). If reassigning a vehicle's customer becomes a requirement, it must be
> specified explicitly as a new capability — including how it interacts with Service Order,
> which does not exist yet either.

## 9. Open questions resolved before implementation

Resolved directly with the requester (§3 above) rather than assumed, per `CLAUDE.md` §17 and
the SDD process in `specs/README.md`. No further open questions remain for `design.md`.
