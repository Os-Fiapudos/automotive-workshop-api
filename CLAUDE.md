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
RECEBIDA → EM_DIAGNOSTICO → AGUARDANDO_APROVACAO → EM_EXECUCAO → FINALIZADA → ENTREGUE
```

> The status values above are intentionally kept in Portuguese — see the note in section 8.

It also keeps audit trails: history of service order status changes
(`ServiceOrderHistory`) and start/end records of the execution of each service
(`AuditServices`).

The full domain model (entities, fields, enums) is documented in
[docs/entities.md](docs/entities.md); the corresponding PostgreSQL schema in
[docs/schema.sql](docs/schema.sql).

**Current state**: the project is at an early stage (skeleton). Today there is only a
minimal HTTP server with a `/health` endpoint (no framework, no real database connection)
and the folder structure for the vertical slice architecture, still empty. The database
schema and sample data are already modeled in `docs/`, even though no Go code consumes them
yet.

## 2. Technology stack

- **Language**: Go 1.25 (see [go.mod](go.mod)). Bumped from the original 1.22 when
  `pgx v5.10.0` was added for the Customer Management feature — it requires Go ≥ 1.25.
  [Dockerfile](Dockerfile)'s build image must stay at or above this version (`golang:1.25-alpine`
  today) or `docker compose build` fails with `go: go.mod requires go >= 1.25.0`.
- **External dependencies**: `github.com/jackc/pgx/v5` (Postgres driver), `github.com/google/uuid`,
  and `github.com/stretchr/testify` (test-only) — added deliberately for the Customer
  Management feature, after explicit alignment rather than assumed (see
  `specs/customer-management/requirements.md` §8). Still the only three; keep this list
  accurate here as new dependencies are added, per the "don't add a dependency without
  alignment" rule in §12 below.
- **HTTP**: stdlib `net/http`, using Go 1.22+'s method-aware `http.ServeMux` route patterns
  (e.g. `"POST /api/v1/customers"`). No third-party web framework/router — the Customer
  Management feature confirmed the stdlib mux is sufficient; do not add a router dependency
  without a concrete reason the stdlib mux can't handle.
- **Database**: PostgreSQL 16, schema versioned in [docs/schema.sql](docs/schema.sql)
  (UUID via `pgcrypto`, native enums, sequential `code` via
  `GENERATED ALWAYS AS IDENTITY`). Accessed via `pgx v5` (`pgxpool.Pool`), no ORM/query
  builder — see `internal/shared/database/` and each feature's `repository.go`.
- **Local infra**: Docker Compose with `db` (Postgres), `adminer` (DB UI), and `api`
  services ([docker-compose.yml](docker-compose.yml), [Dockerfile](Dockerfile)).
- **CI**: GitHub Actions ([.github/workflows/ci.yml](.github/workflows/ci.yml)) running
  `go build ./...`, `go vet ./...`, and `go test ./...` on every push/PR.

## 3. Project structure

```
cmd/api/main.go            → HTTP entrypoint, wires up the server and registers feature routes
internal/features/         → one folder per business feature (vertical slice)
  features/user/           → example slice: controller + service + repository + model
                              (today it only has doc.go, no implementation)
internal/shared/            → cross-cutting code reused across features (utils, types,
                              middlewares, database client, etc.) — today only has doc.go
internal/handlers_test/    → handler/integration tests (currently empty, only .gitkeep)
docs/                      → domain model (entities.md) and PostgreSQL schema
                              (schema.sql, seed.sql)
