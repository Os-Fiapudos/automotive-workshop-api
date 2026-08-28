# automotive-workshop-api — Context and rules for Claude agents

This document is the permanent context for the project. Any change made by a Claude agent
in this repository must respect what is described here — in particular the
**Specification-Driven Development (SDD)** rules in the final section.

## 1. Project goal

REST API for managing an automotive workshop. It covers the full service flow: customer and
vehicle registration, catalog of products (parts/supplies) and services, opening service
orders, generating and approving quotes, and tracking the service order status until the
vehicle is delivered:

```
RECEIVED → IN_DIAGNOSIS → AWAITING_APPROVAL → IN_PROGRESS → COMPLETED → DELIVERED
```

> The status values above were renamed from Portuguese to English on 2026-08-26 — see the
> note in section 8.

It also keeps audit trails: history of service order status changes
(`ServiceOrderHistory`) and start/end records of the execution of each service
(`AuditServices`).

The full domain model (entities, fields, enums) is documented in
[docs/entities.md](docs/entities.md); the corresponding PostgreSQL schema in
[docs/schema.sql](docs/schema.sql).

**Current state**: the skeleton stage is over. The HTTP server (stdlib `net/http`, no
framework) exposes `/health` plus the implemented vertical slices:
- **auth** (`internal/features/auth/`, [specs/auth/](specs/auth/)): administrative login
  (`POST /api/v1/auth/login`) issuing a JWT, and a protected `GET /api/v1/auth/me`.
- **customer** (`internal/features/customer/`, [specs/customer-management/](specs/customer-management/)):
  full CRUD + logical deactivation for workshop customers (`/api/v1/customers*`, 6
  endpoints), CPF/CNPJ normalization and check-digit validation (including the
  alphanumeric CNPJ format in effect since July 2026).
- **service-catalog** (`internal/features/service-catalog/`, Go package `servicecatalog`,
  [specs/service-catalog/](specs/service-catalog/)):
  protected CRUD over `/api/v1/services` (5 endpoints) for the catalog of services and
  prices, with an `active` flag and logical deletion.

`internal/features/product/` and `internal/features/service-order/` are also present in the
code but are not described in this document or in `specs/architecture.md` yet — treat those
two sections as incomplete rather than authoritative for them.

