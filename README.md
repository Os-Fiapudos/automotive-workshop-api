# automotive-workshop-api

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
[specs/architecture.md](specs/architecture.md) for the current, code-observed architecture
and [specs/customer-management/](specs/customer-management/) for the first implemented
feature's requirements/design/tasks.

## API

The Customer Management feature is implemented and documented in
[docs/openapi.yaml](docs/openapi.yaml):

```
POST   /api/v1/customers
GET    /api/v1/customers
GET    /api/v1/customers/{id}
GET    /api/v1/customers/document/{document}
PATCH  /api/v1/customers/{id}
DELETE /api/v1/customers/{id}   (logical deactivation, not a physical delete)
```

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

To recreate the database from scratch (e.g. after changing `schema.sql`), since the script only runs on initial volume creation:

```bash
docker compose down -v
docker compose up -d
```

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

Unit tests (`internal/shared/document/*_test.go`, `internal/features/customer/*_test.go`)
run with no external dependency. Integration tests
(`internal/handlers_test/customer_test.go`) connect to a real Postgres via `DATABASE_URL`
(defaulting to the local docker-compose credentials) and **skip themselves** — they don't
fail — when that database isn't reachable, so `go test ./...` passes either way. Start
`docker compose up -d db` first to actually exercise them.

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
  n12("customer_test.go")
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
