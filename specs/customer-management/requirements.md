# Requirements — Customer Management

Status: **Approved for implementation**
Feature folder: `internal/features/customer/`

## 1. Context

This is the first business feature implemented in `automotive-workshop-api`. Today the
repository only has a `/health` endpoint and the domain/schema documentation in `docs/`
(see `docs/entities.md`, `docs/schema.sql`). No feature, persistence layer, or dependency
has been implemented yet.

## 2. User story

> As a front-desk attendant,
> I want to register and maintain customers,
> so that I can reliably identify them when opening a service order.

## 3. Business rules

1. A customer is an individual (CPF) or a company (CNPJ). Exactly one of the two document
   types applies to a given customer; the type is derived from the document itself.
2. The document (CPF or CNPJ) must be normalized (punctuation/spaces stripped, letters
   uppercased) before validation, persistence, or lookup. CPF is always purely numeric.
   CNPJ may be purely numeric (legacy format) or alphanumeric — Receita Federal's
   Instrução Normativa RFB nº 2.229/2024 introduced letters A–Z in a CNPJ's first 12
   characters starting July 2026; its last two characters (the check digits) are always
   numeric. Both forms must be accepted, since pre-existing numeric CNPJs are not reissued.
3. The document must pass structural validation, including verifying its check digits — not
   just length/regex. A document that fails check-digit validation must be rejected.
4. The document is unique across all customers, active or inactive. Attempting to create or
   update a customer with a document that already belongs to another customer must fail.
4.1. The e-mail, when provided, is also unique across all customers. This is a **pre-existing**
   database invariant (`ux_customers_email` in `docs/schema.sql`, present since the initial
   project skeleton, predating this feature) that this feature's write paths (create, update)
   must respect — a duplicate e-mail must be rejected, with an error distinct from a duplicate
   document, not silently reported as "document already belongs to another customer." E-mail
   remains optional; the uniqueness rule only applies when it is present (matching the
   database's partial unique index).
5. A customer starts, at creation, in status `ACTIVE`. No other initial status is allowed.
6. A customer can be moved from `ACTIVE` to `INACTIVE` (deactivation). This is a **logical**
   status change — the record is never physically deleted, and its history is preserved.
7. Deactivating a customer is not the same as un-deactivating one: there is no endpoint or
   implicit rule in this feature that moves a customer back from `INACTIVE` to `ACTIVE`. If
   reactivation becomes a requirement, it must be specified explicitly as a new capability.
8. Inactive customers remain fully queryable (by id, by document, in listings). Inactivity
   only affects future eligibility rules for other features (see §7 below) — it does not
   hide the record.
9. Updates to name, document, phone, and e-mail are partial: a field not sent in the update
   request must remain unchanged. When the document is part of an update, it goes through
   the same normalization → validation → uniqueness pipeline as on creation.

## 4. Scope

In scope for this feature:

- Customer domain model (individual/company, document, contact info, status).
- Create, retrieve (by id, by document), list (paginated), partially update, and logically
  deactivate a customer.
- CPF/CNPJ normalization and check-digit validation.
- Document uniqueness, enforced both by the application and by a database constraint.
- REST contract for the six endpoints below, with a consistent error format.
- OpenAPI documentation for the feature.
- Unit and integration tests covering the rules above.

Out of scope for this feature (see §7, "Future requirements," for how each is handled):

- Service Order (does not exist yet in the codebase).
- Authentication, authorization, JWT, login, users, roles, permissions, refresh tokens.
- Reactivating a previously deactivated customer.
- Any functionality not required to manage customers.

## 5. Endpoints covered (contract details in `design.md`)

```
POST   /api/v1/customers
GET    /api/v1/customers
GET    /api/v1/customers/{id}
GET    /api/v1/customers/document/{document}
PATCH  /api/v1/customers/{id}
DELETE /api/v1/customers/{id}   (logical deactivation, not physical delete)
```

> **Naming note**: the task that originated this feature was written in Portuguese and used
> `/api/v1/clientes`, `ATIVO`/`INATIVO`, etc. `CLAUDE.md` §8 establishes English as the
> project's domain language for every identifier except the already-existing
> `ServiceOrder.status` enum, which is a single, explicitly documented exception. To avoid
> silently introducing a second Portuguese enum/route family, this feature uses the English
> equivalents (`customers`, `ACTIVE`/`INACTIVE`, etc.) consistently with `docs/entities.md`
& `docs/schema.sql`. This is a deliberate, documented deviation from the literal task
> wording, not an invented requirement.

## 6. Acceptance criteria

```
[ ] Register an individual with a valid CPF
[ ] Register a company with a valid CNPJ
[ ] Normalize CPF/CNPJ before validating/persisting/looking up
[ ] Persist the normalized CPF/CNPJ (CPF digits only; CNPJ digits or uppercase letters)
[ ] Accept an alphanumeric CNPJ (post-July-2026 Receita Federal format), not just numeric
[ ] Reject a structurally invalid document (bad length or bad check digits)
[ ] Reject a duplicate document (create and update)
[ ] Reject a duplicate e-mail (create and update), with an error distinct from duplicate document
[ ] Retrieve a customer by id
[ ] Retrieve a customer by document
[ ] List customers
[ ] Paginate the listing
[ ] Update name
[ ] Update phone
[ ] Update e-mail
[ ] Update document (re-validated, re-normalized, uniqueness re-checked)
[ ] Guarantee uniqueness is enforced during update, not only at creation
[ ] Deactivate a customer
[ ] Never physically delete a customer record
[ ] Query an inactive customer (by id, by document, in listings)
[ ] Preserve history (row is updated in place, never removed)
[ ] Document the future Service Order eligibility rule (§7)
[ ] Document every endpoint in OpenAPI
[ ] Unit tests for CPF validation/normalization
[ ] Unit tests for CNPJ validation/normalization
[ ] Integration tests for the full CRUD flow
```

## 7. Future requirements (explicitly deferred, not implemented here)

### 7.1 Service Order eligibility

> Inactive customers must not be assignable to new Service Orders.

Service Order does not exist in the codebase yet. When it is specified and implemented, its
"open service order" use case must validate that the referenced customer's status is
`ACTIVE` before allowing the order to be created. This is recorded here as a **future
integration invariant** on the Service Order feature, not implemented by this feature. No
code in `internal/features/customer/` should anticipate or stub this check.

### 7.2 JWT / administrative authentication

> RNF02: administrative APIs must be protected by JWT.

No authentication/authorization infrastructure exists in this project yet (`CLAUDE.md` §13
marks this explicitly as "To be defined"). This feature must **not** implement its own
authentication, and must not add a security framework/library to satisfy this requirement
prematurely. The customer endpoints are implemented unauthenticated. A dedicated Security
feature, specified separately, will add JWT authentication/authorization as a cross-cutting
concern (most likely HTTP middleware in `internal/shared/`) applied on top of the existing
routes, without requiring changes to `internal/features/customer/`'s business logic.

## 8. Open questions resolved before implementation

The following ambiguities were resolved with the requester before writing `design.md`
(rather than assumed), per `CLAUDE.md` §12 and the SDD process in `specs/README.md`:

- **Database driver**: `pgx v5` (`pgxpool`) — chosen over `database/sql` + `lib/pq` (frozen
  maintenance mode) and over the pgx `database/sql` stdlib adapter.
- **Test dependency**: `testify` (`require`/`assert`) is added as the project's first
  external test dependency.
- **Integration test database**: tests connect to the existing docker-compose Postgres via
  `DATABASE_URL`; they skip (not fail) when that database is unreachable, rather than
  spinning up an isolated container (e.g. `testcontainers-go`).
