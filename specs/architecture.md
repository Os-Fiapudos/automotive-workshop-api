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
third-party web framework/router. It exposes `GET /health` plus the six Customer Management
endpoints under `/api/v1/customers` (the project's first implemented business feature — see
`specs/customer-management/`).

The folder organization follows the **cmd/ + internal/** pattern, with **vertical slice
(organization by feature)** as the adopted convention: each business feature gathers
handler, service, repository, and model in a single package under
`internal/features/<feature>/`. `internal/features/customer/` is the first feature to
actually implement this (organized internally by responsibility file — `model.go`,
`dto.go`, `repository.go`, `service.go`, `handler.go`, `errors.go` — rather than by use
case; see `specs/customer-management/design.md` §1.1 for the rationale).
`internal/features/user/` remains an empty placeholder (`doc.go` only).

Cross-cutting code now lives in `internal/shared/`: `document` (CPF/CNPJ
normalize/validate), `apierror` (the project-wide JSON error envelope), `config`
(environment-variable loading), and `database` (the shared `pgx` connection pool).

Communication is now real for the customer feature: `main.go` → `customer.RegisterRoutes`
→ `handler` → `CustomerService` → `CustomerRepository` (Postgres, via `pgx`) → the
`customers` table.

## 2. Main components

| Component | Path | State |
| --- | --- | --- |
| HTTP entrypoint | `cmd/api/main.go` | Implemented — loads config, opens the Postgres pool, registers `/health` and the customer routes, starts the server. |
| `customer` feature | `internal/features/customer/` | Implemented — model, DTOs, Postgres repository, service, HTTP handlers for all 6 endpoints. See `specs/customer-management/`. |
| `user` feature | `internal/features/user/` | Not implemented — only `doc.go` declaring the package (placeholder for a future feature). |
| `features` package (root) | `internal/features/doc.go` | Not implemented — only a package comment. |
| Shared: `document` | `internal/shared/document/` | Implemented — CPF/CNPJ normalize/detect-type/validate (check-digit algorithm, no third-party library). |
| Shared: `apierror` | `internal/shared/apierror/` | Implemented — the project-wide JSON error envelope and HTTP status mapping. |
| Shared: `config` | `internal/shared/config/` | Implemented — reads `DATABASE_URL`/`PORT` from the environment. |
| Shared: `database` | `internal/shared/database/` | Implemented — builds/pings the shared `pgxpool.Pool`. |
| Handler tests | `internal/handlers_test/` | Implemented — `customer_test.go`, real HTTP against a real Postgres, skips (not fails) without `DATABASE_URL`/a reachable database. |
| Database schema | `docs/schema.sql` | Implemented as plain SQL; now consumed by the `customer` feature's repository (customers table, including the two new columns/enums added for this feature). |
| Sample data | `docs/seed.sql` | Implemented as plain SQL; applied manually via `psql`, not by the Go code. Customer rows now include `document_type`/`status` and store normalized documents. |
| Domain model | `docs/entities.md` | Domain documentation for all entities; `Customer` is the first to have a corresponding Go implementation. |
| API documentation | `docs/openapi.yaml` | Implemented — documents the 6 customer endpoints, schemas, pagination, and the error envelope. |
| Local environment | `docker-compose.yml`, `Dockerfile` | Implemented — orchestrates `db` (Postgres), `adminer`, and `api`. |
| CI | `.github/workflows/ci.yml` | Implemented — runs `go build ./...`, `go vet ./...`, `go test ./...`. |

## 3. Responsibilities

- **`cmd/api/main.go`**: the process's single entrypoint. Loads config
  (`internal/shared/config`), opens the Postgres pool (`internal/shared/database`), builds
  the `customer` repository/service, builds one `*http.ServeMux`, registers `/health`
  inline and delegates every `/api/v1/customers*` route to
  `customer.RegisterRoutes`. It stays thin — no business logic, request parsing, or data
  access lives in `main.go` itself, per `CLAUDE.md` §9.4.
- **`internal/features/<feature>/`**: one Go package per feature, gathering its own
  handler, service, repository, and model. `internal/features/customer/` is the concrete
  implementation of this pattern (see `specs/customer-management/design.md` §1.1 for why it
  is organized by responsibility file rather than by use case). Within it:
  - `handler.go` — HTTP layer: request parsing/validation, DTO ⇄ domain conversion, status
    code mapping. Depends on `service.go`.
  - `service.go` — one method per use case, orchestrates domain + repository. Depends on
    the `CustomerRepository` interface (not the concrete Postgres type).
  - `repository.go` — the `CustomerRepository` interface and its `pgx`-backed
    implementation. Depends on `model.go` and `internal/shared/document`.
  - `model.go` — the `Customer` aggregate and its invariants (always starts `ACTIVE`,
    document only settable through validated construction, no `Activate` method).
  - `dto.go` — HTTP request/response shapes, independent of the domain type.
- **`internal/shared/`**: genuinely cross-cutting code only (`CLAUDE.md` §9.3), used by
  `customer` today and available to every future feature: `document` (CPF/CNPJ),
  `apierror` (HTTP error envelope), `config` (env var loading), `database` (pgx pool).

## 4. Flow of the main operations

```
HTTP client → GET /health → inline handler in main.go → json.Encode({"status":"ok"}) → HTTP response

HTTP client → POST/GET/PATCH/DELETE /api/v1/customers... (net/http ServeMux)
            → customer.handler (parses/validates request, maps errors to the shared envelope)
            → customer.CustomerService (business rules: starts ACTIVE, no reactivation, partial update, ...)
            → customer.CustomerRepository → pgx → Postgres `customers` table
            → customer.handler (DTO response) → HTTP response
```

No other feature's operation flow is implemented (vehicle, product, service registration,
service orders, quotes, etc.). `docs/entities.md` describes the **data model** of these
entities and a service order status flow
(`RECEBIDA → EM_DIAGNOSTICO → AGUARDANDO_APROVACAO → EM_EXECUCAO → FINALIZADA → ENTREGUE`),
but that remains domain documentation only — no Go logic implements it yet. **To be
defined** once those features are specified and implemented. Note the documented **future**
invariant (not yet implemented, since Service Order does not exist): opening a service
order must reject an `INACTIVE` customer (see `specs/customer-management/requirements.md`
§7.1).

## 5. Communication between components

For the customer feature, communication now matches the convention declared in
`CLAUDE.md`: handler → service → repository within the feature, with no other feature
importing `internal/features/customer` directly (none exists yet to do so) and no
feature-specific logic placed in `internal/shared/` beyond the genuinely generic packages
listed in §3. `cmd/api/main.go` is the only place that wires concrete types together
(`PostgresCustomerRepository` → `CustomerService` → `RegisterRoutes`); `customer.handler`
and `customer.service.go` depend only on the `CustomerRepository` interface, not on `pgx`
directly.

## 6. Persistence

Implemented for the `customer` feature via `pgx v5` (`pgxpool.Pool`):
- **Driver**: `github.com/jackc/pgx/v5` + `pgxpool`, no `database/sql` wrapper, no ORM/query
  builder — chosen over `database/sql`+`lib/pq` (`lib/pq` is in maintenance mode); see
  `specs/customer-management/requirements.md` §8.
- **Connection**: `internal/shared/database.NewPool` builds and pings the pool from
  `config.Load()`'s `DATABASE_URL` (still the same env var declared in
  `docker-compose.yml`, now actually consumed by Go code).
- **Schema**: [docs/schema.sql](../docs/schema.sql) — unchanged mechanism (applied only on
  initial Docker volume creation), but the `customers` table now has two additional columns
  (`document_type customer_document_type`, `status customer_status`) and two new enums,
  added directly in the `CREATE TABLE` (no separate migration tool exists yet — see
  `specs/customer-management/design.md` §1.4/§3.1).
- **Repository pattern**: one `CustomerRepository` interface per feature (not a generic
  repository), implemented against Postgres. The `ux_customers_document` unique index is
  the final authority on document uniqueness; the repository maps a `23505` unique-violation
  `pgconn.PgError` to `customer.ErrDuplicateDocument` in addition to the service's
  application-level pre-check.
- **Seed**: [docs/seed.sql](../docs/seed.sql) — still applied manually via `psql`, outside
  Go code; customer rows now include `document_type`/`status` and normalized documents.

## 7. External integrations

No external integration (third-party APIs, message queues, e-mail/SMS services, payment
gateways, etc.) is present in the code or in `go.mod`. `docs/entities.md` mentions e-mail
notification as the *purpose* of the Customer's `email` field ("used for notifications and
communication"), but no notification-sending mechanism is implemented. **To be defined**
should any external integration be specified in the future.

## 8. Error handling

Implemented project-wide via `internal/shared/apierror`:
- A single JSON envelope, `{ "error": { "code", "message", "details"? } }`, written by
  `apierror.Write`.
- HTTP status mapping fixed by `specs/customer-management/design.md` §1.5: `400` for a
  malformed body or any validation failure (structural or business — **400 was chosen over
  422**, for simplicity, and is now the project-wide convention), `404` not found, `409`
  duplicate document.
- `customer/handler.go`'s `writeServiceError` maps the feature's sentinel errors
  (`ErrNotFound`, `ErrDuplicateDocument`, `ErrInvalidDocument`) to that envelope; anything
  else (e.g. an unexpected database error) becomes a generic `500` via `apierror.Internal`,
  never leaking internal error text to the client.
- `main.go` still uses `log.Fatal` for startup failures (config missing, pool unreachable,
  `ListenAndServe` failing) — unchanged from before.

Future features should reuse `internal/shared/apierror` rather than inventing a second
error shape (see `specs/customer-management/design.md` §1.5).

## 9. Testing strategy

Implemented for the `customer` feature, and intended as the project-wide pattern going
forward:
- **Test library**: stdlib `testing` + `testify` (`require`/`assert`) — `testify` is now
  the project's first external test dependency, added deliberately (see
  `specs/customer-management/requirements.md` §8), not assumed.
- **Unit tests**, colocated with the code: `internal/shared/document/*_test.go` (CPF/CNPJ
  table-driven cases) and `internal/features/customer/{model,service}_test.go`. The service
  tests use a hand-written in-memory `fakeRepository` (in `fake_repository_test.go`)
  implementing `CustomerRepository` — no mocking framework.
- **Integration tests**, in `internal/handlers_test/customer_test.go`: build the real
  `*http.ServeMux` wired to a real `pgxpool.Pool` pointed at `DATABASE_URL` (defaulting to
  the local docker-compose credentials), drive the six endpoints over real HTTP via
  `httptest.NewServer`, and physically clean up their own rows afterward. Every test
  **skips** (via `t.Skip`, not a failure) when the database is unreachable, so
  `go test ./...` still passes without Docker Compose running — confirmed both with and
  without a live database during implementation.
- CI ([.github/workflows/ci.yml](../.github/workflows/ci.yml)) still only runs
  `go build/vet/test ./...` with no Postgres service configured, so the integration tests
  currently run in **skip mode** in CI; provisioning a Postgres service for CI to exercise
  them for real remains **To be defined**.

## 10. Identified architectural decisions

Decisions that **are actually observable** in the repository's code/configuration, with
their source:

1. **Organization by feature (vertical slice)**, not by global technical layer — declared
   in [README.md](../README.md) ("Vertical Slice (Feature-based)") and implemented in
   `internal/features/customer/`.
2. **Plain Go stdlib for HTTP**, no framework/router — Go 1.22+'s method-aware
   `http.ServeMux` patterns (`"POST /api/v1/customers"`, `.../{id}`) are used directly;
   confirmed sufficient by the first real feature, so no router dependency was added.
3. **PostgreSQL as the database**, with a schema-first design in plain SQL
   ([docs/schema.sql](../docs/schema.sql)), accessed via `pgx v5` (no ORM).
4. **UUID technical key + `code` sequential identifier** as the pattern on every registry
   table — an explicit decision documented in the header of `docs/schema.sql`, now actually
   read/written by `internal/features/customer/repository.go`.
5. **Status/type enums as native Postgres `ENUM`**, not `CHECK` — with the rationale
   recorded in `docs/schema.sql` itself ("more compact on disk/index and
   self-documenting"); `customer_document_type` and `customer_status` follow the same
   pattern.
6. **Containerized local environment** via Docker Compose with three services (`db`,
   `adminer`, `api`) — [docker-compose.yml](../docker-compose.yml).
7. **Minimum CI gate**: every change goes through `go build ./...`, `go vet ./...`, and
   `go test ./...` on GitHub Actions on every push/PR —
   [.github/workflows/ci.yml](../.github/workflows/ci.yml).
8. **Domain identifiers in English**, consistently between `docs/entities.md` and
   `docs/schema.sql` — with a single deliberate exception: `ServiceOrder.status` enum
   values are kept in Portuguese (`RECEBIDA`, `EM_DIAGNOSTICO`, `AGUARDANDO_APROVACAO`,
   `EM_EXECUCAO`, `FINALIZADA`, `ENTREGUE`) by explicit product decision. The customer
   feature's own new fields (`documentType`, `status`) and its routes (`/customers`, not
   `/clientes`) deliberately follow this English convention even though the task that
   originated the feature was written in Portuguese — see
   `specs/customer-management/requirements.md` §5.
9. **Single project-wide JSON error envelope** (`internal/shared/apierror`), `400` for all
   validation failures (never `422`) — established by the customer feature, intended for
   reuse by every future feature (see §8 above).
10. **`pgx v5` as the Postgres driver, `testify` as the test assertion library** — both
    added deliberately for the customer feature after explicit alignment (not assumed), per
    `CLAUDE.md` §12; see `specs/customer-management/requirements.md` §8.

Any architectural decision outside this list (authentication, migration strategy beyond
"edit `schema.sql` + recreate the volume," CI database provisioning) **has not been made
yet in code** and should be treated as **"To be defined"** until resolved by a
specification in `specs/<feature>/design.md`.
