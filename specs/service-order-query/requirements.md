# Requirements — Service Order Listing and Detail

Status: **Approved for implementation**
Feature folder: `internal/features/service-order/` (extends the existing package — same
`ServiceOrder` aggregate the opening/diagnosis-quote features already use, not a new
feature package; see `design.md` §1.1 for why).

## 1. Context

Source: Jira ticket "Consulta Administrativa de Ordens de Serviço" (RF11, RNF02, RNF04).

**Changed on 2026-10-12** (Fase 2 Tech Challenge, `SOAT - Fase 2 - Tech challenge.pdf` p.3,
"Listagem de ordens de serviço"): the Fase 2 brief mandates a specific default order and a
default exclusion for this same endpoint. BR3 and AC1 below are updated accordingly; see the
note under BR3 for the exact rule and for `CANCELED`'s inclusion, which the PDF itself does
not address and was resolved as an explicit team decision instead of assumed.

`internal/features/service-order/` currently only lets a caller *write* a service order
(`POST /api/v1/service-orders`, `POST .../{id}/diagnosis`, `PUT .../{id}/quote`) and read
back its own quote (`GET .../{id}/quote`). There is no way to list existing orders or see
one order's full picture (customer, vehicle, requested services, quote, status, history) —
this feature adds exactly that, as a read-only extension of the same aggregate.

## 2. User story

> As a workshop manager,
> I want to list and view the detail of service orders,
> so that I can follow the operation and locate a specific case.

## 3. Business rules

1. The listing is paginated (`page`/`pageSize`, same convention as `customer`, `vehicle`,
   and `product`: 1-based `page`, default `pageSize` 20, capped at 100).
2. The listing accepts the following optional filters, combinable, all narrowing the result
   set (AND, not OR): service order code, status, customer CPF/CNPJ, vehicle license plate,
   and a creation-date range.
3. ~~The listing's default order is by creation date, most recent first, with the order code
   (descending) as a stable tiebreaker.~~ **Changed on 2026-10-12** (Fase 2 Tech Challenge):
   when the caller does not supply a `status` filter, the listing (a) excludes `COMPLETED`,
   `DELIVERED`, and `CANCELED` orders (logical exclusion only — no row is deleted or
   otherwise altered) and (b) orders the remaining orders by status priority —
   `IN_PROGRESS` > `AWAITING_APPROVAL` > `IN_DIAGNOSIS` > `RECEIVED` — then oldest first
   within a status (`created_at` ascending), with order code (ascending) as a stable
   tiebreaker. When the caller *does* supply a `status` filter, both the exclusion and the
   status-priority order are moot (every returned row already shares one status) and the
   listing falls back to plain oldest-first (`created_at` ascending, `code` ascending) — an
   explicit `status=COMPLETED`/`DELIVERED`/`CANCELED` therefore still works and returns
   those orders; the PDF only asks to exclude terminal orders from "a listagem" (the
   default/dashboard view), not to make them unfilterable. `CANCELED`'s exclusion
   specifically is a team decision (not in the PDF's four-status priority list nor its
   exclusion list, which only names "finalizadas e entregues") — excluded from the default
   view by analogy with the other two terminal statuses.
4. The detail view (`GET .../{id}` or `GET .../code/{code}`) shows: customer, vehicle,
   status, notes, the services requested at opening, the current quote (products/services,
   quantities, and unit/total prices) when one has been composed, and the full status-change
   history.
5. Quote item values shown in the detail come from the quote's own stored snapshot
   (`quote_products.applied_*` / `quote_services.applied_*`), never from the current catalog
   price of the referenced product/service — this is already how `quote_products`/
   `quote_services` are written (`specs/service-order-diagnosis-quote/`), so listing/detail
   only has to read them as stored, never re-join the catalog for pricing.
6. A customer, vehicle, product, or service referenced by a service order/quote that has
   since become `INACTIVE` (or, for a service, simply changed) must still be shown in the
   order's detail and history — inactive status of a related record is never a reason to
   hide it from an existing order's read view. No query in this feature filters a *detail*
   read by any related record's `status`/`active` flag (listing's own filters, §3.2, are the
   only server-side narrowing, and they apply to the service order being searched for, not
   to its related records).
