# service-catalog — Requirements

Source: Jira card "Manter catálogo de serviços e valores" (RF03), refined on 2026-08-18.

## Context and goal

Quotes (`Quote`) are composed of products and services. Today the `services` table exists in
[docs/schema.sql](../../docs/schema.sql) but no API exposes it: there is no way for the
workshop manager to register, consult, update, or retire a service. This feature introduces
the service catalog so quotes can later be composed from standardized, priced services.

## User story

> As a workshop manager,
> I want to maintain a catalog of services and prices,
> so that I can compose quotes in a standardized way.

## Related requirements

- **RF03** — Manage services.
- **RNF02** — JWT authentication.
- **RNF04** — Consistent REST API (shared error envelope, see `internal/shared/httpx`).
- **RNF10** — OpenAPI documentation. **Out of scope for this feature** by explicit decision
  (2026-08-18, same cut recorded in [specs/auth/requirements.md](../auth/requirements.md)):
  the project still has no OpenAPI artifact, and creating one is treated as a separate
  feature. Recorded here so the scope cut stays traceable.

## Functional requirements

- **FR1** — A manager can register a service with code, name, price, and optionally
  description and estimated time.
- **FR2** — A manager can list the registered services and tell active ones from inactive
  ones.
- **FR3** — A manager can retrieve a single service by its technical identifier.
- **FR4** — A manager can update a service's name, description, price, and estimated time.
- **FR5** — A manager can retire a service (deactivation), preserving its history.
- **FR6** — Every catalog operation requires an authenticated user (RNF02); no catalog
  route is public.

## Business rules

- **BR1** — Name is required. Code is unique.
- **BR2** — Code, when informed by the caller, must not collide with an existing service's
  code.
- **BR3** — Price is required on registration and can never be negative.
- **BR4** — Estimated time is optional; when informed, it must be greater than zero.
- **BR5** — Changing a service's price must not modify quotes already generated.
- **BR6** — An inactive service must not be included in new quotes.
- **BR7** — Deletion is logical (deactivation) whenever the service is used by a service
  order or a quote.

## Decisions taken during refinement (2026-08-18)

Approved by the project owner; each resolves an ambiguity in the original card:

- **D1 (BR1/BR2)** — `services.code` is currently `GENERATED ALWAYS AS IDENTITY`, which makes
  a duplicate code impossible and the card's 409 criterion unreachable. Decision: the code
  becomes **caller-supplied but optional** — when the request carries a code, that value is
  used and a collision is rejected; when it does not, the database generates the code as it
  does today.
- **D2 (BR7/FR5)** — Deletion is **always logical**: the delete operation deactivates the
  service, it never removes the row. This satisfies BR7 unconditionally (a service used by a
  quote or a service order keeps its row and its history) and keeps the operation
  predictable, instead of branching on whether the service happens to be referenced yet.
- **D3 (BR5/BR6)** — The `Quote` feature does not exist in the code yet, so neither rule can
  be enforced or tested end to end by this feature:
  - BR5 is already structurally guaranteed by the schema: `quote_services.applied_price`
    snapshots the price at the moment the service enters the quote, so a later catalog price
    change cannot reach an existing quote. No catalog code is needed for it.
  - BR6 is **deferred to the Quote feature**, which will reject inactive services when
    composing a quote. This feature only provides the `active` flag that rule will read.
  No speculative validation or helper is written now for a feature that does not exist.
- **D4 (RNF10)** — OpenAPI documentation is out of scope (see above).

## Acceptance criteria

- **AC1** — Registering a service with code, name, and price returns HTTP 201 with the
  created service.
- **AC2** — Registering a service with a code that already exists returns HTTP 409.
- **AC3** — A negative price is rejected with HTTP 400, on registration and on update.
- **AC4** — An estimated time that is not greater than zero is rejected with HTTP 400.
- **AC5** — The listing lets the caller tell active from inactive services.
- **AC6** — Description, price, and estimated time can be updated.
- **AC7** — Deactivation preserves the record and its history: the service is still
  retrievable, flagged as inactive.
- **AC8** — Every catalog route called without a valid token returns HTTP 401.
- **AC9** — Unit and integration tests cover the catalog.

Criteria from the original card **not** covered by this feature, per D3/D4:

- "Inactive service is not accepted in a new quote" — deferred to the Quote feature (BR6).
- "Price change does not affect quote items already created" — already guaranteed by the
  `quote_services.applied_price` snapshot in the schema; testable only once the Quote
  feature exists (BR5).
- "Endpoints documented in OpenAPI" — out of scope (D4).
