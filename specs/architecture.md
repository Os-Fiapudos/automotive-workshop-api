# Current architecture — automotive-workshop-api

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
third-party web framework/router. It exposes `GET /health` plus three implemented business
features:
- **auth** (`internal/features/auth/`, [specs/auth/](auth/)) — `POST /api/v1/auth/login`
  and the protected `GET /api/v1/auth/me`.
- **customer** (`internal/features/customer/`, [specs/customer-management/](customer-management/))
  — the six Customer Management endpoints under `/api/v1/customers`.
- **vehicle** (`internal/features/vehicle/`, [specs/vehicle-management/](vehicle-management/))
  — the seven Vehicle Management endpoints under `/api/v1/vehicles`, every one of them
  JWT-protected (unlike `customer`'s routes).

The folder organization follows the **cmd/ + internal/** pattern, with **vertical slice
(organization by feature)** as the adopted convention: each business feature gathers
handler, service, repository, and model in a single package under
`internal/features/<feature>/`. `internal/features/auth/`, `internal/features/customer/`,
and `internal/features/vehicle/` all implement this end to end (`customer` and `vehicle`
organized internally by responsibility file — `model.go`, `dto.go`, `repository.go`,
`service.go`, `handler.go`, `errors.go`, plus `vehicle`'s own `plate.go` and both features'
`httpsupport.go` — rather than by use case; see `specs/customer-management/design.md` §1.1
for the rationale, reused as-is by `specs/vehicle-management/design.md` §1.1).
`internal/features/user/` remains an empty placeholder (`doc.go` only) — note this is
unrelated to auth's `users` database table; it's a distinct, not-yet-specified future
feature.

`internal/shared/` holds cross-cutting code from all three features: a single shared
Postgres connection pool (`shared/database`, first introduced by auth, now used by every
feature's repository), JWT issuing/verification (`shared/token`) and the authentication
middleware (`shared/middleware`) from auth — reused unchanged by `vehicle` for its own
JWT-protected routes — and CPF/CNPJ validation (`shared/document`) and the environment
config loader (`shared/config`) from Customer Management. `vehicle` deliberately does
**not** add a third shared package for its own license-plate validation
(`vehicle/plate.go` stays feature-local — see `specs/vehicle-management/design.md` §1.2):
unlike CPF/CNPJ, a license plate isn't a cross-feature concept, so it doesn't meet the
"genuinely generic" bar `CLAUDE.md` §9.3 sets for `shared/`. **Both `auth` and `customer`
each introduced their own JSON error-envelope package** (`shared/httpx` from auth,
`shared/apierror` from Customer Management) — this is flagged, not resolved, in §8 below; it
is a real duplication surfaced by merging the two branches, not a design decision to
imitate. `vehicle` reuses `shared/apierror` (the feature it most resembles structurally),
not a third envelope.

Communication is real across all three features: `main.go` builds the shared pool and JWT
manager, then wires `auth.NewHandler(...)` (routes: login public, `/me` behind
`middleware.RequireAuth`), `customer.RegisterRoutes(...)` (all six routes still public — see
§10, decision 16), and `vehicle.RegisterRoutes(...)` (all seven routes wrapped in the same
`requireAuth` middleware auth already built — see §10, decision 17) onto one
`*http.ServeMux`. `vehicle` needs to check, at vehicle creation, that a referenced customer
exists and is `ACTIVE` — it does this through a small `CustomerLookup` interface it declares
itself (mirroring how `auth.Service` depends on `UserFinder`), implemented by an adapter
`main.go` builds around the already-constructed `*customer.CustomerService` — so `vehicle`
never imports `internal/features/customer` directly, preserving "no feature imports another
feature" (§5 below) even though this is the first case where one feature's business logic
genuinely depends on another's current state.

## 2. Main components

| Component | Path | State |
| --- | --- | --- |
| HTTP entrypoint | `cmd/api/main.go` | Implemented — loads config (`DATABASE_URL`, `JWT_SECRET`, `JWT_TTL`, `PORT`), opens the shared Postgres pool, wires both `auth` and `customer`, registers `/health`, the public/protected auth routes, and the customer routes, starts the server. |
| `auth` feature | `internal/features/auth/` | Implemented — `handler.go` (HTTP), `service.go` (login/lookup logic), `repository.go` (pgx queries), `model.go` (`User`). Unit-tested (`handler_test.go`, `service_test.go`). See `specs/auth/`. |
| `customer` feature | `internal/features/customer/` | Implemented — model, DTOs, Postgres repository, service, HTTP handlers for all 6 endpoints. See `specs/customer-management/`. |
| `vehicle` feature | `internal/features/vehicle/` | Implemented — model, plate validation, DTOs, Postgres repository, service, HTTP handlers for all 7 endpoints, every route JWT-protected. See `specs/vehicle-management/`. |
| `user` feature | `internal/features/user/` | Not implemented — only `doc.go` declaring the package (placeholder for a future feature; unrelated to auth's `users` table). |
| `features` package (root) | `internal/features/doc.go` | Not implemented — only a package comment. |
| Shared: `database` | `internal/shared/database/` | Implemented — `NewPool` builds and pings a `pgxpool.Pool` from a `postgres://` URL. Used by both features' repositories. (Consolidated from two independent implementations, `NewPool` and `Connect`, that each branch introduced — see §10.) |
| Shared: `token` | `internal/shared/token/` | Implemented — `Manager` issues and verifies HS256 JWTs (`golang-jwt/jwt/v5`). Unit-tested (`token_test.go`), including alg-confusion regression coverage. |
| Shared: `middleware` | `internal/shared/middleware/` | Implemented — `RequireAuth` extracts and verifies the `Authorization: Bearer` header, injects the user id into the request context, or responds 401. Unit-tested (`auth_test.go`). |
| Shared: `httpx` | `internal/shared/httpx/` | Implemented — `JSON`/`Error` helpers producing `{"error":{"code","message"}}`. Used by `auth`. Unit-tested (`respond_test.go`). See §8 re: overlap with `apierror`. |
| Shared: `apierror` | `internal/shared/apierror/` | Implemented — the JSON error envelope and HTTP status mapping used by `customer` (`{"error":{"code","message","details"?}}`). See §8 re: overlap with `httpx`. |
| Shared: `document` | `internal/shared/document/` | Implemented — CPF/CNPJ normalize/detect-type/validate (check-digit algorithm, no third-party library), including the alphanumeric CNPJ format. |
| Shared: `config` | `internal/shared/config/` | Implemented — reads `DATABASE_URL`, `JWT_SECRET`, `JWT_TTL`, `PORT` from the environment. |
| Handler/integration tests | `internal/handlers_test/` | Implemented — `auth_test.go`, `customer_test.go`, and `vehicle_test.go`, each driving real HTTP against a real Postgres, each independently skipping (not failing) without `DATABASE_URL`/a reachable database. |
| Database schema | `docs/schema.sql` | Implemented as plain SQL; consumed by every feature's repository (`users` table for auth, `customers` table — with its `document_type`/`status` columns/enums — for Customer Management, `vehicles` table — with its new `status` column/enum — for Vehicle Management). |
| Sample data | `docs/seed.sql` | Implemented as plain SQL; applied manually via `psql`. Includes one seeded administrative user (bcrypt-hashed via pgcrypto `crypt()`), four sample customers, and five sample vehicles (one `INACTIVE`, owned by the one `INACTIVE` customer). |
| Domain model | `docs/entities.md` | Domain documentation for all entities, now including `User`. `Customer`, `Vehicle`, and `User` are the only entities with a corresponding Go implementation. |
| API documentation | `docs/openapi.yaml` | Implemented for the Customer Management and Vehicle Management endpoints (schemas, pagination, error envelope, `bearerAuth` security scheme for Vehicle Management). The auth endpoints are not yet documented here — `specs/auth/requirements.md` scoped that out (RNF10) as a separate future feature. |
| Local environment | `docker-compose.yml`, `Dockerfile` | Implemented — orchestrates `db` (Postgres), `adminer`, and `api`; `api` now requires `JWT_SECRET` (fails fast via compose variable substitution if unset). |
| CI | `.github/workflows/ci.yml` | Implemented — runs `go build ./...`, `go vet ./...`, `go test ./...`. |

## 3. Responsibilities

- **`cmd/api/main.go`**: the process's single entrypoint. Loads config
  (`internal/shared/config`), opens the shared Postgres pool (`internal/shared/database`),
  builds the JWT token manager and both features' repositories/services, builds one
  `*http.ServeMux`, registers `/health` inline, registers auth's public/protected routes,
  and delegates every `/api/v1/customers*` route to `customer.RegisterRoutes`. It stays
  thin — no business logic, request parsing, or data access lives in `main.go` itself, per
  `CLAUDE.md` §9.4.
- **`internal/features/auth/`**:
  - `handler.go` — decodes/validates HTTP input, translates service errors to the HTTP
    error envelope (`httpx`), never logs credentials or tokens.
  - `service.go` — `Login` (credential check + token issuance) and `UserByID` (identity
    lookup for `/me`); depends only on the `UserFinder`/`TokenIssuer` interfaces it
    declares, not on concrete `shared` types, so it stays unit-testable with fakes.
  - `repository.go` — parameterized `pgx` queries against `users`; maps "no rows" to
    `ErrUserNotFound`.
  - `model.go` — the `User` struct mirroring the `users` table / `docs/entities.md`.
- **`internal/features/customer/`**: (see `specs/customer-management/design.md` §1.1 for
  the full rationale for this file layout)
  - `handler.go` — HTTP layer: request parsing/validation, DTO ⇄ domain conversion, status
    code mapping (`apierror`). Depends on `service.go`.
  - `service.go` — one method per use case, orchestrates domain + repository. Depends on
    the `CustomerRepository` interface (not the concrete Postgres type).
  - `repository.go` — the `CustomerRepository` interface and its `pgx`-backed
    implementation. Depends on `model.go` and `internal/shared/document`.
  - `model.go` — the `Customer` aggregate and its invariants (always starts `ACTIVE`,
    document only settable through validated construction, no `Activate` method).
  - `dto.go` — HTTP request/response shapes, independent of the domain type.
- **`internal/features/vehicle/`**: (see `specs/vehicle-management/design.md` §1.1 for the
  full rationale — same file layout as `customer`, plus `plate.go`)
  - `handler.go`/`httpsupport.go` — HTTP layer: request parsing/validation, DTO ⇄ domain
    conversion, status code mapping (`apierror`, reused from `customer`, not a third
    envelope). Every route is wrapped in the `requireAuth` middleware `main.go` passes in
    (RNF02). Depends on `service.go`.
  - `service.go` — one method per use case, orchestrates domain + repository +
    `CustomerLookup`. Depends on the `VehicleRepository` interface (not the concrete
    Postgres type) and the `CustomerLookup` interface it declares itself (satisfied by an
    adapter `main.go` builds around `*customer.CustomerService` — see §1 above).
  - `repository.go` — the `VehicleRepository` interface and its `pgx`-backed implementation.
    Depends only on `model.go` — no dependency on `internal/shared/document` or any
    `customer` type.
  - `model.go` — the `Vehicle` aggregate and its invariants (always starts `ACTIVE`, plate
    only settable through validated construction, year re-validated on every update, no
    `Activate` method).
  - `plate.go` — license-plate `Normalize`/`Validate` (legacy + Mercosul formats), feature-
    local rather than `internal/shared/` (§1 above).
  - `dto.go` — HTTP request/response shapes, independent of the domain type; the update
    request type has no field for license plate or customer id — both are immutable after
    creation, enforced by the type itself, not just handler logic.
- **`internal/features/user/`**: intended responsibility unchanged (folder convention +
  package comment) — still no concrete implementation.
- **`internal/shared/`**: genuinely cross-cutting code only (`CLAUDE.md` §9.3), each
  subpackage imported only by the feature(s) that need it and by `main.go`: `database`
  (both features), `token`/`middleware` (auth), `httpx` (auth's error envelope),
  `document`/`config`/`apierror` (customer's CPF/CNPJ validation, config loading, and error
  envelope respectively).
- Responsibility split within a feature (handler = HTTP concerns and error mapping, service
  = business rules against interfaces/repository, repository = SQL, model = data shape) is
  now observable in both implemented features and is expected of future ones too.

## 4. Flow of the main operations

```
GET /health → inline handler in main.go → json.Encode({"status":"ok"}) → HTTP response

POST /api/v1/auth/login
  → auth.Handler.Login: decode/validate JSON body (400 on malformed/missing fields)
  → auth.Service.Login(email, password)
      → Repository.FindByEmail (pgx, parameterized query on users)
      → bcrypt.CompareHashAndPassword (unknown email OR wrong password → single
        ErrInvalidCredentials, so the client cannot distinguish them — BR4)
      → token.Manager.Generate (HS256 JWT, sub=user id, exp=now+TTL)
  → 200 {"access_token", "token_type", "expires_in"}  |  401 generic envelope on
    ErrInvalidCredentials  |  500 on unexpected error (DB/signing failure)

GET /api/v1/auth/me
  → middleware.RequireAuth: extract "Authorization: Bearer <token>", token.Manager.Verify
    (rejects missing header, bad signature, wrong alg, and expired tokens) → 401 on failure,
    otherwise injects the user id into the request context
  → auth.Handler.Me → auth.Service.UserByID → Repository.FindByID
  → 200 {"id","code","name","email"} (no password hash)  |  401 if the user no longer
    exists  |  500 on unexpected error

POST/GET/PATCH/DELETE /api/v1/customers... (currently unauthenticated — see §10 decision 16)
  → customer.handler (parses/validates request, maps errors to apierror's envelope)
  → customer.CustomerService (business rules: starts ACTIVE, no reactivation, partial update, ...)
  → customer.CustomerRepository → pgx → Postgres `customers` table
  → customer.handler (DTO response) → HTTP response

POST/GET/PATCH/DELETE /api/v1/vehicles... (every route requires a valid JWT — §10 decision 17)
  → middleware.RequireAuth (same check as GET /api/v1/auth/me) → 401 on missing/invalid/
    expired token, otherwise proceeds
  → vehicle.handler (parses/validates request, maps errors to apierror's envelope)
  → vehicle.VehicleService
      → on Create: CustomerLookup.IsActiveCustomer (→ 404 CUSTOMER_NOT_FOUND if the
        customer doesn't exist, 409 CUSTOMER_INACTIVE if it exists but isn't ACTIVE) →
        NewVehicle (normalizes/validates the plate, validates the year range) →
        ExistsByPlate pre-check (→ 409 DUPLICATE_LICENSE_PLATE) → Create
      → other use cases: business rules (starts ACTIVE, no reactivation, PATCH limited to
        brand/model/year/color, plate/customerId immutable, ...)
  → vehicle.VehicleRepository → pgx → Postgres `vehicles` table (unique-violation on
    `ux_vehicles_license_plate` also mapped to DUPLICATE_LICENSE_PLATE, catching a
    concurrent-request race the pre-check alone can't)
  → vehicle.handler (DTO response) → HTTP response
```

No other feature's operation flow is implemented (product, service registration, service
orders, quotes, etc.). `docs/entities.md` describes the **data model** of these entities and
a service order status flow
(`RECEBIDA → EM_DIAGNOSTICO → AGUARDANDO_APROVACAO → EM_EXECUCAO → FINALIZADA → ENTREGUE`),
but that remains domain documentation only — no Go logic implements it yet. **To be
defined** once those features are specified and implemented. Note the documented **future**
invariants (not yet implemented, since Service Order does not exist): opening a service
order must reject an `INACTIVE` customer (see `specs/customer-management/requirements.md`
§7.1) or an `INACTIVE` vehicle (see `specs/vehicle-management/requirements.md` §7.1).

## 5. Communication between components

Both implemented features follow the convention declared in [CLAUDE.md](../CLAUDE.md) §9:

- **Within a feature**: `handler.go` → `service.go` → `repository.go`. `auth`'s `Service`
  depends on the `UserFinder`/`TokenIssuer` interfaces it defines itself (not on
  `*Repository`/`*token.Manager` directly), which is what lets its tests use fakes instead
  of a real database/signer. `customer`'s `service.go` similarly depends only on the
  `CustomerRepository` interface, not the concrete Postgres type. `vehicle`'s `service.go`
  depends on both the `VehicleRepository` interface and the `CustomerLookup` interface (see
  below), never a concrete type.
- **Between a feature and shared code**: `auth` imports
  `internal/shared/{database,token,httpx,middleware}`; `customer` imports
  `internal/shared/{database,document,config,apierror}`; `vehicle` imports
  `internal/shared/{database,apierror}` only — no `middleware` import (the `requireAuth`
  middleware value is passed into `vehicle.RegisterRoutes` by `main.go` instead, so
  `vehicle` never needs to import `internal/shared/middleware` or hold a `token.Manager`
  itself) and no new shared package for plate validation (§1 above). `cmd/api/main.go` is
  the only place that imports every feature and wires their concrete dependencies together.
- **Between features**: no feature imports another feature's package directly — `auth`,
  `customer`, and `vehicle` are fully independent Go packages of each other. `auth` and
  `customer` share nothing but `internal/shared/database`'s pool. `vehicle` is the first
  feature whose business logic genuinely depends on another feature's current state (BR1:
  a vehicle's customer must exist and be `ACTIVE`) — it resolves this via a `CustomerLookup`
  interface it declares itself (mirroring `auth.Service`'s `UserFinder` pattern), satisfied
  by a small adapter `main.go` builds around the already-constructed
  `*customer.CustomerService`. `internal/features/vehicle/` itself never imports
  `internal/features/customer/` — only `main.go`, which already imports every feature as the
  composition root, does.

## 6. Persistence

Implemented via `github.com/jackc/pgx/v5` (`pgxpool.Pool`), no ORM/query builder, one shared
pool for the whole process:

- **Driver**: `pgx/v5`, pinned in `go.mod` at `v5.7.4` (the newest release whose own `go`
  directive still allows Go 1.22 — see `specs/customer-management/design.md` §0). Plain
  parameterized SQL throughout.
- **Connection**: `internal/shared/database.NewPool(ctx, url)` opens a `pgxpool.Pool` and
  validates it with a `Ping`. Called once from `cmd/api/main.go` with `DATABASE_URL`; the
  pool is closed via `defer` on shutdown and shared by both features' repositories.
- **Schema**: [docs/schema.sql](../docs/schema.sql) defines all tables, applied
  automatically by the `db` container only on initial Docker volume creation
  (`docker-entrypoint-initdb.d`), not by application code. No migration tool is used to
  evolve the schema after initial creation — **to be defined**, see `CLAUDE.md` §14.
  - `users` (auth): `id UUID`, `code BIGINT IDENTITY`, `name`, `email UNIQUE`,
    `password_hash`, `created_at`/`updated_at`.
  - `customers` (Customer Management): the pre-existing table plus two columns added by
    this feature, `document_type customer_document_type` and `status customer_status`
    (two new enums), directly in the `CREATE TABLE` — see
    `specs/customer-management/design.md` §3.1.
  - `vehicles` (Vehicle Management): the pre-existing table plus one column added by this
    feature, `status vehicle_status` (one new enum, mirroring `customer_status`), directly
    in the `CREATE TABLE` — see `specs/vehicle-management/design.md` §3.1. The pre-existing
    `customer_id UUID NOT NULL REFERENCES customers (id) ON DELETE RESTRICT` foreign key
    (present before this feature) guarantees the referenced customer *exists*; it cannot
    express "and is `ACTIVE`" (Postgres FKs don't see other columns) — that half of BR1 is
    the application-level `CustomerLookup` check (§1 above).
- **Repository pattern**: one interface + implementation per feature (not a generic
  repository). `auth`'s `Repository` maps `pgx.ErrNoRows` to `ErrUserNotFound`.
  `customer`'s `CustomerRepository` maps `pgx.ErrNoRows` to `ErrNotFound`, and inspects
  `pgconn.PgError.ConstraintName` on a `23505` unique-violation to distinguish a duplicate
  `document` (`ux_customers_document`) from a duplicate `email`
  (`ux_customers_email` — a pre-existing invariant predating the feature, see
  `specs/customer-management/requirements.md` §3.4.1) rather than assuming it's always the
  document. `vehicle`'s `VehicleRepository` maps `pgx.ErrNoRows` to `ErrNotFound` and a
  `23505` violation on `ux_vehicles_license_plate` (a pre-existing index, predating this
  feature) to `ErrDuplicatePlate`, same two-layer defense (application pre-check +
  database-constraint mapping) as `customer`'s document uniqueness.
- **Seed**: [docs/seed.sql](../docs/seed.sql) populates sample data via manual `psql`: one
  administrative user (`admin@workshop.local`) with a bcrypt hash produced by pgcrypto's
  `crypt()` at insert time (plaintext dev-only password documented only in the seed file's
  SQL comment, never in Go code or logs), four sample customers with normalized CPF/CNPJ
  documents, and five sample vehicles with normalized Mercosul-format plates (one
  `INACTIVE`, owned by the one `INACTIVE` sample customer — illustrating that inactivating a
  customer doesn't retroactively touch its pre-existing vehicles' own status).
- **Other entities** (`products`, `services`, `service_orders`, `quotes`,
  `service_order_history`, `audit_services`): schema exists in
  [docs/schema.sql](../docs/schema.sql) but still has no Go repository.

## 7. External integrations

No external integration (third-party APIs, message queues, e-mail/SMS services, payment
gateways, etc.) is present in the code or in `go.mod`. `docs/entities.md` mentions e-mail
notification as the *purpose* of the Customer's `email` field ("used for notifications and
communication"), but no notification-sending mechanism is implemented. **To be defined**
should any external integration be specified in the future.

## 8. Error handling

**Two error-envelope implementations currently coexist** — this is the most significant
open item this document surfaces, not a design decision to preserve:

- `internal/shared/httpx` (`JSON`/`Error`), used by `auth`: `{"error": {"code", "message"}}`.
  Decided in `specs/auth/design.md` §4 ("Future features must reuse these helpers").
- `internal/shared/apierror` (`Write` + typed constructors), used by `customer`:
  `{"error": {"code", "message", "details"?}}`. Decided in
  `specs/customer-management/design.md` §1.5 ("Future features should reuse
  `internal/shared/apierror`").

Both specs independently claimed to set "the" project-wide convention, written in parallel
without either branch aware of the other. Both packages compile and work correctly today —
this is a documentation/consistency problem, not a build error — but it means a third
feature has no unambiguous shared error helper to reach for. Resolving which one survives
(and migrating the other feature's handler to it) is an explicit open decision (see §10 and
`CLAUDE.md` §17), deliberately not resolved as part of merging the two branches.

Per-feature error handling, as implemented today:
- `auth.Handler` maps `ErrInvalidCredentials` → 401, `ErrUserNotFound` → 401 (so a token
  for a deleted user behaves like "unauthenticated," not "not found" — avoids leaking
  account existence), any other error → 500 with a generic body (the underlying error is
  logged server-side only, checked to never contain a password/token/hash — BR5).
  `shared/middleware.RequireAuth` responds 401 via the same `httpx.Error` envelope for a
  missing/malformed `Authorization` header and for any token verification failure (bad
  signature, wrong algorithm, expired — all folded into one generic `ErrInvalidToken`
  before reaching the HTTP layer).
- `customer/handler.go`'s `writeServiceError` maps the feature's sentinel errors
  (`ErrNotFound`, `ErrDuplicateDocument`, `ErrDuplicateEmail`, `ErrInvalidDocument`) to
  `apierror`'s envelope; anything else becomes a generic `500` via `apierror.Internal`,
  never leaking internal error text to the client. HTTP status mapping: `400` for a
  malformed body or any validation failure (structural or business — **400 was chosen over
  422** for simplicity), `404` not found, `409` duplicate document/email.
- `vehicle/httpsupport.go`'s `writeServiceError` follows the identical pattern, mapping
  `ErrNotFound` → `404 NOT_FOUND`, `ErrCustomerNotFound` → `404 CUSTOMER_NOT_FOUND` (a
  distinct code from the vehicle's own not-found, same reasoning as splitting
  `DUPLICATE_DOCUMENT`/`DUPLICATE_EMAIL`), `ErrCustomerInactive` → `409 CUSTOMER_INACTIVE`
  (a state conflict, not a malformed request — kept `409`, not `400`, on purpose),
  `ErrDuplicatePlate` → `409 DUPLICATE_LICENSE_PLATE`, `ErrInvalidPlate`/`ErrInvalidYear` →
  `400 VALIDATION_ERROR`. `CUSTOMER_NOT_FOUND` is built from `apierror.Error`'s exported
  fields directly rather than `apierror.NotFound(...)`, since that constructor hardcodes its
  code to `"NOT_FOUND"` — this required no change to the shared `apierror` package itself.
- There is no centralized error-handling middleware in any feature; each handler performs
  its own `errors.Is` mapping. Whether a shared error-mapping helper is worth extracting (on
  top of resolving the envelope duplication above) is **to be defined**.
- **Startup fatal errors**: `main.go` uses `log.Fatal`/`log.Fatalf` for missing required
  configuration (`DATABASE_URL`, `JWT_SECRET`, malformed `JWT_TTL`), database connection
  failure, and `http.ListenAndServe` failure.

## 9. Testing strategy

All three implemented features follow the convention in [CLAUDE.md](../CLAUDE.md) §11 —
tests alongside the code, integration tests in `internal/handlers_test/`, each independently
skipping without `DATABASE_URL` — but differ, deliberately, on test library choice:

- **auth**: stdlib `testing` only (no test library adopted — considered and explicitly
  declined, `specs/auth/design.md` §9). Hand-written fakes satisfy the service's own small
  interfaces (`UserFinder`, `TokenIssuer`, `TokenVerifier`) — no mocking library.
  - Unit: `internal/features/auth/service_test.go` (login logic against fakes),
    `internal/features/auth/handler_test.go` (HTTP layer via `httptest`, including 500 and
    400 paths), `internal/shared/token/token_test.go` (generate/verify round trip, expired/
    wrong-secret/wrong-algorithm rejection — alg-confusion regression coverage),
    `internal/shared/middleware/auth_test.go`, `internal/shared/httpx/respond_test.go`.
  - Integration: `internal/handlers_test/auth_test.go` — login success/failure (AC1, AC2,
    BR4), `/me` without/with invalid/with valid token (AC3, AC4).
- **customer**: stdlib `testing` + `testify` (`require`/`assert`), adopted deliberately
  (`specs/customer-management/requirements.md` §8).
  - Unit: `internal/shared/document/*_test.go` (CPF/CNPJ table-driven cases, including the
    alphanumeric CNPJ format), `internal/features/customer/{model,service}_test.go` against
    a hand-written in-memory `fakeRepository` — no mocking framework either.
  - Integration: `internal/handlers_test/customer_test.go` — full CRUD flow, invalid CPF/
    CNPJ, duplicate document/email (create and update), pagination, logical-deactivation-
    not-physical-delete, and a test proving the database unique constraint (not just the
    application pre-check) catches a simulated race condition.
- **vehicle**: stdlib `testing` + `testify`, following `customer`'s choice (the feature it
  most resembles, per `specs/vehicle-management/requirements.md` §8's application of
  `customer-management`'s "reuse what the closest feature already uses" logic).
  - Unit: `internal/features/vehicle/plate_test.go` (legacy/Mercosul table-driven cases),
    `internal/features/vehicle/{model,service}_test.go` against a hand-written in-memory
    `fakeRepository` and `fakeCustomerLookup` — no mocking framework.
  - Integration: `internal/handlers_test/vehicle_test.go` — every route asserted `401`
    without a bearer token, full CRUD flow, invalid plate (both formats rejected), year out
    of range, duplicate plate (create), customer not found (404)/inactive (409) on create,
    pagination on both list endpoints, logical-deactivation-not-physical-delete, and the
    same database-unique-constraint-catches-a-race test `customer_test.go` has for its own
    uniqueness rule.
- **Coverage targets**: none set numerically for any feature — "every new feature needs
  tests" (`CLAUDE.md` §11), not a percentage gate.
- CI ([.github/workflows/ci.yml](../.github/workflows/ci.yml)) runs `go build/vet/test
  ./...` with no Postgres service configured, so all three features' integration tests
  currently run in **skip mode** in CI; provisioning a Postgres service for CI to exercise
  them for real remains **to be defined**.

## 10. Identified architectural decisions

Decisions that **are actually observable** in the repository's code/configuration, with
their source:

1. **Organization by feature (vertical slice)**, not by global technical layer — declared
   in [README.md](../README.md) ("Vertical Slice (Feature-based)") and implemented in both
   `internal/features/auth/` and `internal/features/customer/`.
2. **Plain Go stdlib for HTTP**, no framework/router — Go 1.22+'s method-aware
   `http.ServeMux` patterns are used directly by both features; confirmed sufficient at the
   current route count.
3. **PostgreSQL as the database**, with a schema-first design in plain SQL
   ([docs/schema.sql](../docs/schema.sql)), accessed via `pgx v5` (no ORM), one shared
   connection pool for the whole process.
4. **UUID technical key + `code` sequential identifier** as the pattern on every registry
   table (`users`, `customers`, and every other table) — documented in the header of
   `docs/schema.sql`.
5. **Status/type enums as native Postgres `ENUM`**, not `CHECK` — rationale recorded in
   `docs/schema.sql` itself; `customer_document_type` and `customer_status` follow the same
   pattern.
6. **Containerized local environment** via Docker Compose with three services (`db`,
   `adminer`, `api`) — [docker-compose.yml](../docker-compose.yml); `api` now requires
   `JWT_SECRET` to start.
7. **Minimum CI gate**: every change goes through `go build ./...`, `go vet ./...`, and
   `go test ./...` on GitHub Actions on every push/PR.
8. **Domain identifiers in English**, consistently between `docs/entities.md` and
   `docs/schema.sql` — with a single deliberate exception: `ServiceOrder.status` enum
   values are kept in Portuguese (`RECEBIDA`, `EM_DIAGNOSTICO`, `AGUARDANDO_APROVACAO`,
   `EM_EXECUCAO`, `FINALIZADA`, `ENTREGUE`) by explicit product decision. Customer
   Management's own new fields (`documentType`, `status`) and its routes (`/customers`, not
   `/clientes`) deliberately follow this English convention even though the task that
   originated the feature was written in Portuguese — see
   `specs/customer-management/requirements.md` §5.
9. **`pgx/v5` (pgxpool) as the PostgreSQL driver**, plain parameterized SQL, no ORM/query
   builder — decided independently by both `specs/auth/design.md` §2 and
   `specs/customer-management/requirements.md` §8; versions reconciled to `v5.7.4` when the
   branches merged (Go 1.22 compatibility constraint — see `specs/customer-management/design.md` §0).
10. **JWT (HS256) via `golang-jwt/jwt/v5` for authentication** (RNF02), secret from
    `JWT_SECRET` (never versioned, BR2), every token carries an expiration (BR3) — decided
    in `specs/auth/design.md` §5, implemented in `internal/shared/token`.
11. **Password hashing with bcrypt** (`golang.org/x/crypto/bcrypt`), passwords never stored
    or logged in plain text (BR1, BR5) — implemented in `internal/features/auth/service.go`
    and `docs/seed.sql`.
12. **`testify` as the Customer Management feature's test assertion library**, added
    deliberately after explicit alignment (`specs/customer-management/requirements.md` §8);
    the auth feature deliberately did not adopt a test library (`specs/auth/design.md` §9).
    Both stand as valid per-feature choices — see `CLAUDE.md` §11.
13. **Explicit public-route allowlist** (FR6, from auth): routes not listed as public in
    `cmd/api/main.go` should be registered wrapped in `middleware.RequireAuth`. Today only
    `/health` and `POST /api/v1/auth/login` are public by that rule's original intent; the
    six Customer Management routes are, in practice, also currently unwrapped/public — see
    decision 16 below for why that's flagged rather than silently corrected.
14. **No role/permission model in the MVP** — a valid token is sufficient to reach any
    protected route; there is no 403 path, by explicit scope cut recorded in
    `specs/auth/requirements.md`.
15. **Two JSON error envelopes currently coexist** (`shared/httpx` and `shared/apierror`) —
    see §8. Not a decision so much as an unresolved merge outcome; recorded here so it
    isn't mistaken for an intentional dual-envelope design.
16. **The Customer Management routes are not wrapped in `middleware.RequireAuth`.** This
    technically diverges from decision 13's stated convention. It is left this way
    deliberately for now — `specs/customer-management/requirements.md` was written and
    approved before the auth feature existed, explicitly scoped JWT out
    ("implemented unauthenticated... a dedicated Security feature... will add JWT
    authentication/authorization... applied on top of the existing routes"), and wrapping
    the routes now would be a behavioral change (breaking any unauthenticated client/test
    already exercising them) made silently during a branch merge, which `CLAUDE.md` §17
    explicitly prohibits. Whether/when to wrap them is an open decision, not a bug.
17. **All seven Vehicle Management routes are wrapped in `middleware.RequireAuth`**, unlike
    Customer Management. This is not an inconsistency to reconcile: Vehicle Management's own
    requirements (RNF02, "todas as rotas administrativas exigem JWT") explicitly demand it,
    unlike Customer Management's requirements, which explicitly deferred JWT to a future
    Security feature (decision 16). `vehicle.RegisterRoutes` takes the same
    `middleware.RequireAuth(tokens)` value already built for auth's `/me` route as a plain
    `func(http.Handler) http.Handler` parameter — no new middleware/JWT code, and `vehicle`
    itself never imports `internal/shared/middleware`. This does not retroactively wrap the
    Customer Management routes; decision 16 remains open on its own.
18. **The customer-scoped vehicle listing lives at `GET /api/v1/vehicles/customer/{customerId}`,
    not the originally-specified `GET /api/v1/customers/{customerId}/vehicles`.** Discovered
    during implementation (the process panics at startup, not caught by `go build`/`go vet`/
    `go test`): Go 1.22's `http.ServeMux` requires that any two patterns matching an
    overlapping path set have one strictly more specific than the other, and
    `/api/v1/customers/{customerId}/vehicles` is genuinely ambiguous against customer's own
    pre-existing `GET /api/v1/customers/document/{document}` — both would match
    `/api/v1/customers/document/vehicles`. The fix keeps the route inside `vehicle`'s own
    `/api/v1/vehicles/` path prefix (mirroring the already-working
    `GET /api/v1/vehicles/plate/{plate}`), so it can never collide with a route any other
    feature registers, without touching customer's already-shipped route. See
    `specs/vehicle-management/design.md` §1.5 for the full account. This is a reusable
    lesson for any future feature that considers nesting a route under another feature's URL
    prefix: verify the actual `*http.ServeMux` registration succeeds (a real process start,
    not just `go build`/`go vet`/`go test`), don't assume path-string composition is
    automatically conflict-free.

Any architectural decision outside this list (migration tool, linter beyond
`gofmt`/`go vet`, authorization/roles, refresh tokens, OpenAPI/Swagger documentation for
auth, external integrations) **has not been made yet in code** and should be treated as
**"To be defined"** until resolved by a specification in `specs/<feature>/design.md`.