.github/workflows/ci.yml   → CI pipeline
Dockerfile, docker-compose.yml, .env.example → containerized local environment
```

specs/                      → SDD specifications: specs/README.md (process) and
                              specs/architecture.md (current architecture), with one
                              subfolder per feature once the first one is specified (see
                              section 17)

## 4. Identified architecture

**Vertical Slice (organized by feature)**: each business feature lives in its own package
under `internal/features/<feature>/`, gathering all of that feature's layers
(handler/controller, service, repository, model) together — instead of splitting by
cross-cutting technical layer (a global `handlers/` package, a global `models/` package,
etc.). This is the pattern declared in [README.md](README.md) and reflected in the
`internal/features/user/` folder.

There are no infrastructure layers implemented yet (database connection, middlewares,
authentication) — only the architectural intent and the skeleton folders.

## 5. How to run the application

Run the API locally (without a database):
```bash
go run ./cmd/api
```

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

Populate the database with sample data ([docs/seed.sql](docs/seed.sql), idempotent via
`ON CONFLICT DO NOTHING`):
```bash
docker compose cp docs/seed.sql db:/tmp/seed.sql
docker compose exec db psql -U workshop -d automotive_workshop -f /tmp/seed.sql
```

## 6. How to run the tests

```bash
go test ./...
```
No tests are implemented today (`internal/handlers_test/` only contains `.gitkeep`). This is
the command the CI runs, and any new feature must keep it passing.

## 7. Lint, static analysis, and other checks

- `go vet ./...` — run in CI, the only static analysis configured today.
- `go build ./...` — also run in CI as a compilation check.
- No additional linter is configured (no `.golangci.yml`, no `golangci-lint`, no
  `.editorconfig`). **To be defined**: whether a more complete linter (e.g.
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
  - **Deliberate exception**: `ServiceOrder.status` values (`RECEBIDA`,
    `EM_DIAGNOSTICO`, `AGUARDANDO_APROVACAO`, `EM_EXECUCAO`, `FINALIZADA`, `ENTREGUE`) are
    kept in Portuguese by explicit product decision. Every other identifier in the domain
    was translated to English; this single enum's values were not, and must not be silently
    translated in a future change.
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
  (pattern already used in `internal/features/doc.go`, `internal/features/user/doc.go`, and
  `internal/shared/doc.go`) — keep this pattern in new features.
- **Commits**: the history uses type-prefixed short, descriptive messages in the style
  `feat:`, `fix:`, `docs:` (see section 16).
- **To be defined**: error handling, logging, API error response format, and API
  versioning conventions — none of this has been implemented yet, do not assume a pattern.

## 9. Architectural patterns that must be respected

1. **One feature = one complete vertical slice.** Every new business feature must live in
   `internal/features/<name>/`, containing its own handler, service, repository, and model.
   Do not create cross-cutting technical packages (`internal/handlers/`, `internal/models/`,
   global `internal/services/`) — that breaks the vertical slice organization already
   adopted.
2. **No direct coupling between features.** A feature must not import internal packages of
   another feature directly. If two features need to share logic or types, that logic must
   move up to `internal/shared/`.
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
- Every new route is registered from `cmd/api/main.go`, without business logic in it.
- Any new field/entity must be reflected together in
  [docs/entities.md](docs/entities.md), [docs/schema.sql](docs/schema.sql), and in the Go
  code — the three must not diverge.

## 11. Rules for tests

- Every new feature needs tests (SDD rule, section 17) — no exceptions.
- Handler/integration tests live in `internal/handlers_test/`; unit tests for a feature live
  alongside that feature's code (`*_test.go` next to the code, package
  `internal/features/<feature>/`).
- Use the stdlib `testing` package plus `testify` (`require`/`assert`) — `testify` was
  adopted deliberately for the Customer Management feature (see
  `specs/customer-management/requirements.md` §8) and is now the project-wide convention;
  no other test framework has been added.
- `go test ./...` must pass before any delivery is considered complete.

## 12. Rules for dependency management

- The module started with **no external dependency** (`go.mod` only declared `go 1.22`, no
  `go.sum`) — a deliberate initial choice, not a gap to fill automatically. The Customer
  Management feature added the first three (`pgx/v5`, `google/uuid`, `stretchr/testify`,
  see §2 above), each after explicit alignment, not assumed. The bar for adding a new one
  stays the same as before: justified need, alignment when there's a choice to make.
- Do not add a dependency (`go get`) just for convenience. Before adding any package
  (database driver, HTTP router, test library, etc.), confirm it is necessary and, if there
  is ambiguity about which one to choose, ask before deciding — do not assume the "most
  popular" library in the Go ecosystem without alignment.
- When adding a dependency, run `go mod tidy` to keep `go.mod`/`go.sum` consistent, and
  confirm that `go build ./...`, `go vet ./...`, and `go test ./...` still pass in CI.

## 13. Security rules

- Secrets (database credentials, etc.) come from environment variables / `.env`, never
  hardcoded in code — `.env` is already in `.gitignore` and must not be versioned.
- `.env.example` documents the expected variables without real sensitive values; keep it
  up to date when introducing new variables, without putting real secrets in it.
- When implementing database access, use parameterized queries — never concatenate user
  input into SQL (SQL injection protection).
- Validate and sanitize all input received in handlers before passing it to
  services/queries.
- **To be defined**: the API's authentication/authorization mechanism — nothing has been
  implemented yet; do not assume a scheme (JWT, session, API key) unless it is specified in
  `specs/`.
- Follow general security best practices (OWASP Top 10) in any new code: avoid XSS, SQL
  injection, exposure of sensitive data in logs/errors, and do not disable security checks
  (e.g. `sslmode=disable` in production) without an explicit, documented decision.

## 14. Database and migration rules

- The schema is defined in [docs/schema.sql](docs/schema.sql) and applied automatically by
  Postgres only on the **initial creation of the Docker volume**
  (`docker-entrypoint-initdb.d`). There is no migration tool (e.g. `golang-migrate`,
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

- Go 1.25, per `go.mod` — do not use syntax/features from newer versions without first
  deliberately updating `go.mod` (as happened when `pgx v5.10.0` required bumping from the
  original 1.22 — see §2 above). Keep [Dockerfile](Dockerfile)'s Go build image in sync with
  whatever `go.mod` requires.
- Format with `gofmt` (the Go community standard); do not introduce alternative formatting
  styles.
- Follow the standard `cmd/` + `internal/` layout already established — non-exported
  application code goes in `internal/`.
- One package per directory, with a `go doc` (`doc.go`) describing the package's purpose,
  per the pattern already used in the project (section 8).
- Errors in Go must be handled explicitly (no silent `_ = err`) — the current `main.go`
  uses `log.Fatal` for fatal startup errors; for request/handler errors, the handling
  convention is still **to be defined** alongside the first implemented feature.
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
changes the real architecture. No feature folder exists in `specs/` yet; any request to
implement a feature without a corresponding specification should be treated as ambiguous:
ask for the requirements before writing code.
