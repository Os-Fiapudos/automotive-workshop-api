# automotive-workshop-api

REST API for managing an automotive workshop's full service flow: registering customers and
their vehicles, maintaining a catalog of products (parts/supplies) and services, opening
service orders, running diagnosis and quote approval, tracking execution, and delivering the
vehicle. A service order moves through
`RECEBIDA → EM_DIAGNOSTICO → AGUARDANDO_APROVACAO → EM_EXECUCAO → FINALIZADA → ENTREGUE`
(status values are kept in Portuguese by product decision), with a full audit trail of status
changes and per-service execution timestamps kept alongside it.

```bash
export DATABASE_URL=postgres://workshop:workshop@localhost:5432/automotive_workshop?sslmode=disable
go run ./cmd/api
```

(`DATABASE_URL` is required — the API refuses to start without it. Bring up Postgres first,
e.g. with `docker compose up -d db`, or run everything via Docker Compose as below.)

## Stack

**Go REST API** — Go API with the standard cmd/ + internal/ layout (handlers/models/services).
Database access uses `pgx v5`; tests use the stdlib `testing` package plus `testify`
(`require`/`assert`) for assertions.

## Architecture

**Vertical Slice (Feature-based)** — Organized by functionality; each feature gathers its own
layers (handler, service, repository, model) in `internal/features/<feature>/`. See
[specs/architecture.md](specs/architecture.md) for the current, code-observed architecture,
[specs/customer-management/](specs/customer-management/) for the first implemented
feature's requirements/design/tasks, and [specs/vehicle-management/](specs/vehicle-management/)
for the second.

## API

Every implemented endpoint is documented in [docs/openapi.yaml](docs/openapi.yaml):

```
POST   /api/v1/auth/login
GET    /api/v1/auth/me
```

```
POST   /api/v1/customers
GET    /api/v1/customers
GET    /api/v1/customers/{id}
GET    /api/v1/customers/document/{document}
PATCH  /api/v1/customers/{id}
DELETE /api/v1/customers/{id}   (logical deactivation, not a physical delete)
```

```
POST   /api/v1/vehicles
GET    /api/v1/vehicles
GET    /api/v1/vehicles/{id}
GET    /api/v1/vehicles/plate/{plate}
GET    /api/v1/vehicles/customer/{customerId}
PATCH  /api/v1/vehicles/{id}    (brand, model, year, color only)
DELETE /api/v1/vehicles/{id}    (logical deactivation, not a physical delete)
```

```
POST   /api/v1/services
GET    /api/v1/services
GET    /api/v1/services/{id}
PATCH  /api/v1/services/{id}
DELETE /api/v1/services/{id}    (logical deactivation, not a physical delete)
```

```
POST   /api/v1/produtos
GET    /api/v1/produtos
GET    /api/v1/produtos/{id}
PATCH  /api/v1/produtos/{id}
DELETE /api/v1/produtos/{id}    (logical deactivation, not a physical delete)
POST   /api/v1/produtos/{id}/estoque/ajustes
GET    /api/v1/produtos/{id}/estoque
GET    /api/v1/produtos/{id}/movimentacoes
```

```
POST   /api/v1/service-orders                                     (unauthenticated)
GET    /api/v1/service-orders
GET    /api/v1/service-orders/{id}                                 (id or sequential code)
POST   /api/v1/service-orders/{id}/diagnosis
PUT    /api/v1/service-orders/{id}/quote
GET    /api/v1/service-orders/{id}/quote
POST   /api/v1/service-orders/{id}/quote/send
POST   /api/v1/service-orders/{id}/executions
POST   /api/v1/service-orders/{id}/executions/{executionId}/finish
POST   /api/v1/service-orders/{id}/finalize
POST   /api/v1/service-orders/{id}/deliver
```

```
GET    /api/v1/acompanhamento/{codigo}                             (unauthenticated, tracking token)
POST   /api/v1/acompanhamento/{codigo}/orcamento/aprovar            (unauthenticated, tracking token)
POST   /api/v1/acompanhamento/{codigo}/orcamento/reprovar           (unauthenticated, tracking token)
```

Except for the routes marked otherwise above (login itself, Customer Management, service order
creation, and the customer-facing `/acompanhamento` tracking routes), every route requires a
JWT (`Authorization: Bearer <token>`, obtained from `POST /api/v1/auth/login`) — see
[specs/auth/](specs/auth/) and [specs/vehicle-management/](specs/vehicle-management/) for the
authentication contract, and `docs/openapi.yaml`'s `bearerAuth`/`trackingToken` security
schemes for the exact per-endpoint requirement.

### API documentation (Swagger)

[docs/openapi.yaml](docs/openapi.yaml) is the OpenAPI 3.0 contract for every implemented
endpoint. Browse it as Swagger UI locally via the `swagger-ui` service started by Docker
Compose below:

- **Swagger UI**: http://localhost:8082

## Database

The data model is documented in [docs/entities.md](docs/entities.md) and the corresponding PostgreSQL schema (tables, enums, indexes, and comments) in [docs/schema.sql](docs/schema.sql).

Bring up the database (Postgres + Adminer + API) with Docker Compose:

```bash
cp .env.example .env
docker compose up -d
```

- **Postgres**: `localhost:5432` (credentials in `.env`), with `docs/schema.sql` applied automatically on first startup.
- **Adminer**: http://localhost:8081 — system `PostgreSQL`, server `db`, user/password/database as in `.env`.
- **API**: http://localhost:8080/health
- **Swagger UI**: http://localhost:8082

### Migrations

