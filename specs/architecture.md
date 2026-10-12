# Current architecture â€” automotive-workshop-api

This document describes the architecture **actually present in the code** of this
repository as of this date. It does not describe the desired/planned architecture beyond
what is already implemented or explicitly declared as a convention in the code/README
itself. Where the code does not allow something to be determined, it is marked as
**"To be defined"** instead of assumed.

This document must be updated whenever a new feature (via `specs/<feature>/`) changes the
real architecture of the system.

## 1. Architecture overview

The system is a single Go binary (`cmd/api`) that starts an HTTP server using only the
standard library's `net/http` (Go 1.22+'s method-aware `http.ServeMux` patterns), with no
third-party web framework/router. It exposes `GET /health` plus the implemented business
features:
- **auth** (`internal/features/auth/`, [specs/auth/](auth/)) â€” `POST /api/v1/auth/login`
  and the protected `GET /api/v1/auth/me`.
- **customer** (`internal/features/customer/`, [specs/customer-management/](customer-management/))
  â€” the six Customer Management endpoints under `/api/v1/customers`.
- **vehicle** (`internal/features/vehicle/`, [specs/vehicle-management/](vehicle-management/))
  â€” the seven Vehicle Management endpoints under `/api/v1/vehicles`, every one of them
  JWT-protected (unlike `customer`'s routes).
- **service-catalog** (`internal/features/service-catalog/`, Go package `servicecatalog`,
  [specs/service-catalog/](service-catalog/design.md))
  â€” the five protected service catalog endpoints: `POST|GET /api/v1/services` and
  `GET|PATCH|DELETE /api/v1/services/{id}`.

`internal/features/product/` and `internal/features/service-order/` are also present in the
code but are not yet described in this document; the sections below cover `auth`,
`customer`, `vehicle`, and `servicecatalog` only.

> **Addendum (`specs/service-order-tracking/`)**: a fifth implemented feature exists beyond
> the four described below â€” `service-order-tracking`
> (`internal/features/service-order-tracking/`, Go package `servicetracking`), the
> customer-facing `GET /api/v1/acompanhamento/{codigo}` (RF12). Unlike every feature
> described in this document, it is deliberately **not** wrapped in `middleware.RequireAuth`
> â€” it validates its own high-entropy tracking token (`internal/shared/trackingtoken`,
> sent via the `X-Tracking-Token` header) instead of the administrative JWT. It reuses
> `shared/apierror` (the envelope its closest sibling, `service-order`, uses) and reads
> `service_orders`/`vehicles`/`service_order_history` directly via SQL, the same
> cross-feature-table-read-without-a-Go-import pattern `service-order` already established
> for `vehicles`/`customers`. `service-order` itself gained one change for this: its
> `POST /api/v1/service-orders` now also auto-generates the tracking token in the same
> creation transaction, returned once as `trackingToken` in that response. See
> `specs/service-order-tracking/design.md` for the full design â€” not folded into Â§1/Â§2 below
> since, like `product`/`service-order`, this document's per-feature sections were not kept
> current for every implemented feature.

> **Addendum (`specs/service-order-query/`)**: `service-order` (the same package/feature
> `service-order-opening` and `service-order-diagnosis-quote` extend, not a new one) gained
> two read-only, `requireAuth`-wrapped routes: `GET /api/v1/service-orders` (paginated,
> filterable listing) and `GET /api/v1/service-orders/{id}` (full detail â€” `{id}` accepts
> either the order's UUID or its sequential code, since Go 1.22's `http.ServeMux` cannot
> host a separate literal `.../code/{code}` route alongside the pre-existing `{id}/quote`/
> `{id}/diagnosis` routes at the same path depth; discovered the same way decision 18 below
> was, by actually registering the routes, not just `go build`/`go vet`/`go test`). See
> `specs/service-order-query/design.md` for the full design, including why the ticket's
> two-route suggestion had to become one.

> **Addendum (`specs/service-order-execution/`)**: `service-order` gained four more
> `requireAuth`-wrapped routes: `POST /api/v1/service-orders/{id}/executions` (start a
> service execution), `POST /api/v1/service-orders/{id}/executions/{executionId}/finish`
> (finish one), `POST /api/v1/service-orders/{id}/finalize` (`IN_PROGRESS` â†’
> `COMPLETED`), and `POST /api/v1/service-orders/{id}/deliver` (`COMPLETED` â†’
> `DELIVERED`). It also implements `ServiceExecution` â€” the Go name for the
> `AuditServices` entity `docs/entities.md` already documented but no feature had built
> yet â€” restructuring its table from a start/end event log to one row per execution with
> its own `started_at`/`ended_at` columns (`docs/schema.sql`), needed to give the finish
> endpoint a stable execution id to act on. One caveat carried over from planning: no code
> anywhere implements the `AWAITING_APPROVAL` â†’ `IN_PROGRESS` transition (quote
> approval) â€” this feature treats an order already being `IN_PROGRESS` as an external
> precondition, so its endpoints cannot be exercised end-to-end from a freshly opened order
> through the public API alone yet. See `specs/service-order-execution/design.md` for the
> full design.

> **Addendum (`specs/service-order-quote-decision/`)**: `service-order` gained three more
> routes: `POST /api/v1/service-orders/{id}/quote/send` (`requireAuth`-wrapped, an attendant
> action â€” `IN_DIAGNOSIS` â†’ `AWAITING_APPROVAL`), and the customer-facing
> `POST /api/v1/acompanhamento/{codigo}/orcamento/aprovar` /`.../reprovar` (never
> `requireAuth`-wrapped, authenticated via the same `X-Tracking-Token` mechanism
> `service-order-tracking` established â€” reusing `internal/shared/trackingtoken` and reading
> `service_order_tracking_tokens` directly via SQL, the same no-cross-feature-Go-import
> pattern that table's owning feature already set). This is the feature that finally fills
> the `AWAITING_APPROVAL â†’ IN_PROGRESS` gap `specs/service-order-execution/` flagged as an
> external precondition (approval), and adds a matching `AWAITING_APPROVAL â†’ CANCELED`
> branch for rejection â€” a seventh `ServiceOrder.status` value beyond the six
> `docs/entities.md` originally documented, added by explicit decision (not the source
> ticket's own, which only recommended it) since a rejected quote can never be altered and
> the order would otherwise have no way to leave `AWAITING_APPROVAL`. It also changes
> already-shipped behavior: composing a quote (`PUT /api/v1/service-orders/{id}/quote`) no
> longer transitions the order by itself â€” only the new send step does â€” see
> `specs/service-order-quote-decision/design.md` Â§1.2 for the erratum this introduces into
> `specs/service-order-diagnosis-quote/design.md` Â§1.6. See
> `specs/service-order-quote-decision/design.md` for the full design.

> **Addendum (`specs/service-order-metrics/`)**: `service-order` gained one more
> `requireAuth`-wrapped route, `GET /api/v1/service-orders/metrics/average-execution-time`,
> a read-only aggregate over `specs/service-order-execution/`'s `audit_services`/
> `ServiceExecution` data â€” average duration in minutes, grouped by service, over completed
> executions only (`ended_at IS NOT NULL`; an in-progress execution is excluded, not
> counted as zero). Optional `serviceId`/`startDate`/`endDate` filters, no pagination (the
> result is bounded by the number of distinct services with a qualifying execution, not by
> execution volume). See `specs/service-order-metrics/design.md` for the full design.

> **Addendum (`specs/service-order-stock-usage/`)**: `service-order` gained three more
> `requireAuth`-wrapped routes: `POST /api/v1/service-orders/{id}/stock-movements` (deduct
> one or more parts/supplies from stock against an `IN_PROGRESS` order, all-or-nothing per
> request), `GET /api/v1/service-orders/{id}/stock-movements` (list the movements recorded
> against an order), and `POST .../stock-movements/{movementId}/reversal` (undo a previously
> registered deduction, restoring the quantity and linking back to the original movement).
> This is the first feature to persist to `docs/schema.sql`'s new `stock_movements` table â€”
> a ledger **shared** with `internal/features/product/`'s own manual stock adjustments
> (`POST /api/v1/products/{id}/stock/adjustments`), distinguished by a nullable
> `service_order_id`, rather than two separate "stock movement" concepts in the same domain.
> That product feature's `StockMovement` domain type and `POST .../stock/adjustments` endpoint
> already existed but were a stub before this change â€” `AdjustStock` only updated
> `products.current_stock`, and `GET /produtos/{id}/movements` always returned an empty
> array; this feature's schema work fixed that as a small, explicitly-scoped byproduct (its
> `AdjustStock` now writes to `stock_movements` too, and `listMovements` reads it back), not
> a broader refactor of `product`. As with every other cross-feature table read/write in this
> codebase, `service-order` and `product` each write to `stock_movements` through their own
> SQL â€” neither imports the other's Go package (CLAUDE.md Â§9.2). See
> `specs/service-order-stock-usage/design.md` for the full design.

The folder organization follows the **cmd/ + internal/** pattern, with **vertical slice
(organization by feature)** as the adopted convention: each business feature gathers
handler, service, repository, and model in a single package under
`internal/features/<feature>/`. `internal/features/auth/`, `internal/features/customer/`,
`internal/features/vehicle/`, and `internal/features/service-catalog/` all implement this end
to end (`customer` and `vehicle` organized internally by responsibility file â€” `model.go`,
`dto.go`, `repository.go`, `service.go`, `handler.go`, `errors.go`, plus `vehicle`'s own
`plate.go` and both features' `httpsupport.go` â€” rather than by use case; see
`specs/customer-management/design.md` Â§1.1 for the rationale, reused as-is by
`specs/vehicle-management/design.md` Â§1.1).
`internal/features/user/` remains an empty placeholder (`doc.go` only) â€” note this is
unrelated to auth's `users` database table; it's a distinct, not-yet-specified future
feature.

`internal/shared/` holds cross-cutting code from all four features: a single shared
Postgres connection pool (`shared/database`, first introduced by auth, now used by every
feature's repository), JWT issuing/verification (`shared/token`) and the authentication
middleware (`shared/middleware`) from auth â€” reused unchanged by `vehicle` and
`servicecatalog` for their own JWT-protected routes â€” and CPF/CNPJ validation
(`shared/document`) and the environment config loader (`shared/config`) from Customer
Management. `vehicle` deliberately does **not** add a third shared package for its own
license-plate validation (`vehicle/plate.go` stays feature-local â€” see
`specs/vehicle-management/design.md` Â§1.2): unlike CPF/CNPJ, a license plate isn't a
cross-feature concept, so it doesn't meet the "genuinely generic" bar `CLAUDE.md` Â§9.3 sets
for `shared/`. **Both `auth` and `customer` each introduced their own JSON error-envelope
package** (`shared/httpx` from auth, `shared/apierror` from Customer Management) â€” this is
flagged, not resolved, in Â§8 below; it is a real duplication surfaced by merging the two
branches, not a design decision to imitate. `vehicle` and `servicecatalog` both reuse
`shared/apierror` (the feature each most resembles structurally), not a third envelope;
`servicecatalog` also reuses `httpx.JSON` as its success-response writer.

Communication is real across all four features: `main.go` builds the shared pool and JWT
manager, then wires `auth.NewHandler(...)` (routes: login public, `/me` behind
`middleware.RequireAuth`), `customer.RegisterRoutes(...)` (all six routes still public â€” see
Â§10, decision 16), `vehicle.RegisterRoutes(...)` (all seven routes wrapped in the same
`requireAuth` middleware auth already built â€” see Â§10, decision 17), and the five
`servicecatalog` routes (also wrapped in `requireAuth`) onto one `*http.ServeMux`. `vehicle`
needs to check, at vehicle creation, that a referenced customer exists and is `ACTIVE` â€” it
does this through a small `CustomerLookup` interface it declares itself (mirroring how
`auth.Service` depends on `UserFinder`), implemented by an adapter `main.go` builds around
the already-constructed `*customer.CustomerService` â€” so `vehicle` never imports
`internal/features/customer` directly, preserving "no feature imports another feature" (Â§5
below) even though this is the first case where one feature's business logic genuinely
depends on another's current state.

## 2. Main components

| Component | Path | State |
| --- | --- | --- |
| HTTP entrypoint | `cmd/api/main.go` | Implemented â€” loads config (`DATABASE_URL`, `JWT_SECRET`, `JWT_TTL`, `PORT`), opens the shared Postgres pool, wires `auth`, `customer`, `vehicle`, and `servicecatalog`, registers `/health`, the public/protected auth routes, the customer routes, the vehicle routes, and the five catalog routes, starts the server. |
| `auth` feature | `internal/features/auth/` | Implemented â€” `handler.go` (HTTP), `service.go` (login/lookup logic), `repository.go` (pgx queries), `model.go` (`User`). Unit-tested (`handler_test.go`, `service_test.go`). See `specs/auth/`. |
| `customer` feature | `internal/features/customer/` | Implemented â€” model, DTOs, Postgres repository, service, HTTP handlers for all 6 endpoints. See `specs/customer-management/`. |
| `vehicle` feature | `internal/features/vehicle/` | Implemented â€” model, plate validation, DTOs, Postgres repository, service, HTTP handlers for all 7 endpoints, every route JWT-protected. See `specs/vehicle-management/`. |
| `servicecatalog` feature | `internal/features/service-catalog/` | Implemented â€” `handler.go` (five REST endpoints), `service.go` (`Catalog`: catalog business rules), `repository.go` (pgx queries over `services`), `model.go` (`Service`). Unit-tested (`handler_test.go`, `service_test.go`). See `specs/service-catalog/`. |
| `user` feature | `internal/features/user/` | Not implemented â€” only `doc.go` declaring the package (placeholder for a future feature; unrelated to auth's `users` table). |
| `features` package (root) | `internal/features/doc.go` | Not implemented â€” only a package comment. |
| Shared: `database` | `internal/shared/database/` | Implemented â€” `NewPool` builds and pings a `pgxpool.Pool` from a `postgres://` URL. Used by every feature's repositories. (Consolidated from two independent implementations, `NewPool` and `Connect`, that each branch introduced â€” see Â§10.) |
| Shared: `token` | `internal/shared/token/` | Implemented â€” `Manager` issues and verifies HS256 JWTs (`golang-jwt/jwt/v5`). Unit-tested (`token_test.go`), including alg-confusion regression coverage. |
| Shared: `middleware` | `internal/shared/middleware/` | Implemented â€” `RequireAuth` extracts and verifies the `Authorization: Bearer` header, injects the user id into the request context, or responds 401. Unit-tested (`auth_test.go`). |
| Shared: `httpx` | `internal/shared/httpx/` | Implemented â€” `JSON`/`Error` helpers producing `{"error":{"code","message"}}`. `Error` is used by `auth` and `middleware`; `JSON` is also the success-response writer of `servicecatalog`. Unit-tested (`respond_test.go`). See Â§8 re: overlap with `apierror`. |
| Shared: `apierror` | `internal/shared/apierror/` | Implemented â€” the JSON error envelope and HTTP status mapping used by `customer`, `vehicle`, and `servicecatalog` (`{"error":{"code","message","details"?}}`). See Â§8 re: overlap with `httpx`. |
| Shared: `document` | `internal/shared/document/` | Implemented â€” CPF/CNPJ normalize/detect-type/validate (check-digit algorithm, no third-party library), including the alphanumeric CNPJ format. |
| Shared: `config` | `internal/shared/config/` | Implemented â€” reads `DATABASE_URL`, `JWT_SECRET`, `JWT_TTL`, `PORT` from the environment. |
| Handler/integration tests | `internal/handlers_test/` | Implemented â€” `auth_test.go`, `customer_test.go`, `vehicle_test.go`, and `service_catalog_test.go`, each driving real HTTP against a real Postgres, each independently skipping (not failing) without `DATABASE_URL`/a reachable database. |
| Database schema | `docs/schema.sql` | Implemented as plain SQL; consumed by every feature's repository (`users` table for auth, `customers` table â€” with its `document_type`/`status` columns/enums â€” for Customer Management, `vehicles` table â€” with its new `status` column/enum â€” for Vehicle Management, `services` â€” with the `active` flag and `GENERATED BY DEFAULT` code added by the catalog â€” for the service catalog). |
| Sample data | `docs/seed.sql` | Implemented as plain SQL; applied manually via `psql`. Includes one seeded administrative user (bcrypt-hashed via pgcrypto `crypt()`), four sample customers, five sample vehicles (one `INACTIVE`, owned by the one `INACTIVE` customer), and six sample services (one deliberately inactive). |
| Domain model | `docs/entities.md` | Domain documentation for all entities, now including `User` and the `Service.active` field. `Customer`, `Vehicle`, `User`, and `Service` are the only entities with a corresponding Go implementation. |
| API documentation | `docs/openapi.yaml` | Implemented for the Customer Management and Vehicle Management endpoints (schemas, pagination, error envelope, `bearerAuth` security scheme for Vehicle Management). The auth and service catalog endpoints are not documented here â€” `specs/auth/requirements.md` and `specs/service-catalog/requirements.md` both scoped RNF10 out as a separate future feature. |
| Local environment | `docker-compose.yml`, `Dockerfile` | Implemented â€” orchestrates `db` (Postgres), `adminer`, and `api`; `api` now requires `JWT_SECRET` (fails fast via compose variable substitution if unset). |
| CI | `.github/workflows/ci.yml` | Implemented â€” runs `go build ./...`, `go vet ./...`, `go test ./...`. |

## 3. Responsibilities

- **`cmd/api/main.go`**: the process's single entrypoint. Loads config
  (`internal/shared/config`), opens the shared Postgres pool (`internal/shared/database`),
  builds the JWT token manager and both features' repositories/services, builds one
  `*http.ServeMux`, registers `/health` inline, registers auth's public/protected routes,
  and delegates every `/api/v1/customers*` route to `customer.RegisterRoutes`. It stays
  thin â€” no business logic, request parsing, or data access lives in `main.go` itself, per
  `CLAUDE.md` Â§9.4.
- **`internal/features/auth/`**:
  - `handler.go` â€” decodes/validates HTTP input, translates service errors to the HTTP
    error envelope (`httpx`), never logs credentials or tokens.
  - `service.go` â€” `Login` (credential check + token issuance) and `UserByID` (identity
    lookup for `/me`); depends only on the `UserFinder`/`TokenIssuer` interfaces it
    declares, not on concrete `shared` types, so it stays unit-testable with fakes.
  - `repository.go` â€” parameterized `pgx` queries against `users`; maps "no rows" to
    `ErrUserNotFound`.
  - `model.go` â€” the `User` struct mirroring the `users` table / `docs/entities.md`.
- **`internal/features/customer/`**: (see `specs/customer-management/design.md` Â§1.1 for
  the full rationale for this file layout)
  - `handler.go` â€” HTTP layer: request parsing/validation, DTO â‡„ domain conversion, status
    code mapping (`apierror`). Depends on `service.go`.
  - `service.go` â€” one method per use case, orchestrates domain + repository. Depends on
    the `CustomerRepository` interface (not the concrete Postgres type).
  - `repository.go` â€” the `CustomerRepository` interface and its `pgx`-backed
    implementation. Depends on `model.go` and `internal/shared/document`.
  - `model.go` â€” the `Customer` aggregate and its invariants (always starts `ACTIVE`,
    document only settable through validated construction, no `Activate` method).
  - `dto.go` â€” HTTP request/response shapes, independent of the domain type.
- **`internal/features/vehicle/`**: (see `specs/vehicle-management/design.md` Â§1.1 for the
  full rationale â€” same file layout as `customer`, plus `plate.go`)
  - `handler.go`/`httpsupport.go` â€” HTTP layer: request parsing/validation, DTO â‡„ domain
    conversion, status code mapping (`apierror`, reused from `customer`, not a third
    envelope). Every route is wrapped in the `requireAuth` middleware `main.go` passes in
    (RNF02). Depends on `service.go`.
  - `service.go` â€” one method per use case, orchestrates domain + repository +
    `CustomerLookup`. Depends on the `VehicleRepository` interface (not the concrete
    Postgres type) and the `CustomerLookup` interface it declares itself (satisfied by an
    adapter `main.go` builds around `*customer.CustomerService` â€” see Â§1 above).
  - `repository.go` â€” the `VehicleRepository` interface and its `pgx`-backed implementation.
    Depends only on `model.go` â€” no dependency on `internal/shared/document` or any
    `customer` type.
  - `model.go` â€” the `Vehicle` aggregate and its invariants (always starts `ACTIVE`, plate
    only settable through validated construction, year re-validated on every update, no
    `Activate` method).
  - `plate.go` â€” license-plate `Normalize`/`Validate` (legacy + Mercosul formats), feature-
    local rather than `internal/shared/` (Â§1 above).
  - `dto.go` â€” HTTP request/response shapes, independent of the domain type; the update
    request type has no field for license plate or customer id â€” both are immutable after
    creation, enforced by the type itself, not just handler logic.
- **`internal/features/service-catalog/`**: same layer split as `auth`.
  - `handler.go` â€” decodes/validates HTTP input (JSON body, `{id}` UUID, `active` query
    param), maps business errors to 400/404/409/500 through `httpx`, never returns an
    internal error text.
  - `service.go` â€” `Catalog`, the catalog's business rules (required name, non-negative
    price, positive estimated time and code, non-empty update); depends only on the `Store`
    interface it declares, so it is unit-testable with a fake.
  - `repository.go` â€” parameterized `pgx` queries against `services`, including the
    caller-supplied-or-generated `code` on insert and the `23505` â†’ `ErrCodeAlreadyExists`
    mapping.
  - `model.go` â€” the `Service` struct mirroring the `services` table / `docs/entities.md`.
- **`internal/features/user/`**: intended responsibility unchanged (folder convention +
  package comment) â€” still no concrete implementation.
- **`internal/shared/`**: genuinely cross-cutting code only (`CLAUDE.md` Â§9.3), each
  subpackage imported only by the feature(s) that need it and by `main.go`: `database`
  (every feature), `token`/`middleware` (auth, plus `middleware` for the vehicle and catalog
  routes), `httpx` (auth's error envelope, plus its `JSON` writer used by the service
  catalog's success responses),
  `document`/`config`/`apierror` (customer's CPF/CNPJ validation, config loading, and error
  envelope respectively â€” `apierror` also reused by `vehicle` and `servicecatalog`).
- Responsibility split within a feature (handler = HTTP concerns and error mapping, service
  = business rules against interfaces/repository, repository = SQL, model = data shape) is
  now observable in the implemented features and is expected of future ones too.

## 4. Flow of the main operations

```
GET /health â†’ inline handler in main.go â†’ json.Encode({"status":"ok"}) â†’ HTTP response

POST /api/v1/auth/login
  â†’ auth.Handler.Login: decode/validate JSON body (400 on malformed/missing fields)
  â†’ auth.Service.Login(email, password)
      â†’ Repository.FindByEmail (pgx, parameterized query on users)
      â†’ bcrypt.CompareHashAndPassword (unknown email OR wrong password â†’ single
        ErrInvalidCredentials, so the client cannot distinguish them â€” BR4)
      â†’ token.Manager.Generate (HS256 JWT, sub=user id, exp=now+TTL)
  â†’ 200 {"access_token", "token_type", "expires_in"}  |  401 generic envelope on
    ErrInvalidCredentials  |  500 on unexpected error (DB/signing failure)

GET /api/v1/auth/me
  â†’ middleware.RequireAuth: extract "Authorization: Bearer <token>", token.Manager.Verify
    (rejects missing header, bad signature, wrong alg, and expired tokens) â†’ 401 on failure,
    otherwise injects the user id into the request context
  â†’ auth.Handler.Me â†’ auth.Service.UserByID â†’ Repository.FindByID
  â†’ 200 {"id","code","name","email"} (no password hash)  |  401 if the user no longer
    exists  |  500 on unexpected error

POST/GET/PATCH/DELETE /api/v1/customers... (currently unauthenticated â€” see Â§10 decision 16)
  â†’ customer.handler (parses/validates request, maps errors to apierror's envelope)
  â†’ customer.CustomerService (business rules: starts ACTIVE, no reactivation, partial update, ...)
  â†’ customer.CustomerRepository â†’ pgx â†’ Postgres `customers` table
  â†’ customer.handler (DTO response) â†’ HTTP response

POST/GET/PATCH/DELETE /api/v1/vehicles... (every route requires a valid JWT â€” Â§10 decision 17)
  â†’ middleware.RequireAuth (same check as GET /api/v1/auth/me) â†’ 401 on missing/invalid/
    expired token, otherwise proceeds
  â†’ vehicle.handler (parses/validates request, maps errors to apierror's envelope)
  â†’ vehicle.VehicleService
      â†’ on Create: CustomerLookup.IsActiveCustomer (â†’ 404 CUSTOMER_NOT_FOUND if the
        customer doesn't exist, 409 CUSTOMER_INACTIVE if it exists but isn't ACTIVE) â†’
        NewVehicle (normalizes/validates the plate, validates the year range) â†’
        ExistsByPlate pre-check (â†’ 409 DUPLICATE_LICENSE_PLATE) â†’ Create
      â†’ other use cases: business rules (starts ACTIVE, no reactivation, PATCH limited to
        brand/model/year/color, plate/customerId immutable, ...)
  â†’ vehicle.VehicleRepository â†’ pgx â†’ Postgres `vehicles` table (unique-violation on
    `ux_vehicles_license_plate` also mapped to DUPLICATE_LICENSE_PLATE, catching a
    concurrent-request race the pre-check alone can't)
  â†’ vehicle.handler (DTO response) â†’ HTTP response

POST /api/v1/services  (and GET/PATCH/DELETE on the same prefix)
  â†’ middleware.RequireAuth (401 when the token is missing/invalid/expired)
  â†’ servicecatalog.Handler: decode body / validate {id} as UUID / parse ?active
  â†’ servicecatalog.Catalog: business rules (name required, price >= 0, estimated time > 0,
    code > 0 when supplied, at least one field on update)
  â†’ Repository: parameterized SQL on services; unique violation (23505) â†’
    ErrCodeAlreadyExists, no rows â†’ ErrServiceNotFound
  â†’ 201/200/204 on success  |  400 INVALID_REQUEST  |  404 SERVICE_NOT_FOUND  |
    409 CODE_ALREADY_EXISTS  |  500 generic envelope
```

DELETE on the catalog is a **logical deletion**: it sets `services.active = false` and never
removes the row, so history stays intact.

No other feature's operation flow is implemented here (product, service orders, quotes,
etc.). `docs/entities.md` describes the **data model** of these entities and a service order
status flow
(`RECEIVED â†’ IN_DIAGNOSIS â†’ AWAITING_APPROVAL â†’ IN_PROGRESS â†’ COMPLETED â†’ DELIVERED`),
but that remains domain documentation only â€” no Go logic implements it yet. **To be
defined** once those features are specified and implemented. Note the documented **future**
invariants (not yet implemented, since Service Order does not exist): opening a service
order must reject an `INACTIVE` customer (see `specs/customer-management/requirements.md`
Â§7.1) or an `INACTIVE` vehicle (see `specs/vehicle-management/requirements.md` Â§7.1).

## 5. Communication between components

Both implemented features follow the convention declared in [CLAUDE.md](../CLAUDE.md) Â§9:

- **Within a feature**: `handler.go` â†’ `service.go` â†’ `repository.go`. `auth`'s `Service`
  depends on the `UserFinder`/`TokenIssuer` interfaces it defines itself (not on
  `*Repository`/`*token.Manager` directly), which is what lets its tests use fakes instead
  of a real database/signer. `customer`'s `service.go` similarly depends only on the
  `CustomerRepository` interface, not the concrete Postgres type. `vehicle`'s `service.go`
  depends on both the `VehicleRepository` interface and the `CustomerLookup` interface (see
  below), never a concrete type, and `servicecatalog`'s `Catalog` depends only on the
  `Store` interface it declares.
- **Between a feature and shared code**: `auth` imports
  `internal/shared/{database,token,httpx,middleware}`; `customer` imports
  `internal/shared/{database,document,config,apierror}`; `vehicle` imports
  `internal/shared/{database,apierror}` only â€” no `middleware` import (the `requireAuth`
  middleware value is passed into `vehicle.RegisterRoutes` by `main.go` instead, so
  `vehicle` never needs to import `internal/shared/middleware` or hold a `token.Manager`
  itself) and no new shared package for plate validation (Â§1 above); `servicecatalog`
  imports `internal/shared/httpx` (and is likewise wrapped in `middleware` from `main.go`
  rather than importing it directly). `cmd/api/main.go` is the only place that imports every
  feature and wires their concrete dependencies together.
- **Between features**: no feature imports another feature's package directly â€” `auth`,
  `customer`, `vehicle`, and `servicecatalog` are fully independent Go packages of each
  other. `auth`, `customer`, and `servicecatalog` share nothing but
  `internal/shared/database`'s pool. `vehicle` is the first feature whose business logic
  genuinely depends on another feature's current state (BR1: a vehicle's customer must
  exist and be `ACTIVE`) â€” it resolves this via a `CustomerLookup` interface it declares
  itself (mirroring `auth.Service`'s `UserFinder` pattern), satisfied by a small adapter
  `main.go` builds around the already-constructed `*customer.CustomerService`.
  `internal/features/vehicle/` itself never imports `internal/features/customer/` â€” only
  `main.go`, which already imports every feature as the composition root, does.

## 6. Persistence

Implemented via `github.com/jackc/pgx/v5` (`pgxpool.Pool`), no ORM/query builder, one shared
pool for the whole process:

- **Driver**: `pgx/v5`, pinned in `go.mod` at `v5.7.4` (the newest release whose own `go`
  directive still allows Go 1.22 â€” see `specs/customer-management/design.md` Â§0). Plain
  parameterized SQL throughout.
- **Connection**: `internal/shared/database.NewPool(ctx, url)` opens a `pgxpool.Pool` and
  validates it with a `Ping`. Called once from `cmd/api/main.go` with `DATABASE_URL`; the
  pool is closed via `defer` on shutdown and shared by both features' repositories.
- **Schema**: [docs/schema.sql](../docs/schema.sql) defines all tables, applied
  automatically by the `db` container only on initial Docker volume creation
  (`docker-entrypoint-initdb.d`), not by application code. No migration tool is used to
  evolve the schema after initial creation â€” **to be defined**, see `CLAUDE.md` Â§14.
  - `users` (auth): `id UUID`, `code BIGINT IDENTITY`, `name`, `email UNIQUE`,
    `password_hash`, `created_at`/`updated_at`.
  - `customers` (Customer Management): the pre-existing table plus two columns added by
    this feature, `document_type customer_document_type` and `status customer_status`
    (two new enums), directly in the `CREATE TABLE` â€” see
    `specs/customer-management/design.md` Â§3.1.
  - `vehicles` (Vehicle Management): the pre-existing table plus one column added by this
    feature, `status vehicle_status` (one new enum, mirroring `customer_status`), directly
    in the `CREATE TABLE` â€” see `specs/vehicle-management/design.md` Â§3.1. The pre-existing
    `customer_id UUID NOT NULL REFERENCES customers (id) ON DELETE RESTRICT` foreign key
    (present before this feature) guarantees the referenced customer *exists*; it cannot
    express "and is `ACTIVE`" (Postgres FKs don't see other columns) â€” that half of BR1 is
    the application-level `CustomerLookup` check (Â§1 above).
  - `services` (service catalog): the pre-existing table plus the `active BOOLEAN NOT NULL
    DEFAULT TRUE` column and `code` switched to `GENERATED BY DEFAULT AS IDENTITY`, so a
    registration may supply its own code â€” see `specs/service-catalog/design.md` Â§2.
- **Repository pattern**: one interface + implementation per feature (not a generic
  repository). `auth`'s `Repository` maps `pgx.ErrNoRows` to `ErrUserNotFound`.
  `customer`'s `CustomerRepository` maps `pgx.ErrNoRows` to `ErrNotFound`, and inspects
  `pgconn.PgError.ConstraintName` on a `23505` unique-violation to distinguish a duplicate
  `document` (`ux_customers_document`) from a duplicate `email`
  (`ux_customers_email` â€” a pre-existing invariant predating the feature, see
  `specs/customer-management/requirements.md` Â§3.4.1) rather than assuming it's always the
  document. `vehicle`'s `VehicleRepository` maps `pgx.ErrNoRows` to `ErrNotFound` and a
  `23505` violation on `ux_vehicles_license_plate` (a pre-existing index, predating this
  feature) to `ErrDuplicatePlate`, same two-layer defense (application pre-check +
  database-constraint mapping) as `customer`'s document uniqueness. `servicecatalog`'s
  `Repository` maps `pgx.ErrNoRows` to `ErrServiceNotFound` and a `23505` unique-violation to
  `ErrCodeAlreadyExists`.
- **Seed**: [docs/seed.sql](../docs/seed.sql) populates sample data via manual `psql`: one
  administrative user (`admin@workshop.local`) with a bcrypt hash produced by pgcrypto's
  `crypt()` at insert time (plaintext dev-only password documented only in the seed file's
  SQL comment, never in Go code or logs), four sample customers with normalized CPF/CNPJ
  documents, five sample vehicles with normalized Mercosul-format plates (one `INACTIVE`,
  owned by the one `INACTIVE` sample customer â€” illustrating that inactivating a customer
  doesn't retroactively touch its pre-existing vehicles' own status), and six sample
  services (one deliberately inactive, so the catalog listing has both states).
- **Other entities** (`products`, `service_orders`, `quotes`,
  `service_order_history`, `audit_services`): schema exists in
  [docs/schema.sql](../docs/schema.sql) but still has no Go repository.

## 7. External integrations

No external integration (third-party APIs, message queues, e-mail/SMS services, payment
gateways, etc.) is present in the code or in `go.mod`. `docs/entities.md` mentions e-mail
notification as the *purpose* of the Customer's `email` field ("used for notifications and
communication"), but no notification-sending mechanism is implemented. **To be defined**
should any external integration be specified in the future.

## 8. Error handling

**Two error-envelope implementations currently coexist** â€” this is the most significant
open item this document surfaces, not a design decision to preserve:

- `internal/shared/httpx` (`JSON`/`Error`), used by `auth` and `middleware.RequireAuth`:
  `{"error": {"code", "message"}}`. Decided in `specs/auth/design.md` Â§4 ("Future features
  must reuse these helpers").
- `internal/shared/apierror` (`Write` + typed constructors), used by `customer` and
  `servicecatalog`: `{"error": {"code", "message", "details"?}}`. Decided in
  `specs/customer-management/design.md` Â§1.5 ("Future features should reuse
  `internal/shared/apierror`"); the service catalog adopted it on 2026-08-19 instead of
  keeping a second exception (`specs/service-catalog/design.md` Â§2).

Both specs independently claimed to set "the" project-wide convention, written in parallel
without either branch aware of the other. Both packages compile and work correctly today â€”
this is a documentation/consistency problem, not a build error â€” but it means a further
feature had no unambiguous shared error helper to reach for. `auth` is now the only feature
still on `httpx`; whether it migrates too is an explicit open decision (see Â§10 and
`CLAUDE.md` Â§17) â€” its 401/500 response bodies, and `middleware.RequireAuth`'s, would change
with it, so it was not done while merging the branches.

Per-feature error handling, as implemented today:
- `auth.Handler` maps `ErrInvalidCredentials` â†’ 401, `ErrUserNotFound` â†’ 401 (so a token
  for a deleted user behaves like "unauthenticated," not "not found" â€” avoids leaking
  account existence), any other error â†’ 500 with a generic body (the underlying error is
  logged server-side only, checked to never contain a password/token/hash â€” BR5).
  `shared/middleware.RequireAuth` responds 401 via the same `httpx.Error` envelope for a
  missing/malformed `Authorization` header and for any token verification failure (bad
  signature, wrong algorithm, expired â€” all folded into one generic `ErrInvalidToken`
  before reaching the HTTP layer).
- `customer/handler.go`'s `writeServiceError` maps the feature's sentinel errors
  (`ErrNotFound`, `ErrDuplicateDocument`, `ErrDuplicateEmail`, `ErrInvalidDocument`) to
  `apierror`'s envelope; anything else becomes a generic `500` via `apierror.Internal`,
  never leaking internal error text to the client. HTTP status mapping: `400` for a
  malformed body or any validation failure (structural or business â€” **400 was chosen over
  422** for simplicity), `404` not found, `409` duplicate document/email.
- `vehicle/httpsupport.go`'s `writeServiceError` follows the identical pattern, mapping
  `ErrNotFound` â†’ `404 NOT_FOUND`, `ErrCustomerNotFound` â†’ `404 CUSTOMER_NOT_FOUND` (a
  distinct code from the vehicle's own not-found, same reasoning as splitting
  `DUPLICATE_DOCUMENT`/`DUPLICATE_EMAIL`), `ErrCustomerInactive` â†’ `409 CUSTOMER_INACTIVE`
  (a state conflict, not a malformed request â€” kept `409`, not `400`, on purpose),
  `ErrDuplicatePlate` â†’ `409 DUPLICATE_LICENSE_PLATE`, `ErrInvalidPlate`/`ErrInvalidYear` â†’
  `400 VALIDATION_ERROR`. `CUSTOMER_NOT_FOUND` is built from `apierror.Error`'s exported
  fields directly rather than `apierror.NotFound(...)`, since that constructor hardcodes its
  code to `"NOT_FOUND"` â€” this required no change to the shared `apierror` package itself.
- `servicecatalog.Handler`'s `fail` helper maps `ValidationError` (via `errors.As`) to
  `apierror.Validation` (400 `VALIDATION_ERROR`, naming the offending field in `details`),
  `ErrServiceNotFound` â†’ `apierror.NotFound` (404), `ErrCodeAlreadyExists` â†’
  `apierror.Conflict` (409 `CODE_ALREADY_EXISTS`), a body that is not JSON or a non-UUID
  `{id}` â†’ `apierror.BadRequest` (400 `INVALID_BODY`), anything else â†’
  `apierror.Internal` (500) with the cause logged only.
- There is no centralized error-handling middleware in any feature; each handler performs
  its own `errors.Is`/`errors.As` mapping. Whether a shared error-mapping helper is worth
  extracting (on top of resolving the envelope duplication above) is **to be defined**.
- **Startup fatal errors**: `main.go` uses `log.Fatal`/`log.Fatalf` for missing required
  configuration (`DATABASE_URL`, `JWT_SECRET`, malformed `JWT_TTL`), database connection
  failure, and `http.ListenAndServe` failure.

## 9. Testing strategy

All four implemented features follow the convention in [CLAUDE.md](../CLAUDE.md) Â§11 â€”
tests alongside the code, integration tests in `internal/handlers_test/`, each independently
skipping without `DATABASE_URL` â€” but differ, deliberately, on test library choice:

- **auth**: stdlib `testing` only (no test library adopted â€” considered and explicitly
  declined, `specs/auth/design.md` Â§9). Hand-written fakes satisfy the service's own small
  interfaces (`UserFinder`, `TokenIssuer`, `TokenVerifier`) â€” no mocking library.
  - Unit: `internal/features/auth/service_test.go` (login logic against fakes),
    `internal/features/auth/handler_test.go` (HTTP layer via `httptest`, including 500 and
    400 paths), `internal/shared/token/token_test.go` (generate/verify round trip, expired/
    wrong-secret/wrong-algorithm rejection â€” alg-confusion regression coverage),
    `internal/shared/middleware/auth_test.go`, `internal/shared/httpx/respond_test.go`.
  - Integration: `internal/handlers_test/auth_test.go` â€” login success/failure (AC1, AC2,
    BR4), `/me` without/with invalid/with valid token (AC3, AC4).
- **customer**: stdlib `testing` + `testify` (`require`/`assert`), adopted deliberately
  (`specs/customer-management/requirements.md` Â§8).
  - Unit: `internal/shared/document/*_test.go` (CPF/CNPJ table-driven cases, including the
    alphanumeric CNPJ format), `internal/features/customer/{model,service}_test.go` against
    a hand-written in-memory `fakeRepository` â€” no mocking framework either.
  - Integration: `internal/handlers_test/customer_test.go` â€” full CRUD flow, invalid CPF/
    CNPJ, duplicate document/email (create and update), pagination, logical-deactivation-
    not-physical-delete, and a test proving the database unique constraint (not just the
    application pre-check) catches a simulated race condition.
- **vehicle**: stdlib `testing` + `testify`, following `customer`'s choice (the feature it
  most resembles, per `specs/vehicle-management/requirements.md` Â§8's application of
  `customer-management`'s "reuse what the closest feature already uses" logic).
  - Unit: `internal/features/vehicle/plate_test.go` (legacy/Mercosul table-driven cases),
    `internal/features/vehicle/{model,service}_test.go` against a hand-written in-memory
    `fakeRepository` and `fakeCustomerLookup` â€” no mocking framework.
  - Integration: `internal/handlers_test/vehicle_test.go` â€” every route asserted `401`
    without a bearer token, full CRUD flow, invalid plate (both formats rejected), year out
    of range, duplicate plate (create), customer not found (404)/inactive (409) on create,
    pagination on both list endpoints, logical-deactivation-not-physical-delete, and the
    same database-unique-constraint-catches-a-race test `customer_test.go` has for its own
    uniqueness rule.
- **servicecatalog**: stdlib `testing` only, with a hand-written in-memory fake satisfying
  the `Store` interface the business layer declares â€” no test library, following the auth
  slice it extends.
  - Unit: `internal/features/service-catalog/service_test.go` (required name, negative
    price, invalid estimated time/code, empty update, duplicate code, logical deletion),
    `internal/features/service-catalog/handler_test.go` (the five endpoints via `httptest`
    on the same route patterns as `main.go`, including the 400/404/409 paths and a 500 path
    asserted not to leak the underlying error text).
  - Integration: `internal/handlers_test/service_catalog_test.go` â€” creation with and
    without a caller-supplied code (AC1), duplicate code (AC2), invalid price/estimated
    time (AC3/AC4), active vs. inactive listing (AC5), update persistence (AC6), logical
    deletion asserted at the row level (AC7), and 401 on every route without a valid token
    (AC8). Rows created by the tests are removed in `t.Cleanup`, since a logical deletion
    would otherwise leave them behind.
- **Coverage targets**: none set numerically for any feature â€” "every new feature needs
  tests" (`CLAUDE.md` Â§11), not a percentage gate.
- CI ([.github/workflows/ci.yml](../.github/workflows/ci.yml)) runs `go build/vet/test
  ./...` with no Postgres service configured, so every feature's integration tests currently
  run in **skip mode** in CI; provisioning a Postgres service for CI to exercise them for
  real remains **to be defined**.

## 10. Identified architectural decisions

Decisions that **are actually observable** in the repository's code/configuration, with
their source:

1. **Organization by feature (vertical slice)**, not by global technical layer â€” declared
   in [README.md](../README.md) ("Vertical Slice (Feature-based)") and implemented in
   `internal/features/auth/`, `internal/features/customer/`, and
   `internal/features/service-catalog/`.
2. **Plain Go stdlib for HTTP**, no framework/router â€” Go 1.22+'s method-aware
   `http.ServeMux` patterns are used directly by every feature; confirmed sufficient at the
   current route count.
3. **PostgreSQL as the database**, with a schema-first design in plain SQL
   ([docs/schema.sql](../docs/schema.sql)), accessed via `pgx v5` (no ORM), one shared
   connection pool for the whole process.
4. **UUID technical key + `code` sequential identifier** as the pattern on every registry
   table (`users`, `customers`, and every other table) â€” documented in the header of
   `docs/schema.sql`.
5. **Status/type enums as native Postgres `ENUM`**, not `CHECK` â€” rationale recorded in
   `docs/schema.sql` itself; `customer_document_type` and `customer_status` follow the same
   pattern.
6. **Containerized local environment** via Docker Compose with three services (`db`,
   `adminer`, `api`) â€” [docker-compose.yml](../docker-compose.yml); `api` now requires
   `JWT_SECRET` to start.
7. **Minimum CI gate**: every change goes through `go build ./...`, `go vet ./...`, and
   `go test ./...` on GitHub Actions on every push/PR.
8. **Domain identifiers in English**, consistently between `docs/entities.md` and
   `docs/schema.sql` â€” with no exception since 2026-08-26, when `ServiceOrder.status`'s
   enum values were renamed from Portuguese to English (`RECEBIDA` -> `RECEIVED`,
   `EM_DIAGNOSTICO` -> `IN_DIAGNOSIS`, `AGUARDANDO_APROVACAO` -> `AWAITING_APPROVAL`,
   `EM_EXECUCAO` -> `IN_PROGRESS`, `FINALIZADA` -> `COMPLETED`, `ENTREGUE` ->
   `DELIVERED`, `CANCELADA` -> `CANCELED`); until then they were the one deliberate
   carve-out. Customer Management's own new fields (`documentType`, `status`) and its routes (`/customers`, not
   `/clientes`) deliberately follow this English convention even though the task that
   originated the feature was written in Portuguese â€” see
   `specs/customer-management/requirements.md` Â§5.
9. **`pgx/v5` (pgxpool) as the PostgreSQL driver**, plain parameterized SQL, no ORM/query
   builder â€” decided independently by both `specs/auth/design.md` Â§2 and
   `specs/customer-management/requirements.md` Â§8; versions reconciled to `v5.7.4` when the
   branches merged (Go 1.22 compatibility constraint â€” see `specs/customer-management/design.md` Â§0).
10. **JWT (HS256) via `golang-jwt/jwt/v5` for authentication** (RNF02), secret from
    `JWT_SECRET` (never versioned, BR2), every token carries an expiration (BR3) â€” decided
    in `specs/auth/design.md` Â§5, implemented in `internal/shared/token`.
11. **Password hashing with bcrypt** (`golang.org/x/crypto/bcrypt`), passwords never stored
    or logged in plain text (BR1, BR5) â€” implemented in `internal/features/auth/service.go`
    and `docs/seed.sql`.
12. **`testify` as the Customer Management feature's test assertion library**, added
    deliberately after explicit alignment (`specs/customer-management/requirements.md` Â§8);
    the auth feature deliberately did not adopt a test library (`specs/auth/design.md` Â§9).
    Both stand as valid per-feature choices â€” see `CLAUDE.md` Â§11.
13. **Explicit public-route allowlist** (FR6, from auth): routes not listed as public in
    `cmd/api/main.go` should be registered wrapped in `middleware.RequireAuth`. Today only
    `/health` and `POST /api/v1/auth/login` are public by that rule's original intent; the
    six Customer Management routes are, in practice, also currently unwrapped/public â€” see
    decision 16 below for why that's flagged rather than silently corrected.
14. **No role/permission model in the MVP** â€” a valid token is sufficient to reach any
    protected route; there is no 403 path, by explicit scope cut recorded in
    `specs/auth/requirements.md`.
15. **Two JSON error envelopes still coexist** (`shared/httpx` and `shared/apierror`) â€”
    see Â§8. Not a decision so much as an unresolved merge outcome; recorded here so it
    isn't mistaken for an intentional dual-envelope design. Narrowed on 2026-08-19: the
    service catalog adopted `apierror`, so `auth` (and `middleware.RequireAuth`) is the
    only remaining `httpx` user.
16. **The Customer Management routes are not wrapped in `middleware.RequireAuth`.** This
    technically diverges from decision 13's stated convention. It is left this way
    deliberately for now â€” `specs/customer-management/requirements.md` was written and
    approved before the auth feature existed, explicitly scoped JWT out
    ("implemented unauthenticated... a dedicated Security feature... will add JWT
    authentication/authorization... applied on top of the existing routes"), and wrapping
    the routes now would be a behavioral change (breaking any unauthenticated client/test
    already exercising them) made silently during a branch merge, which `CLAUDE.md` Â§17
    explicitly prohibits. Whether/when to wrap them is an open decision, not a bug.
17. **All seven Vehicle Management routes are wrapped in `middleware.RequireAuth`**, unlike
    Customer Management. This is not an inconsistency to reconcile: Vehicle Management's own
    requirements (RNF02, "todas as rotas administrativas exigem JWT") explicitly demand it,
    unlike Customer Management's requirements, which explicitly deferred JWT to a future
    Security feature (decision 16). `vehicle.RegisterRoutes` takes the same
    `middleware.RequireAuth(tokens)` value already built for auth's `/me` route as a plain
    `func(http.Handler) http.Handler` parameter â€” no new middleware/JWT code, and `vehicle`
    itself never imports `internal/shared/middleware`. This does not retroactively wrap the
    Customer Management routes; decision 16 remains open on its own.
18. **The customer-scoped vehicle listing lives at `GET /api/v1/vehicles/customer/{customerId}`,
    not the originally-specified `GET /api/v1/customers/{customerId}/vehicles`.** Discovered
    during implementation (the process panics at startup, not caught by `go build`/`go vet`/
    `go test`): Go 1.22's `http.ServeMux` requires that any two patterns matching an
    overlapping path set have one strictly more specific than the other, and
    `/api/v1/customers/{customerId}/vehicles` is genuinely ambiguous against customer's own
    pre-existing `GET /api/v1/customers/document/{document}` â€” both would match
    `/api/v1/customers/document/vehicles`. The fix keeps the route inside `vehicle`'s own
    `/api/v1/vehicles/` path prefix (mirroring the already-working
    `GET /api/v1/vehicles/plate/{plate}`), so it can never collide with a route any other
    feature registers, without touching customer's already-shipped route. See
    `specs/vehicle-management/design.md` Â§1.5 for the full account. This is a reusable
    lesson for any future feature that considers nesting a route under another feature's URL
    prefix: verify the actual `*http.ServeMux` registration succeeds (a real process start,
    not just `go build`/`go vet`/`go test`), don't assume path-string composition is
    automatically conflict-free.

19. **Caller-supplied but optional `code` on the service catalog**: `services.code` is
    `GENERATED BY DEFAULT AS IDENTITY`, so a registration may carry its own code (rejected
    with 409 on collision) or omit it and let the database generate one â€” decided in
    `specs/service-catalog/design.md` Â§2 (D1). Every other table keeps
    `GENERATED ALWAYS`.
20. **Logical deletion in the service catalog**: `DELETE /api/v1/services/{id}` sets
    `active = false` and never removes the row, so quotes/service orders referencing it keep
    their history (D2). Customer Management reached the same conclusion independently for
    `customers.status`; whether it becomes a project-wide convention is **to be defined**.
21. **List responses use an `{"items": [...]}` envelope** in the service catalog (its
    listing has no pagination), while Customer Management's listing carries its own
    pagination shape â€” another consistency item for whoever unifies the response
    conventions.

Any architectural decision outside this list (migration tool, linter beyond
`gofmt`/`go vet`, authorization/roles, refresh tokens, OpenAPI/Swagger documentation for
auth and the service catalog, external integrations) **has not been made yet in code** and
should be treated as **"To be defined"** until resolved by a specification in
`specs/<feature>/design.md`.
