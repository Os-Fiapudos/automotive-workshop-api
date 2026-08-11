# Current architecture — automotive-workshop-api

This document describes the architecture **actually present in the code** of this
repository as of this date. It does not describe the desired/planned architecture beyond
what is already implemented or explicitly declared as a convention in the code/README
itself. Where the code does not allow something to be determined, it is marked as
**"To be defined"** instead of assumed.

This document must be updated whenever a new feature (via `specs/<feature>/`) changes the
real architecture of the system.

## 1. Architecture overview

The system is, today, a single Go binary (`cmd/api`) that starts an HTTP server using only
the standard library (`net/http`), with no web framework. It exposes a single real
endpoint, `GET /health`, implemented inline in `cmd/api/main.go`.

The folder organization follows the **cmd/ + internal/** pattern, with the declared
architectural intent of **vertical slice (organization by feature)**: each business
feature is meant to gather handler, service, repository, and model in a single package
under `internal/features/<feature>/`. This intent is reflected in the folder structure
(`internal/features/user/`), but **no business feature is implemented yet** — the `user`
package contains only a `doc.go` with the package comment, with no functional code.

There is, therefore, no real communication between components, no connected persistence
layer, and no business error handling implemented — the architecture observable in code
today amounts to the HTTP entrypoint and the health-check endpoint.

## 2. Main components

| Component | Path | State |
| --- | --- | --- |
| HTTP entrypoint | `cmd/api/main.go` | Implemented — registers `/health` and starts the server on port `:8080`. |
| `user` feature | `internal/features/user/` | Not implemented — only `doc.go` declaring the package. |
| `features` package (root) | `internal/features/doc.go` | Not implemented — only a package comment. |
| Shared code | `internal/shared/` | Not implemented — only `doc.go` declaring the package. |
| Handler tests | `internal/handlers_test/` | Not implemented — the directory only contains `.gitkeep`. |
| Database schema | `docs/schema.sql` | Implemented as plain SQL; not consumed by any Go code yet. |
| Sample data | `docs/seed.sql` | Implemented as plain SQL; applied manually via `psql`, not by the Go code. |
| Domain model | `docs/entities.md` | Domain documentation (Customer, Vehicle, Product, Service, ServiceOrder, Quote, ServiceOrderHistory, AuditServices), with no corresponding Go code yet. |
| Local environment | `docker-compose.yml`, `Dockerfile` | Implemented — orchestrates `db` (Postgres), `adminer`, and `api`. |
| CI | `.github/workflows/ci.yml` | Implemented — runs `go build ./...`, `go vet ./...`, `go test ./...`. |

## 3. Responsibilities

- **`cmd/api/main.go`**: the process's single entrypoint. Today it concentrates everything
  that exists: the `/health` handler definition and HTTP server startup. The code itself
  contains the comment `// TODO: register the handlers of the chosen architecture in
  internal/`, confirming that `main.go`'s intended responsibility is only to
  orchestrate/register handlers defined in `internal/`, not to contain business logic — but
  this has not been exercised by any real feature yet.
- **`internal/features/<feature>/`**: the intended responsibility (by folder convention and
  by the package comment in `internal/features/user/doc.go`: *"Feature 'user': controller,
  service, repository, and model together"*) is to gather all the layers of a business
  feature. No concrete responsibility is implemented.
- **`internal/shared/`**: the intended responsibility (by the package comment) is to gather
  code reused across features (utils, types). No concrete content exists.
- Responsibilities of specific layers (handler vs. controller, service, repository, model)
  within a feature: **To be defined** — there is no concrete implementation in the code to
  observe how these layers are actually split in this project.

## 4. Flow of the main operations

The only observable flow in the code is:

```
HTTP client → GET /health → inline handler in main.go → json.Encode({"status":"ok"}) → HTTP response
```

No other operation flow is implemented (customer, vehicle, product, service registration,
service orders, quotes, etc.). `docs/entities.md` describes the **data model** of these
entities and a service order status flow
(`RECEBIDA → EM_DIAGNOSTICO → AGUARDANDO_APROVACAO → EM_EXECUCAO → FINALIZADA → ENTREGUE`),
but that is domain documentation, not an implemented operation flow in code — the
transition between these statuses has no associated Go logic yet. **To be defined** once
these features are specified and implemented.

## 5. Communication between components

Today there is no communication between Go components, because there is only one component
with executable code (`main.go`) — no `internal/features/*` or `internal/shared` package is
imported by `main.go` or by any other `.go` file in the repository.

The intended communication pattern between layers within a feature (e.g.
handler → service → repository) and between features (via `internal/shared`, without
direct import between feature packages) is described as an architectural convention in
[CLAUDE.md](../CLAUDE.md), but **it is not observable in the code today** — it is a rule to
be followed once implementation starts, not a pattern already in use. Marked here as
**"To be defined"** in terms of *observed* architecture, so as not to confuse intended
convention with an already-implemented fact.

## 6. Persistence

No persistence is implemented in Go. No database driver, ORM, or query builder is present
in `go.mod` (which declares no external dependency), and no `.go` file in the repository
opens a connection to Postgres or runs a query.

What exists related to persistence:
- **Schema**: [docs/schema.sql](../docs/schema.sql) defines the tables
  (`customers`, `vehicles`, `products`, `services`, `service_orders`, `quotes`,
  `quote_products`/`quote_services` — as defined in the file —, `service_order_history`,
  `audit_services`), native Postgres enums, and is applied automatically by the `db`
  container only on initial Docker volume creation (Postgres's
  `docker-entrypoint-initdb.d` mechanism), not by application code.
- **Seed**: [docs/seed.sql](../docs/seed.sql) populates sample data via manual `psql`,
  also outside the Go code.
- **Connection string**: `DATABASE_URL` is defined as an environment variable of the `api`
  service in `docker-compose.yml`, but **no Go code reads that variable today** — it is
  available in the container's environment, with no implemented consumer.
- **Driver/ORM/repository pattern**: **To be defined** — no choice has been made in code
  (see also `CLAUDE.md`, technology stack section).

## 7. External integrations

No external integration (third-party APIs, message queues, e-mail/SMS services, payment
gateways, etc.) is present in the code or in `go.mod`. `docs/entities.md` mentions e-mail
notification as the *purpose* of the Customer's `email` field ("used for notifications and
communication"), but no notification-sending mechanism is implemented. **To be defined**
should any external integration be specified in the future.

## 8. Error handling

The only observable error handling in the code:
- `main.go` uses `log.Fatal(http.ListenAndServe(":8080", nil))` — if the server fails to
  start (e.g. port in use), the process is terminated with the cause logged. This is the
  only fatal error-handling point of the process.
- The `/health` handler calls `json.NewEncoder(w).Encode(...)` without checking the
  returned error — there is no handling (not even logging) for a potential
  encoding/response-write failure in this handler, as the code stands today.

There is no:
- API error format convention (e.g. a standard JSON error envelope).
- Centralized error-handling middleware.
- Mapping of domain/database errors to HTTP status codes.

All of this is **"To be defined"** — it must be decided and documented as part of the
first feature specification that needs to return a business error.

## 9. Testing strategy

- CI ([.github/workflows/ci.yml](../.github/workflows/ci.yml)) runs `go test ./...` on
  every push/PR, but **no `_test.go` file exists in the repository today**. The only sign
  of testing intent is the `internal/handlers_test/` directory, which contains only a
  `.gitkeep`.
- There is no test library beyond the stdlib `testing` package (no dependency declared in
  `go.mod`).
- Strategy (unit vs. integration vs. end-to-end), use of mocks/fakes for the database,
  coverage targets, and whether `internal/handlers_test/` will hold integration tests via
  real HTTP or package tests (`httptest`): **To be defined** — none of this is implemented
  or declared in code.
- The convention for where tests should live (unit alongside the feature, integration in
  `internal/handlers_test/`) is described as a rule in [CLAUDE.md](../CLAUDE.md), but there
  is still no real test to confirm how that convention materializes in practice.

## 10. Identified architectural decisions

Decisions that **are actually observable** in the repository's code/configuration, with
their source:

1. **Organization by feature (vertical slice)**, not by global technical layer — declared
   in [README.md](../README.md) ("Vertical Slice (Feature-based)") and reflected in the
   `internal/features/user/` folder.
2. **Plain Go stdlib for HTTP**, no framework/router — `go.mod` declares no external
   dependency, and `main.go` uses `net/http` directly.
3. **PostgreSQL as the database**, with a schema-first design in plain SQL
   ([docs/schema.sql](../docs/schema.sql)), not generated from Go code/an ORM.
4. **UUID technical key + `code` sequential identifier** as the pattern on every registry
   table — an explicit decision documented in the header of `docs/schema.sql`.
5. **Status/type enums as native Postgres `ENUM`**, not `CHECK` — with the rationale
   recorded in `docs/schema.sql` itself ("more compact on disk/index and
   self-documenting").
6. **Containerized local environment** via Docker Compose with three services (`db`,
   `adminer`, `api`) — [docker-compose.yml](../docker-compose.yml).
7. **Minimum CI gate**: every change goes through `go build ./...`, `go vet ./...`, and
   `go test ./...` on GitHub Actions on every push/PR —
   [.github/workflows/ci.yml](../.github/workflows/ci.yml).
8. **Domain identifiers in English**, consistently between `docs/entities.md` and
   `docs/schema.sql` — with a single deliberate exception: `ServiceOrder.status` enum
   values are kept in Portuguese (`RECEBIDA`, `EM_DIAGNOSTICO`, `AGUARDANDO_APROVACAO`,
   `EM_EXECUCAO`, `FINALIZADA`, `ENTREGUE`) by explicit product decision, documented in
   `docs/entities.md` and in the `service_order_status` enum comment in
   `docs/schema.sql`.

Any architectural decision outside this list (HTTP framework, database driver,
authentication, error format, migration strategy, testing strategy, external integrations)
**has not been made yet in code** and should be treated as **"To be defined"** until
resolved by a specification in `specs/<feature>/design.md`.