There is no separate migration tool (e.g. `golang-migrate`, `goose`) in this project today.
[docs/schema.sql](docs/schema.sql) is the single source of truth for the database schema, and
Postgres applies it automatically — via `docker-entrypoint-initdb.d` — only on the **initial
creation** of the `db_data` volume. To apply a schema change (a new/edited table, column, or
enum in `schema.sql`), recreate the volume so it re-runs the init script from scratch:

```bash
docker compose down -v
docker compose up -d
```

This discards any data in the local Postgres volume; reload sample data afterwards with the
seed command below if needed.

### Sample data (seed)

[docs/seed.sql](docs/seed.sql) populates the database with customers, vehicles, products, services, and service orders covering the main statuses of the flow. It uses fixed IDs with `ON CONFLICT DO NOTHING`, so it can be re-run without duplicating data.

Apply it by copying the file into the container before running (avoids UTF-8 encoding issues that occur when using pipe/redirection from stdin on PowerShell):

```bash
docker compose cp docs/seed.sql db:/tmp/seed.sql
docker compose exec db psql -U workshop -d automotive_workshop -f /tmp/seed.sql
```

Works the same way on PowerShell, Bash, Git Bash, macOS, and Linux.

## Tests

```bash
go test ./...
```

Unit tests (`internal/shared/*/*_test.go`, `internal/features/*/*_test.go`) run with no
external dependency. Integration tests (`internal/handlers_test/`) connect to a real
Postgres via `DATABASE_URL` (defaulting to the local docker-compose credentials) and
**skip themselves** — they don't fail — when that database isn't reachable, so
`go test ./...` passes either way.

To actually exercise them, the database needs **both** `docs/schema.sql` and
`docs/seed.sql` applied — the seed is not optional here, because it creates the
administrative user the authentication tests log in as (schema alone produces 74
failures):

```bash
docker compose up -d db
docker compose cp docs/schema.sql db:/tmp/schema.sql
docker compose cp docs/seed.sql   db:/tmp/seed.sql
docker compose exec db psql -U workshop -d automotive_workshop -f /tmp/schema.sql
docker compose exec db psql -U workshop -d automotive_workshop -f /tmp/seed.sql

export DATABASE_URL='postgres://workshop:workshop@localhost:5432/automotive_workshop?sslmode=disable'
export JWT_SECRET=dev-secret
go test ./...
```

### Coverage

```bash
scripts/coverage.sh                    # measure, print the table, fail below 80%
COVERAGE_HTML=1 scripts/coverage.sh    # also write coverage/coverage.html
```

Enforces RNF06: at least 80% statement coverage on the critical domains
(`service-order`, `product`, `service-order-tracking`), and prints every other package for
information. Requires the same `DATABASE_URL` and `JWT_SECRET` as the integration tests and
**refuses to run without them** — coverage measured over skipped tests would report roughly
a third of the real figure. CI runs it on every push
([.github/workflows/ci.yml](.github/workflows/ci.yml), job `coverage`).

### Security scan

```bash
scripts/security-scan.sh
```

Runs `govulncheck` (dependency and standard-library vulnerabilities) and `gosec` (SAST),
both pinned to exact versions and executed via `go run <module>@<version>`, so nothing is
added to `go.mod`. Output lands in `security/` (git-ignored) and is published as a CI
artifact by the `security` job. The findings, their severity, and what was fixed are in
[docs/security-report.md](docs/security-report.md).

## Project structure

```mermaid
flowchart TD
  n0["automotive-workshop-api/"]
  n1["cmd/"]
  n0 --> n1
  n2["api/"]
  n1 --> n2
  n3("main.go")
  n2 --> n3
  n4["internal/"]
  n0 --> n4
  n5["features/"]
  n4 --> n5
  n6["user/"]
  n5 --> n6
  n7("doc.go")
  n6 --> n7
  n30["customer/"]
  n5 --> n30
  n31("model.go, dto.go, repository.go, service.go, handler.go, errors.go, doc.go")
  n30 --> n31
  n40["vehicle/"]
  n5 --> n40
  n41("model.go, plate.go, dto.go, repository.go, service.go, handler.go, httpsupport.go, errors.go, doc.go")
  n40 --> n41
  n8("doc.go")
  n5 --> n8
  n9["shared/"]
  n4 --> n9
  n32["document/"]
  n9 --> n32
  n33("document.go, cpf.go, cnpj.go, doc.go")
  n32 --> n33
  n34["apierror/"]
  n9 --> n34
  n35["config/"]
  n9 --> n35
  n36["database/"]
  n9 --> n36
  n10("doc.go")
  n9 --> n10
  n11["handlers_test/"]
  n4 --> n11
  n12("customer_test.go, vehicle_test.go")
  n11 --> n12
  n21["docs/"]
  n0 --> n21
  n22("entities.md")
  n21 --> n22
  n23("schema.sql")
  n21 --> n23
  n26("seed.sql")
  n21 --> n26
  n37("openapi.yaml")
  n21 --> n37
  n27["specs/"]
  n0 --> n27
  n28("README.md")
  n27 --> n28
  n29("architecture.md")
  n27 --> n29
  n38["customer-management/"]
  n27 --> n38
  n39("requirements.md, design.md, tasks.md")
  n38 --> n39
  n42["vehicle-management/"]
  n27 --> n42
  n43("requirements.md, design.md, tasks.md")
  n42 --> n43
  n13("go.mod")
  n0 --> n13
  n14(".gitignore")
  n0 --> n14
  n24(".env.example")
  n0 --> n24
  n15("README.md")
  n0 --> n15
  n16("Dockerfile")
  n0 --> n16
  n25("docker-compose.yml")
  n0 --> n25
  n17[".github/"]
  n0 --> n17
  n18["workflows/"]
  n17 --> n18
  n19("ci.yml")
  n18 --> n19
  n20("LICENSE")
  n0 --> n20
```