7. Every route added by this feature requires a valid JWT (RNF02) — this project has no
   role/permission model (`CLAUDE.md` §13), so "administrative user" (the ticket's phrasing)
   means any caller holding a valid token issued by `POST /api/v1/auth/login`, the same
   meaning it has for every other JWT-protected route in the project.
8. A service order that does not exist (unknown id or unknown code) returns `404`.

## 4. Scope

In scope:
- `GET /api/v1/service-orders` — paginated, filterable listing.
- `GET /api/v1/service-orders/{id}` — detail by technical id or by sequential code (§5).
- The read projections (list item shape, detail shape) these two endpoints return.
- OpenAPI documentation of both endpoints, including filters and pagination.

Out of scope (explicitly not implemented by this feature):
- Any write operation (create/update/cancel/delete a service order) — already covered, or
  not yet covered, by other specs.
- Report export, caching, or role/permission-scoped visibility beyond "valid JWT" (see BR7).
- A "diagnosis" entity separate from the order's `status` and its `service_order_history`
  entry — `docs/entities.md` has no such entity; "diagnosis" in the detail view is the
  order's current `status` plus its `diagnosis_started` history event, not a new field.
- Changing where `POST /api/v1/service-orders` sits on the auth allowlist (`CLAUDE.md` §1)
  — unrelated to this feature, and an explicitly open decision on its own.

## 5. Endpoints covered (contract details in `design.md`)

- `GET /api/v1/service-orders?code=&status=&document=&licensePlate=&createdFrom=&createdTo=&page=&pageSize=`
- `GET /api/v1/service-orders/{id}` — `{id}` accepts either the order's technical id (a
  UUID) or its sequential code (an integer); see `design.md` §1.2 for why this is one route
  rather than the two the ticket suggested (`.../{id}` and `.../code/{code}`).

Both require `Authorization: Bearer <token>` (RNF02).

## 6. Acceptance criteria

1. ~~`GET /api/v1/service-orders` returns a paginated envelope (`data`, `page`, `pageSize`,
   `total`, `totalPages`), most recent orders first, when called with no filters.~~
   **Changed on 2026-10-12**: called with no filters, it returns a paginated envelope
   ordered by status priority (`IN_PROGRESS` > `AWAITING_APPROVAL` > `IN_DIAGNOSIS` >
   `RECEIVED`), oldest first within a status, excluding `COMPLETED`, `DELIVERED`, and
   `CANCELED` orders (BR3).
2. `GET /api/v1/service-orders?code=<code>` returns only the order with that code (or an
   empty page if none matches).
3. `GET /api/v1/service-orders?status=<status>` returns only orders in that status.
4. `GET /api/v1/service-orders?document=<cpf-or-cnpj>` returns only orders whose customer
   has that document (accepts the same formatted/unformatted input `customer` already
   normalizes).
5. `GET /api/v1/service-orders?licensePlate=<plate>` returns only orders for that vehicle.
6. `GET /api/v1/service-orders?createdFrom=&createdTo=` returns only orders opened within
   that (inclusive) creation-date range.
7. Filters combine (AND) when more than one is supplied.
8. `GET /api/v1/service-orders/{id}`, called with either a UUID or a sequential code,
   returns: customer, vehicle, status, notes, requested services, the composed quote (with
   its items/quantities/snapshot prices) when one exists (`null`/absent otherwise), and the
   full history of status-change events, oldest first.
9. An unknown id or code returns `404`; a value that is neither a valid UUID nor a valid
   integer also returns `404` (it can never match a real order — see `design.md` §1.9).
10. Every route in this feature returns `401` without a valid bearer token.
11. `docs/openapi.yaml` documents both endpoints, their filters, and pagination.
12. **Added 2026-10-12.** `GET /api/v1/service-orders?status=COMPLETED` (and `=DELIVERED`,
    `=CANCELED`) still returns the matching orders — the default exclusion (BR3) only
    applies when `status` is not supplied.
13. **Added 2026-10-12.** Two orders in the same status, both present in an unfiltered
    listing, appear with the older one (earlier `created_at`) first.

## 7. Open questions / discoveries resolved before or during implementation

- **Route prefix**: the ticket's suggested Portuguese paths
  (`/api/v1/ordens-servico[...]`) were not used. Confirmed with the requester: reuse the
  existing `/api/v1/service-orders` prefix (`POST` create, `.../diagnosis`, `.../quote`
  already live there) instead of opening a second, parallel URL prefix for the same
  resource — also the convention `customer`/`vehicle`/`servicecatalog`/this same feature's
  own write routes already follow (`CLAUDE.md`/`specs/architecture.md` decision 8), even
  though one other feature (`product`, undocumented in `specs/architecture.md`) used
  Portuguese routes without a recorded rationale.
- **Filter parameter names/formats, sort order, response envelope shape, and pagination
  defaults**: not specified by the ticket beyond "paginated" and "predictable, preferably
  most-recent-first." Resolved by reusing this project's own existing, precedented
  conventions rather than inventing new ones — see `design.md` §1.3–§1.5 for the exact
  reasoning and precedent cited for each.
- **A separate `GET /api/v1/service-orders/code/{code}` route, as the ticket suggested, is
  not implementable under this prefix.** Discovered the same way `specs/architecture.md`
  decision 18 was discovered — by actually registering the routes on a real
  `*http.ServeMux`, not just running `go build`/`go vet`/`go test` — and resolved the same
  way: adjust the route, document the discovery here rather than silently coding around it.
  See `design.md` §1.2 for the exact conflict and why `GET /api/v1/service-orders/{id}` now
  accepts either an id or a code instead.