These slices are backed by a real Postgres connection (`pgx/v5` via
`internal/shared/database`), with unit tests alongside the code and integration tests in
`internal/handlers_test/`. The database schema and sample data modeled in `docs/` are now
consumed by the implemented features (`users`, `customers`, and `services` tables). Every
other business entity (Vehicle, ServiceOrderHistory, Quote, etc.) is still schema/docs only,
with no corresponding Go feature yet — `internal/features/user/` (singular, unrelated to
`auth`'s `users` table — an unfortunately similar name, see the note in section 3) remains
an empty placeholder.

**Changed on 2026-08-26**: the customer endpoints, and Service Order Opening's
`POST /api/v1/service-orders`, are now wrapped in `middleware.RequireAuth` like every other
protected route, closing the two open decisions this section and section 17 used to record.
The trigger was a concrete exploit path found during a security review
([docs/owasp-vulnerability-and-coverage-report.md](docs/owasp-vulnerability-and-coverage-report.md),
VULN-01/VULN-02): `GET /api/v1/customers/document/{document}` returned a customer's name,
phone and e-mail to anyone able to produce a valid-looking CPF/CNPJ, with no credential at
all, and the unauthenticated order-creation route accepted that same CPF/CNPJ (or a license
plate) as an identifier, so the two combined let an unauthenticated caller confirm a document
belonged to a registered customer and open orders in their name. Historical notes elsewhere
in this repo describing these routes as unauthenticated (e.g.
`specs/customer-management/requirements.md` §7.2, `specs/service-order-opening/requirements.md`)
are records of the decision as it stood when those specs were written — they are no longer
the rule, same convention already used for the Go 1.22→1.25 note in section 2.

## 2. Technology stack

- **Language**: Go 1.25 (see [go.mod](go.mod)) — CI ([.github/workflows/ci.yml](.github/workflows/ci.yml))
  pins `go-version: "1.25"`, and [Dockerfile](Dockerfile) builds on `golang:1.25-alpine`.
  These three must stay in sync; when adding or upgrading a dependency, check that its own
  `go` directive (and its transitive dependencies') doesn't exceed 1.25.
  **Changed on 2026-08-24, from 1.22.** The project was pinned to Go 1.22 and several
  dependencies were deliberately held back to stay under that ceiling. The security analysis
  in [docs/security-report.md](docs/security-report.md) showed the pin had become the
  project's largest vulnerability source: 28 reachable vulnerabilities under Go 1.22.12, and
  it blocked the fix for a High-severity SQL-injection advisory in `pgx`. Raising the line to
  1.25 took the reachable count to zero. Historical notes about the 1.22 ceiling in
  `specs/auth/`, `specs/customer-management/`, and `specs/service-catalog/` are records of
  decisions made at the time — they are no longer the rule.
- **External dependencies**, all added deliberately after explicit alignment (not assumed —
  see §12): `github.com/jackc/pgx/v5` (Postgres driver/pool, `v5.10.0` — raised from `v5.7.4`
  by the Go 1.25 upgrade, which cleared advisory GO-2026-5004),
  `github.com/golang-jwt/jwt/v5` (JWT, from the auth feature —
  `specs/auth/design.md` §2), `golang.org/x/crypto` (bcrypt, same feature),
  `github.com/google/uuid` (from the Customer Management feature), and
  `github.com/stretchr/testify` (test-only, adopted by Customer Management; the auth
  feature's tests deliberately use stdlib `testing` only — both are fine, see section 11).
  Keep this list accurate as new dependencies are added.
- **HTTP**: stdlib `net/http`, using Go 1.22+'s method-aware `http.ServeMux` route patterns
  (e.g. `"POST /api/v1/customers"`, `"POST /api/v1/auth/login"`). No third-party web
  framework/router — both implemented features confirmed the stdlib mux is sufficient; do
  not add a router dependency without a concrete reason the stdlib mux can't handle.
- **Database**: PostgreSQL 16, schema versioned in [docs/schema.sql](docs/schema.sql)
  (UUID via `pgcrypto`, native enums, sequential `code` via
  `GENERATED ALWAYS AS IDENTITY`). Accessed via `pgx v5` (`pgxpool.Pool`), no ORM/query
  builder — one shared pool built by `internal/shared/database.NewPool` and injected into
  each feature's own `repository.go` from `cmd/api/main.go`.
- **Local infra**: Docker Compose with `db` (Postgres), `adminer` (DB UI), and `api`
  services ([docker-compose.yml](docker-compose.yml), [Dockerfile](Dockerfile)). `api` now
  requires `JWT_SECRET` to be set (compose fails fast via `${JWT_SECRET:?...}` if it's
  missing from `.env`).
- **CI**: GitHub Actions ([.github/workflows/ci.yml](.github/workflows/ci.yml)) running
  `go build ./...`, `go vet ./...`, and `go test ./...` on every push/PR.

## 3. Project structure

```
cmd/api/main.go             → HTTP entrypoint, wires up the server and registers feature routes
internal/features/          → one folder per business feature (vertical slice)
  features/auth/            → implemented slice: handler + service + repository + model
                               (login, /me; unit-tested)
  features/customer/        → implemented slice: handler + service + repository + model
                               (CRUD + deactivation; unit- and integration-tested)
  features/service-catalog/ → implemented slice: handler + service + repository + model
                               (service catalog CRUD over /api/v1/services; unit- and
                               integration-tested)
  features/user/            → placeholder slice: only has doc.go, no implementation yet.
                               NOTE: unrelated to auth's `users` database table — this is a
                               distinct, not-yet-specified future feature; don't conflate them.
internal/shared/            → cross-cutting code reused across features — implemented:
                               database (pgx pool), token (JWT), middleware (auth),
                               httpx (JSON writer + error envelope, used by auth),
                               apierror (JSON error envelope, used by customer and
                               servicecatalog — see section 8 for why there are currently two
                               and what that means for new code)
                               document (CPF/CNPJ), config (env var loading)
internal/handlers_test/     → handler/integration tests — implemented (auth_test.go,
                               customer_test.go, service_catalog_test.go), each skipped
                               independently when DATABASE_URL is unset
docs/                       → domain model (entities.md) and PostgreSQL schema
                               (schema.sql, seed.sql)
.github/workflows/ci.yml    → CI pipeline
Dockerfile, docker-compose.yml, .env.example → containerized local environment
```

specs/                      → SDD specifications: specs/README.md (process) and
                              specs/architecture.md (current architecture), with one
                              subfolder per feature (specs/auth/, specs/customer-management/)

## 4. Identified architecture

**Vertical Slice (organized by feature)**: each business feature lives in its own package
under `internal/features/<feature>/`, gathering all of that feature's layers
(handler/controller, service, repository, model) together — instead of splitting by
cross-cutting technical layer (a global `handlers/` package, a global `models/` package,
etc.). This is the pattern declared in [README.md](README.md), now implemented end to end
by `internal/features/auth/`, `internal/features/customer/`, and
`internal/features/service-catalog/`; `internal/features/user/` remains an unimplemented
placeholder folder.

Infrastructure layers are implemented: database connection (`internal/shared/database`, pgx
pool, shared by every feature), authentication middleware (`internal/shared/middleware`),
and JWT issuing/verification (`internal/shared/token`) — introduced by the auth feature and
reused as-is by the service catalog, which added no new shared code; CPF/CNPJ validation
(`internal/shared/document`), the API config loader (`internal/shared/config`), and a JSON
error envelope (`internal/shared/apierror`) — introduced by the Customer Management feature.
See [specs/architecture.md](specs/architecture.md) for the full, code-derived description.

## 5. How to run the application

Run the API locally (without a database):
```bash
go run ./cmd/api
```
(Needs `DATABASE_URL` and `JWT_SECRET` set in the environment — the process fails fast
without them. See `.env.example` for every variable.)

Bring up the full environment (Postgres + Adminer + API) via Docker Compose:
```bash
cp .env.example .env
docker compose up -d
```
- API: http://localhost:8080/health
- Postgres: `localhost:5432` (credentials in `.env`), schema applied automatically on the
  first startup of the volume.
- Adminer: http://localhost:8081 (system `PostgreSQL`, server `db`).

Recreate the database from scratch (e.g. after changing `schema.sql`, which only runs on
initial volume creation):
```bash
docker compose down -v
docker compose up -d
```

`docs/seed.sql` is mounted as `/docker-entrypoint-initdb.d/02-seed.sql`, so it runs
automatically after `schema.sql` on the initial creation of the volume. To re-apply it to an
existing volume (it is idempotent via `ON CONFLICT DO NOTHING`) — it includes two
administrative users (`admin@workshop.local` / `admin123` and
`soat-architecture@workshop.local` / `soat-architecture`, both dev/evaluation-only,
bcrypt-hashed at insert time):
```bash
docker compose cp docs/seed.sql db:/tmp/seed.sql
docker compose exec db psql -U workshop -d automotive_workshop -f /tmp/seed.sql
```

## 6. How to run the tests

```bash
go test ./...
```
This is the command CI runs, and any new feature must keep it passing. It runs the unit
tests alongside each feature/shared package
(`internal/features/{auth,customer,service-catalog}/*_test.go`, `internal/shared/*/*_test.go`)
plus the integration tests in `internal/handlers_test/` (`auth_test.go`, `customer_test.go`,
`service_catalog_test.go`). Each integration test file self-skips (`t.Skip`, not fail) when
`DATABASE_URL` is unset, so plain `go test ./...` stays green without a database.

To also run the integration tests against the local compose Postgres:
```bash
DATABASE_URL='postgres://workshop:workshop@localhost:5432/automotive_workshop?sslmode=disable' go test ./...
```

## 7. Lint, static analysis, and other checks

- `go vet ./...` — run in CI, the only static analysis configured today.
- `go build ./...` — also run in CI as a compilation check.
- `gosec` (SAST) and `govulncheck` (dependency/standard-library vulnerabilities) — run in
  CI by the `security` job and locally by `scripts/security-scan.sh`, both pinned to exact
  versions and executed via `go run <module>@<version>` so they never enter `go.mod` (see
  [specs/quality-and-security/](specs/quality-and-security/)). Findings and residual risks
  are recorded in [docs/security-report.md](docs/security-report.md).
- `scripts/coverage.sh` — enforces RNF06 (≥80% statement coverage on `service-order`,
  `product`, and `service-order-tracking`), run in CI by the `coverage` job against a real
  Postgres. It fails when `DATABASE_URL` is unset rather than measuring skipped tests.
- No general-purpose linter is configured (no `.golangci.yml`, no `golangci-lint`, no
  `.editorconfig`). **Still to be defined**: whether a more complete linter (e.g.
  `golangci-lint`) will be adopted. Until then, follow `gofmt`/`go vet` as the minimum
  baseline.
- Do not run `go build`, `go vet`, and `go test` with flags that suppress errors; CI runs
  these three checks plain, and a change should only be considered ready if all three pass
  locally.

## 8. Identified code conventions

- **Domain language**: entity, field, status, and enum names are in **English**
  (`Customer`, `Vehicle`, `ServiceOrder`, `PART`, etc.), mirroring
  [docs/entities.md](docs/entities.md). Keep this naming consistent across Go, the
  database, and documentation — do not translate part of the code back to Portuguese.
  - **No exception since 2026-08-26**: `ServiceOrder.status` values used to be the one
    deliberate carve-out, kept in Portuguese by product decision. They were renamed to
    English (`RECEBIDA` → `RECEIVED`, `EM_DIAGNOSTICO` → `IN_DIAGNOSIS`,
    `AGUARDANDO_APROVACAO` → `AWAITING_APPROVAL`, `EM_EXECUCAO` → `IN_PROGRESS`,
    `FINALIZADA` → `COMPLETED`, `ENTREGUE` → `DELIVERED`, `CANCELADA` → `CANCELED`), so
    the English convention now holds with no carve-out. This was a breaking API change and
    a Postgres enum rename — see the migration note in `docs/schema.sql`. Historical notes
    in `specs/service-order-*/` still quoting the Portuguese values are records of the
    decision as it stood when they were written, same convention as section 2's Go
    1.22→1.25 note.
  - **Changed on 2026-08-27**: the product feature's route segments were renamed from
    Portuguese to English — `/api/v1/produtos` → `/api/v1/products`,
    `.../estoque/ajustes` → `.../stock/adjustments`, `.../estoque` → `.../stock`,
    `.../movimentacoes` → `.../movements` — closing that exception. `product.ParseMovementType`
    still accepts `ENTRADA`/`SAIDA`/`SAÍDA` as input aliases for the canonical
    `ENTRY`/`EXIT` — input tolerance only, nothing Portuguese is ever stored.
  - **Still an exception**: route segments of service-order tracking/quote decision
    (`/api/v1/acompanhamento/{codigo}`, `orcamento/aprovar|reprovar`) remain in Portuguese;
    every other feature, including product as of the above change, uses English paths.
    Historical notes in `specs/product-management/` and `specs/service-order-stock-usage/`
    still quoting the old `/produtos` paths are records of the decision as it stood when
    they were written, same convention as section 2's Go 1.22→1.25 note.
- **Database** (conventions already in effect in `docs/schema.sql`):
  - `snake_case` for tables and columns.
  - `id UUID` (technical key, `gen_random_uuid()`) as PRIMARY KEY.
  - `code BIGINT GENERATED ALWAYS AS IDENTITY` as the human-readable sequential
    identifier.
  - `created_at`/`updated_at` on "registry" entities; event-trail tables
    (`service_order_history`, `audit_services`) use the event's own `occurred_at` instead.
  - Status/type enums as native Postgres `ENUM` (not `CHECK`), documented with
    `COMMENT ON`.
- **Go packages**: one `doc.go` per package with a package comment explaining its purpose
  (pattern already used throughout `internal/features/*` and `internal/shared/*`) — keep
  this pattern in new features.
- **Commits**: the history uses type-prefixed short, descriptive messages in the style
  `feat:`, `fix:`, `docs:` (see section 16).
- **API error response format (RNF04) — currently two competing implementations, not one.**
  The auth feature and the Customer Management feature were built in parallel and each
  independently established what its own spec calls "the project-wide convention":
  - `internal/shared/httpx` (`JSON`/`Error`) — `{"error": {"code", "message"}}`, used by
    `auth`'s handler and middleware. Decided in `specs/auth/design.md` §4.
  - `internal/shared/apierror` (`Write` + typed constructors `NotFound`/`Conflict`/
    `Validation`/`BadRequest`/`Internal`) — `{"error": {"code", "message", "details"?}}`,
    used by `customer`'s and `servicecatalog`'s handlers. Decided in
    `specs/customer-management/design.md` §1.5; the service catalog adopted it on
    2026-08-19 (`specs/service-catalog/design.md` §2) rather than keep a second exception,
    so `auth` is the only feature left on `httpx`.
  Both currently compile and work; this is **not a build conflict, it's an unresolved
  architecture decision** surfaced when the two branches were merged. **Do not silently
  pick one and refactor the other feature to match** — that is a cross-feature change with
  real behavioral impact (response shape, e.g. the `details` field) and belongs in an
  explicit follow-up decision, not something to infer from context. Until resolved, a new
  feature should use whichever of the two the feature it most resembles already uses, and
  flag the duplication rather than adding a third shape.
- **Authentication/middleware**: JWT (HS256) authentication is implemented —
  `internal/shared/token` issues/verifies tokens, `internal/shared/middleware.RequireAuth`
  protects routes not explicitly listed as public in `cmd/api/main.go`. See
  [specs/auth/design.md](specs/auth/design.md) for the full contract. Role/permission-based
  authorization (403) is still **to be defined** — not implemented in the MVP: a valid token
  from any user can still reach every protected route regardless of who they are. Every
  feature's routes are wrapped in `RequireAuth` as of 2026-08-26 (Customer Management and
  Service Order Opening's creation route were the last two — see section 1's note); the only
  intentional exceptions are the public routes listed in `cmd/api/main.go` (health, login)
  and Service Order Tracking's own route, which validates its own possession-based tracking
  token instead of a JWT (RF12).
- **Collection responses**: list endpoints return an `{"items": [...]}` envelope (first
  defined by the service catalog listing), leaving room for pagination metadata later.
  Reuse this shape for new list endpoints instead of returning a bare JSON array.
- **To be defined**: which error envelope (`httpx` vs. `apierror`) becomes the single
  project-wide convention (see above), general request/handler error-handling convention
  beyond the envelope itself (e.g. a centralized error-mapping helper across features),
  logging conventions beyond BR5 (never log passwords/hashes/tokens), and API versioning
  conventions.

## 9. Architectural patterns that must be respected

1. **One feature = one complete vertical slice.** Every new business feature must live in
   `internal/features/<name>/`, containing its own handler, service, repository, and model.
   Do not create cross-cutting technical packages (`internal/handlers/`, `internal/models/`,
   global `internal/services/`) — that breaks the vertical slice organization already
   adopted.
2. **No direct coupling between features.** A feature must not import internal packages of
   another feature directly. If two features need to share logic or types, that logic must
   move up to `internal/shared/`. (Both `auth` and `customer` today only import
   `internal/shared/*`, never each other.)
3. **`internal/shared/` is only for what is truly generic.** Reserve this package for
   genuinely cross-cutting code (HTTP middlewares, database client/connection, utility
   types). Do not put a specific feature's business rules there.
4. **`cmd/api/main.go` stays thin.** It is only the entrypoint: wires up the HTTP server,
   injects dependencies (e.g. database connection), and registers the routes exposed by each
   feature. Business logic, request parsing, or data access must not live in `main.go`.
5. **Schema and domain as source of truth.** The Go data model must faithfully reflect
   [docs/entities.md](docs/entities.md) and [docs/schema.sql](docs/schema.sql).

## 10. Rules for creating new features

- Every new feature is born from a specification in `specs/` (see section 17) — not
  directly from code.
- Follow the vertical slice pattern (section 9): handler, service, repository, and model
  together in `internal/features/<feature>/`.
- Reuse what already exists in `internal/shared/` before duplicating logic; if something new
  is genuinely cross-cutting, it goes into `internal/shared/`, not repeated in each feature.
  (Exception currently in effect: the error envelope — see section 8 — until that's
  explicitly resolved, don't add a third implementation either.)
- Every new route is registered from `cmd/api/main.go`, without business logic in it.
- Any new field/entity must be reflected together in
  [docs/entities.md](docs/entities.md), [docs/schema.sql](docs/schema.sql), and in the Go
  code — the three must not diverge.

## 11. Rules for tests

- Every new feature needs tests (SDD rule, section 17) — no exceptions.
- Handler/integration tests live in `internal/handlers_test/`; unit tests for a feature live
  alongside that feature's code (`*_test.go` next to the code, package
  `internal/features/<feature>/`).
- Test library choice currently differs by feature, and that's fine: Customer Management
  uses stdlib `testing` plus `testify` (`require`/`assert`), adopted deliberately (see
  `specs/customer-management/requirements.md` §8); the auth feature deliberately did **not**
  adopt `testify` (or any other test library) even after its own integration tests, using
  stdlib `testing` plus hand-written fakes instead. Don't treat either as "the" convention
  to force on the other feature; adopting a test library for a *new* feature still requires
  the explicit-alignment step from section 12.
- `go test ./...` must pass before any delivery is considered complete.

## 12. Rules for dependency management

- The module started with **no external dependency** (`go.mod` only declared `go 1.22`, no
  `go.sum`) — a deliberate initial choice, not a gap to fill automatically. Two features
  then each added their own, in parallel, both after explicit alignment (not assumed): the
  auth feature added `golang-jwt/jwt/v5`, `golang.org/x/crypto` (bcrypt), and `jackc/pgx/v5`
  (`specs/auth/design.md` §2); the Customer Management feature added `jackc/pgx/v5` (same
  dependency, versions reconciled when the branches merged), `google/uuid`, and
  `stretchr/testify` (`specs/customer-management/requirements.md` §8). The bar for adding a
  new one stays the same as before: justified need, alignment when there's a choice to make.
- Do not add a dependency (`go get`) just for convenience. Before adding any package
  (database driver, HTTP router, test library, etc.), confirm it is necessary and, if there
  is ambiguity about which one to choose, ask before deciding — do not assume the "most
  popular" library in the Go ecosystem without alignment.
- When adding a dependency, run `go mod tidy` to keep `go.mod`/`go.sum` consistent, and
  confirm that `go build ./...`, `go vet ./...`, and `go test ./...` still pass in CI. Watch
  the `go` directive — `go mod tidy` will silently raise it to the highest requirement found
  anywhere in the full transitive module graph, which can exceed what CI's pinned Go version
  can build even when the packages actually used don't need it; see section 2.

## 13. Security rules

- Secrets (database credentials, JWT signing secret, etc.) come from environment variables /
  `.env`, never hardcoded in code — `.env` is already in `.gitignore` and must not be
  versioned.
- `.env.example` documents the expected variables without real sensitive values; keep it
  up to date when introducing new variables, without putting real secrets in it.
- When implementing database access, use parameterized queries — never concatenate user
  input into SQL (SQL injection protection).
- Validate and sanitize all input received in handlers before passing it to
  services/queries.
- **Authentication is implemented**: JWT (HS256), via `internal/shared/token` +
  `internal/shared/middleware.RequireAuth` — see section 8 and
  [specs/auth/design.md](specs/auth/design.md). Passwords are hashed with bcrypt (never
  stored or logged in plain text). **Authorization (roles/permissions, HTTP 403) is still
  not implemented** — a valid token is currently sufficient to reach any protected route;
  do not assume a role/permission scheme unless it is specified in `specs/`.
- Follow general security best practices (OWASP Top 10) in any new code: avoid XSS, SQL
  injection, exposure of sensitive data in logs/errors, and do not disable security checks
  (e.g. `sslmode=disable` in production) without an explicit, documented decision. Never log
  passwords, password hashes, or tokens (BR5, `specs/auth/requirements.md`).

## 14. Database and migration rules

- The schema is defined in [docs/schema.sql](docs/schema.sql) and applied automatically by
  Postgres only on the **initial creation of the Docker volume**
  (`docker-entrypoint-initdb.d`, as `01-schema.sql`; [docs/seed.sql](docs/seed.sql) is
  mounted next to it as `02-seed.sql` and runs right after). There is no migration tool (e.g. `golang-migrate`,
  `goose`) configured in the project today. **To be defined**: incremental migration
  strategy for when the schema needs to evolve after the volume already exists in
  shared/production environments.
- To apply schema changes locally today, the flow is to recreate the volume:
  ```bash
  docker compose down -v
  docker compose up -d
  ```
- Any schema change must keep [docs/entities.md](docs/entities.md),
  [docs/schema.sql](docs/schema.sql), and [docs/seed.sql](docs/seed.sql) in sync with each
  other.
- Schema conventions already in effect (see section 8): `snake_case`, `id UUID` +
  `code BIGINT IDENTITY`, `created_at`/`updated_at` on registry entities, native Postgres
  enums documented with `COMMENT ON`. Follow this pattern for any new table.
- `docs/seed.sql` uses fixed IDs with `ON CONFLICT DO NOTHING` to be re-runnable without
  duplicating data — keep this property when adding new sample data.

## 15. Rules specific to working with Go

- Go 1.25, per `go.mod` and CI's pinned `go-version: "1.25"` — do not use syntax/features
  from newer versions, and check every dependency's own `go` directive stays ≤ 1.25 before
  `go get`/`go mod tidy` (see §2 above). Keep [Dockerfile](Dockerfile)'s Go build image
  (`golang:1.25-alpine`) in sync with whatever `go.mod` requires.
- Format with `gofmt` (the Go community standard); do not introduce alternative formatting
  styles.
- Follow the standard `cmd/` + `internal/` layout already established — non-exported
  application code goes in `internal/`.
- One package per directory, with a `go doc` (`doc.go`) describing the package's purpose,
  per the pattern already used in the project (section 8).
- Errors in Go must be handled explicitly (no silent `_ = err`) — `main.go` uses
  `log.Fatal`/`log.Fatalf` for fatal startup errors (missing config, DB connection failure);
  handler-level error mapping is implemented per feature (section 8) rather than
  centralized.
- Run `go build ./...`, `go vet ./...`, and `go test ./...` before considering any Go
  change complete (same pipeline as CI).

## 16. Git rules relevant to development

- The project history uses short commit messages with a type prefix, in the style
  `feat: ...`, `fix: ...`, `docs: ...` (see `git log`). Follow this pattern for new commits.
- Only create commits when explicitly requested; do not auto-commit as part of an unrequested
  task.
- Never commit `.env` or any file with real secrets — already protected via `.gitignore`,
  but double-check before a broad `git add`.
- CI runs on every `push` and `pull_request` (`on: [push, pull_request]`) — a change should
  only be pushed once `go build ./...`, `go vet ./...`, and `go test ./...` pass locally, so
  as not to break the pipeline.
- Avoid commits that mix unrelated scopes of change (e.g. database schema + unrelated
  business feature) — keep commits cohesive by subject, as is already the pattern in the
  current history.
- When merging two feature branches that each introduced their own cross-cutting code
  (this happened once already — auth's `shared/httpx` vs. Customer Management's
  `shared/apierror`, and two different Postgres pool constructors), resolve the mechanical
  conflict (get the build/tests green) without silently picking a winner between competing
  conventions — flag it instead (see section 8) and let it be resolved as its own explicit
  decision.

## 17. Specification-Driven Development (SDD)

This project adopts **SDD**: no feature is born directly from code. Every feature follows
the **requirements → design → implementation → tests → verification** cycle, with the
specification in `specs/` as the source of truth.

Mandatory rules:

- Do not implement a new feature without first consulting its specification in `specs/`.
- Requirements must be defined before design.
- Design must be defined before implementation.
- Implementation must be traceable back to the requirements.
- Every new feature must have tests.
- Do not invent requirements that are not defined.
- When there is ambiguity in the requirements, ask before implementing.
- Do not silently change a specification to make the code "fit."
- Before implementing, analyze the existing code and reuse patterns already adopted by the
  project.
- After implementing, verify that all requirements and acceptance criteria were met.
- Do not make changes outside the scope of the task.

The full process (the `Requirement → Design → Tasks → Implementation → Tests → Review`
flow, the `specs/<feature>/requirements.md|design.md|tasks.md` organization) is detailed in
[specs/README.md](specs/README.md). The current system architecture, documented
exclusively from the existing code, is in
[specs/architecture.md](specs/architecture.md) — keep it up to date when a new feature
changes the real architecture. Three feature folders exist today: `specs/auth/`,
`specs/customer-management/`, and `specs/service-catalog/`; a request to implement a feature
without a corresponding specification should be treated as ambiguous: ask for the
requirements before writing code.

**Open decisions inherited from merging the feature branches** (none should be resolved
silently — see section 8 for detail):
1. Whether `auth` migrates from `internal/shared/httpx` to `internal/shared/apierror`.
   `customer` and `servicecatalog` use `apierror`; `auth` (and
   `middleware.RequireAuth`'s 401) still uses `httpx`, so migrating it would change the
   401/500 bodies of the auth routes — a behavioral change that needs its own decision.
2. ~~Whether/when the Customer Management routes should be wrapped in
   `middleware.RequireAuth`~~ — **resolved 2026-08-26**: they are now wrapped, along with
   Service Order Opening's creation route, per section 1's note. What remains open is
   role/permission-based authorization (section 13): a valid token still grants access to
   every protected route regardless of who holds it.
