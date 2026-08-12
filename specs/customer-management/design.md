# Design — Customer Management

Satisfies: `requirements.md` (all sections). This is the first feature implemented in the
project, so this document also fixes several architecture-wide conventions (routing,
persistence, error format) that `CLAUDE.md` leaves as "To be defined." Those choices are
called out explicitly below and must be mirrored in `specs/architecture.md` once
implemented.

## 0. Go toolchain note

This project had no Go toolchain installed locally before this feature (see `tasks.md`); Go
1.26.5 was installed via Homebrew to build/test it. That toolchain builds `go.mod`'s
declared `go` version fine either way — the real constraint is CI
(`.github/workflows/ci.yml` pins `go-version: "1.22"`) and `Dockerfile`
(`golang:1.22-alpine`), which only have Go 1.22 available.

Initially, adding `pgx v5.10.0` (`go get`) required Go ≥ 1.25, and Go bumped `go.mod`'s `go`
directive to `1.25.0` automatically — this passed locally (newer toolchain installed) and in
a manually-tested Docker build (after temporarily bumping the Dockerfile's image), but broke
both **CI** and anyone building the real `Dockerfile` image, which are pinned to 1.22 and
were not going to be changed just to accommodate a dependency. The fix: `pgx` was pinned
back to `v5.7.4` — the newest release whose own `go.mod` still requires only Go 1.21 — and
its transitive `golang.org/x/sync`/`golang.org/x/text` dependencies were likewise pinned to
the newest versions that still require ≤ Go 1.22 (`v0.11.0` and `v0.22.0` respectively;
newer patch releases of both jumped to requiring Go 1.23+). `go.mod`'s `go` directive is
back at `1.22.0`, matching CI/`Dockerfile` again, with no code in this feature relying on
any 1.23+ language feature. Per `CLAUDE.md` §15, this whole back-and-forth is recorded here
explicitly rather than left as a silent version churn — future dependency upgrades must
check the dependency's own (and its transitive dependencies') `go` directive against 1.22
*before* running `go get`, not after CI fails.

## 1. Architecture decisions

### 1.1 Slice granularity: one package per feature, not one per use case

`CLAUDE.md` §4 and §9.1 already establish the adopted pattern — observable in
`internal/features/user/` — as **one Go package per business feature**, gathering
handler, service, repository, and model together. The task that originated this feature
sketched a finer-grained alternative (`Clientes/Criar/`, `Clientes/Consultar/`, ... each
with its own `endpoint/dto/validator/usecase/persistence`), but explicitly left the final
structure to be defined here, warning against blindly copying that sketch.

**Decision**: keep the single-package-per-feature convention already adopted by the
project (`internal/features/customer/`), organized internally by responsibility file
rather than by use case:

```
internal/features/customer/
├── doc.go              → package comment
├── model.go             → Customer domain type + invariants (status transitions, etc.)
├── dto.go                → request/response DTOs for the HTTP contract
├── repository.go        → CustomerRepository interface + Postgres implementation
├── service.go            → CustomerService: one method per use case (Create, Get, ...)
├── handler.go             → http.HandlerFunc per endpoint + RegisterRoutes(mux, service)
├── httpsupport.go         → JSON decode/encode, path/query parsing, service-error → HTTP mapping
├── errors.go              → feature-level sentinel errors (ErrNotFound, ErrDuplicateDocument)
├── model_test.go
├── service_test.go
```

Rationale: four use cases (create/get/list/update/deactivate) sharing one model and one
repository do not justify five parallel folder trees, each re-declaring its own DTOs and
persistence code (`CLAUDE.md` §26 explicitly warns against this and against generic/
premature abstractions). A single package keeps the feature's four use cases cohesive and
easy to navigate, while still being one Go package = one clear unit of ownership, which is
what the already-adopted convention requires. If a future feature turns out to need
genuinely different persistence/transport per use case, that can be revisited then — this
decision is not retroactively binding on other features.

Within `service.go`, `CustomerService` exposes one method per use case
(`Create`, `Get`, `GetByDocument`, `List`, `Update`, `Deactivate`); there is no
CQRS-style split into separate command/query types — a single service struct is enough
complexity for this scope (`CLAUDE.md` §6, avoid Clean/Hexagonal/CQRS "just because").

`handler.go` and `httpsupport.go` split the HTTP layer along a different axis than
"one file per endpoint": `handler.go` holds only the six route methods, each reading as
parse input → call the service → write the response; `httpsupport.go` holds the repeated
plumbing every method needs (JSON body decoding, UUID/query-param parsing, and the
service-error → HTTP-status mapping) so it isn't duplicated across methods or left mixed
into route-handling code. This split was added after the initial implementation, once the
duplication (`uuid.Parse(r.PathValue("id"))` three times, `json.NewDecoder(...).Decode(...)`
twice) became visible in the actual code — not designed upfront.

### 1.2 Domain layer

- The `Customer` struct (`model.go`) is the aggregate: id, code, name, `Document` value
  object, phone, e-mail, status, timestamps.
- `Document` is a value object (`{ Value string; Type DocumentType }`) built only through a
  constructor that normalizes and validates — it is impossible to hold an invalid or
  unnormalized `Document` in memory.
- Status transition (`Deactivate()`) is a method on `Customer`, not a bare field mutation,
  so the "no implicit reactivation" rule (`requirements.md` §3.7) lives in the domain type
  itself rather than being re-implemented by callers.
- CPF/CNPJ normalization and check-digit validation are **not** customer-specific — they are
  generic Brazilian tax-document algorithms. They live in `internal/shared/document/`
  (`CLAUDE.md` §9.3: shared is for genuinely cross-cutting code). `model.go` calls into
  `shared/document` when constructing/changing a `Document`; it does not duplicate the
  algorithm.

### 1.3 Application layer

- `CustomerService` (`service.go`) depends on the `CustomerRepository` interface (defined in
  the same package, next to its only consumer — no separate "repository contracts" package).
- Handlers call the service directly; there is no separate command/query dispatcher.
- DTOs (`dto.go`) are distinct Go types from the domain `Customer`, so the HTTP contract can
  evolve independently of the domain model. The handler is the only place that converts
  DTO ⇄ domain.

### 1.4 Persistence

- `CustomerRepository` is implemented against `pgx v5` (`pgxpool.Pool`), injected into
  `CustomerService` from `cmd/api/main.go`.
- The Postgres `document` unique index already exists
  (`ux_customers_document` in `docs/schema.sql`) and remains the final authority on
  uniqueness. The service performs a pre-check (`ExistsByDocument`) to return a clean `409`
  in the common case, but **also** maps the Postgres unique-violation error
  (`SQLSTATE 23505`) to the same domain error, so a race between two concurrent requests is
  still caught correctly (`requirements.md` §3.4 cannot rely on `ExistsByDocument` alone —
  see task background, "the application must not depend only on `existsByDocumento()`").
- `docs/schema.sql` also has a **pre-existing** partial unique index on `email`
  (`ux_customers_email`, present since before this feature) — `requirements.md` §3.4.1.
  `SQLSTATE 23505` alone does not say *which* unique index was violated, so
  `PostgresCustomerRepository` inspects `pgconn.PgError.ConstraintName` and maps
  `ux_customers_document` → `ErrDuplicateDocument`, `ux_customers_email` →
  `ErrDuplicateEmail` — two distinct domain errors, so the API response names the actual
  offending field instead of always blaming the document. There is no `ExistsByEmail`
  pre-check mirroring `ExistsByDocument`: the database constraint is the only guard for
  email, which is enough since a 409 either way still reports a specific, actionable error —
  adding a second pre-check purely for symmetry with document wouldn't fix anything the
  constraint doesn't already fix. (This gap — email uniqueness enforced by the database but
  not reflected in the application's error mapping — was found after implementation, via a
  real request that returned `DUPLICATE_DOCUMENT` for what was actually a duplicate email;
  see `requirements.md` §3.4.1 for the corrected requirement.)
- No migration tool is introduced. Per `CLAUDE.md` §14, schema changes are still applied by
  editing `docs/schema.sql` directly and recreating the Docker volume
  (`docker compose down -v && docker compose up -d`) — this feature adds two columns and
  two enums to `customers` (see §2 below), following that existing flow. An incremental
  migration tool remains "To be defined" for when the schema must evolve after a shared/
  production volume already holds data (unchanged from `CLAUDE.md`).

### 1.5 API layer

- **Router**: Go 1.22's standard-library `http.ServeMux` already supports method-aware
  patterns (`"POST /api/v1/customers"`, `"GET /api/v1/customers/{id}"`), which is exactly
  what this feature needs. No third-party router/framework is introduced — this resolves
  `CLAUDE.md`'s "To be defined" router question for this feature without adding a
  dependency. `cmd/api/main.go` builds one `*http.ServeMux`; each feature exposes
  `RegisterRoutes(mux *http.ServeMux, ...)` and registers itself on it.
- **Response envelope**: success responses return the resource (or
  `{ data: [...], page, pageSize, total, totalPages }` for the paginated list) as JSON,
  `Content-Type: application/json`, no extra envelope around a single resource.
- **Error envelope** (`internal/shared/apierror/`, new cross-cutting package): every error
  response has the shape

  ```json
  {
    "error": {
      "code": "VALIDATION_ERROR",
      "message": "human-readable summary",
      "details": [ { "field": "document", "message": "invalid CPF check digits" } ]
    }
  }
  ```

  `details` is omitted when there is nothing field-specific to report (e.g. `404`, `409`).
- **HTTP status mapping** (decided here, since `requirements.md`/`CLAUDE.md` leave 400 vs.
  422 open):

  | Situation | Status | `error.code` |
  | --- | --- | --- |
  | Malformed JSON body | 400 | `INVALID_BODY` |
  | Missing/invalid field, invalid CPF/CNPJ check digits | 400 | `VALIDATION_ERROR` |
  | Customer not found (by id or document) | 404 | `NOT_FOUND` |
  | Document already belongs to another customer | 409 | `DUPLICATE_DOCUMENT` |
  | E-mail already belongs to another customer | 409 | `DUPLICATE_EMAIL` |

  **Decision: 400, not 422**, for every validation failure (structural or business, e.g. bad
  check digits). Rationale: this is a plain `net/http` API with no framework that
  distinguishes "syntactically valid but semantically unprocessable" (422) from "invalid
  input" (400) out of the box; introducing that distinction here would add a rule with no
  client benefit yet. A single validation status keeps the contract simple, consistent with
  `CLAUDE.md` §26's "avoid overengineering." This becomes the project-wide convention going
  forward — future features should reuse `internal/shared/apierror`, not invent a second
  error shape or reopen the 400 vs. 422 question.
- **Pagination**: query params `page` (default `1`, min `1`) and `pageSize` (default `20`,
  max `100`, values outside range clamped rather than rejected — pagination is a display
  aid, not a business rule worth failing a request over). Parsing stays local to
  `customer/handler.go` for now; it is not extracted into `internal/shared/` yet because
  there is only one caller — extracting it prematurely for a single feature would be the
  kind of speculative abstraction `CLAUDE.md` §26 warns against. Extract to `shared` the
  moment a second feature needs the same parsing.
- **New cross-cutting packages introduced by this feature** (all justified as genuinely
  reusable by any future feature, per `CLAUDE.md` §9.3):
  - `internal/shared/document/` — CPF/CNPJ normalize/validate/detect-type.
  - `internal/shared/apierror/` — error envelope type + `http.ResponseWriter` helper.
  - `internal/shared/config/` — reads `DATABASE_URL`/`PORT` from the environment.
  - `internal/shared/database/` — builds the `pgxpool.Pool` from a connection string.

## 2. Domain model

### 2.1 Customer (updated)

`docs/entities.md`'s `Customer` entity does not yet have a status or an explicit document
type field, both of which this feature's requirements need (`requirements.md` §3.5–§3.8 and
the "Tipo do documento" field called out in the originating task). Per `CLAUDE.md` §10,
these are added to `docs/entities.md`, `docs/schema.sql`, and the Go code together:

| Field | Type | Description |
| --- | --- | --- |
| id | uuid | Unchanged. |
| code | number | Unchanged. |
| name | string | Unchanged. |
| document | string | Unchanged, but now always stored normalized: CPF is 11 digits; CNPJ is 14 characters, digits and/or uppercase letters A–Z (its last 2 characters are always digits) — see §5.2. |
| documentType | string | **New.** `CPF` or `CNPJ`, derived from the document at write time. |
| phone | string | Unchanged. |
| email | string? | Unchanged (already optional in the schema). |
| status | string | **New.** `ACTIVE` or `INACTIVE`. Starts `ACTIVE`; see rule §3.6–§3.7. |
| createdAt | string | Unchanged. |
| updatedAt | string | Unchanged. |

`documentType` is technically derivable from `document`'s length/prefix rules, but it is
stored explicitly (set once, at write time, by the same validation step that already knows
the type) because it is part of what the domain — and the API response — needs to expose
without re-deriving it on every read; this mirrors how every other classification in the
schema (`product_type`, `quote_status`, ...) is a stored enum, not a computed value.

### 2.2 Invariants

- A `Customer` cannot exist with an invalid or unnormalized document — enforced by
  `Document`'s constructor, called from every path that sets/changes a customer's document
  (create, update).
- A `Customer` always starts `ACTIVE`; there is no constructor path that creates one
  `INACTIVE`.
- `Deactivate()` is idempotent-safe at the domain level (calling it twice does not error),
  but the service layer still surfaces a `404` if the target customer does not exist at all;
  it does not surface an error for "already inactive" — deactivating twice is a no-op, not a
  failure (no requirement asks for that to be an error, and treating it as one would be an
  invented requirement).
- There is no `Activate()` method on `Customer` in this feature (`requirements.md` §3.7).

## 3. Persistence design

### 3.1 Schema changes (`docs/schema.sql`)

```sql
DO $$ BEGIN
    CREATE TYPE customer_document_type AS ENUM ('CPF', 'CNPJ');
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

DO $$ BEGIN
    CREATE TYPE customer_status AS ENUM ('ACTIVE', 'INACTIVE');
EXCEPTION WHEN duplicate_object THEN NULL; END $$;
```

plus two new columns on the `customers` table: `document_type customer_document_type NOT
NULL` and `status customer_status NOT NULL DEFAULT 'ACTIVE'`.

`docs/schema.sql` is only ever applied against a brand-new Docker volume
(`docker-entrypoint-initdb.d`, per `CLAUDE.md` §14) — no shared/production volume exists
yet. So, unlike a real incremental migration, this change is made directly in the
`CREATE TABLE customers` statement itself (and in `docs/seed.sql`'s `INSERT`, which must now
supply `document_type`), not as a separate `ALTER TABLE`. Anyone with a local volume created
before this change picks it up the normal way already documented in `CLAUDE.md` §5/§14:
`docker compose down -v && docker compose up -d`. An incremental migration tool remains
"To be defined" for once a shared/production volume actually holds data.

The existing `ux_customers_document` unique index already satisfies
`requirements.md` §3.4 at the database level; no new index is required for uniqueness.

### 3.2 Repository interface

```go
type CustomerRepository interface {
    Create(ctx context.Context, c *Customer) error
    FindByID(ctx context.Context, id uuid.UUID) (*Customer, error)
    FindByDocument(ctx context.Context, document string) (*Customer, error)
    ExistsByDocument(ctx context.Context, document string, excludeID *uuid.UUID) (bool, error)
    List(ctx context.Context, page, pageSize int) ([]*Customer, int, error)
    Update(ctx context.Context, c *Customer) error
}
```

- `ExistsByDocument` takes an optional `excludeID` so an update can check "does this
  document belong to *another* customer" without tripping on the customer's own row.
- `Update` persists the full row (name, document, document_type, phone, email, status); the
  service is responsible for merging the partial PATCH input into the loaded `Customer`
  before calling `Update` — the repository has no notion of "partial."
- `List` returns the page of rows plus the total row count (for `totalPages`), backed by a
  single query using `COUNT(*) OVER()` rather than two round trips.

## 4. API contract

Base path: `/api/v1/customers`. All request/response bodies are JSON.

### 4.1 `POST /api/v1/customers`

Request:
```json
{ "name": "Maria Silva", "document": "123.456.789-09", "phone": "+55 11 91234-5678", "email": "maria@example.com" }
```
- `name`, `document`, `phone` required; `email` optional.
- `document` normalized then validated (CPF or CNPJ, by length after normalization).
- `201 Created`, `Location: /api/v1/customers/{id}`, body = full customer (status `ACTIVE`).
- `400` — missing field or invalid document. `409` — duplicate document.

### 4.2 `GET /api/v1/customers`

Query params: `page`, `pageSize` (see §1.5). Includes active and inactive customers (no
implicit filter — `requirements.md` §3.8).
- `200 OK`, body = `{ "data": [Customer...], "page", "pageSize", "total", "totalPages" }`.

### 4.3 `GET /api/v1/customers/{id}`

- `200 OK`, body = `Customer`. `404` if not found.

### 4.4 `GET /api/v1/customers/document/{document}`

- `{document}` is normalized (punctuation stripped) before lookup, so both
  `.../document/12345678909` and a formatted path segment resolve the same customer.
- `200 OK`, body = `Customer`. `404` if not found.

### 4.5 `PATCH /api/v1/customers/{id}`

Request (all fields optional, partial update — `requirements.md` §3.9):
```json
{ "name": "Maria Silva Santos", "document": "...", "phone": "...", "email": "..." }
```
- Only present fields are changed. If `document` is present, it is normalized, validated,
  and re-checked for uniqueness against every other customer.
- `200 OK`, body = updated `Customer`. `400` invalid document. `404` not found.
  `409` duplicate document.

### 4.6 `DELETE /api/v1/customers/{id}`

- Logical deactivation, not physical delete (`requirements.md` §3.6).
- `200 OK`, body = updated `Customer` (status `INACTIVE`). `404` if not found.
- Idempotent: deactivating an already-inactive customer returns `200` with no change made.

### 4.7 `Customer` response shape

```json
{
  "id": "b7b1...uuid",
  "code": 1042,
  "name": "Maria Silva",
  "document": "12345678909",
  "documentType": "CPF",
  "phone": "+55 11 91234-5678",
  "email": "maria@example.com",
  "status": "ACTIVE",
  "createdAt": "2026-08-11T12:00:00Z",
  "updatedAt": "2026-08-11T12:00:00Z"
}
```

## 5. Validation

Implemented in `internal/shared/document/` and used by `internal/features/customer`.

### 5.1 CPF

Always 11 numeric digits — unaffected by the CNPJ change below. `ValidateCPF` checks the
length, that every character is a digit, both official check digits (modulo-11 algorithm),
and rejects the well-known all-same-digit invalid CPFs (e.g. `00000000000`) that pass the
check-digit math trivially.

### 5.2 CNPJ — numeric and alphanumeric

Receita Federal's Instrução Normativa RFB nº 2.229/2024 replaces the purely numeric CNPJ
with an **alphanumeric** format, in effect since July 2026: positions 1–12 (the "root" +
branch order) may now be digits or uppercase letters A–Z; positions 13–14 (the check
digits) remain numeric, always. Pre-existing numeric CNPJs stay valid and are not reissued
— so `ValidateCNPJ` must accept both the legacy all-numeric form and the new alphanumeric
form, not just one of them.

- Each character (digit or uppercase letter) is converted to a numeric value for the
  check-digit calculation via `char code − 48`: digits keep their value (`'0'`→0 … `'9'`→9);
  letters map to 17–42 (`'A'`→17 … `'Z'`→42). This is the official conversion rule, not
  something invented here.
- The check-digit weights are unchanged from the legacy algorithm
  (`5,4,3,2,9,8,7,6,5,4,3,2` for the first digit; `6,5,4,3,2,9,8,7,6,5,4,3,2` — the first 12
  converted values plus the first check digit — for the second), modulo 11, same
  `remainder < 2 → 0` rule.
- `ValidateCNPJ` still rejects a CNPJ whose last two characters are not both digits (the
  spec never allows letters there) and the all-same-character degenerate case.

### 5.3 Normalization

- `Normalize(raw string) string` — strips formatting characters (spaces, `.`, `/`, `-`) and
  **uppercases** any letters it keeps, in addition to keeping digits. CPF input never
  legitimately contains a letter (an uppercased stray letter simply fails `ValidateCPF`'s
  digits-only check below), so this one function safely serves both document types without
  the caller needing to know which one it's normalizing yet.
- `DetectType(normalized string) (DocumentType, error)` — `CPF` for 11 characters, `CNPJ`
  for 14 characters; error for any other length. Length alone decides the type, exactly as
  before — only what counts as a "valid character" inside a CNPJ changed.
- A single exported `document.New(raw string) (Document, error)` composes
  normalize → detect type → validate into one call, which is what `model.go` uses — no
  regex-only shortcut, per `requirements.md` §3.3.

Field-level request validation (required fields present, non-empty) is a
`Validate() []apierror.Detail` method on `CreateRequest` itself (`dto.go`), called by
`customer/handler.go` before invoking the service, producing `VALIDATION_ERROR` / `400`
with per-field `details`. Keeping `Validate()` on the request type — rather than as
free-standing checks inline in the handler — mirrors the "validation lives with the data
class" convention from annotation-based validators in other ecosystems (e.g. Java's Bean
Validation), without needing struct tags or a validation library: it is still plain,
explicit Go code, just colocated with the type it validates instead of the function that
calls it.

## 6. Testing strategy

- **Unit tests**, stdlib `testing` + `testify` (`require`/`assert`):
  - `internal/shared/document/*_test.go`: table-driven tests for valid/invalid/formatted/
    normalized CPF and CNPJ, including the all-same-digit edge case.
  - `internal/features/customer/model_test.go`: customer starts `ACTIVE`;
    `Deactivate()` transitions and is idempotent; no `Activate()` exists.
  - `internal/features/customer/service_test.go`: an in-memory fake implementing
    `CustomerRepository` (a plain map-backed struct in the test file — no mocking
    framework) drives create/get/list/update/deactivate, duplicate document, and
    not-found cases.
- **Integration tests**, `internal/handlers_test/customer_test.go`:
  - Build the real `*http.ServeMux` wired to a real `pgxpool.Pool` pointed at
    `DATABASE_URL` (falls back to `.env`'s default local Postgres if unset); `t.Skip` the
    whole file if the database is unreachable, so `go test ./...` still passes in
    environments without Docker Compose running (CI already brings up the stack — see
    `tasks.md`).
  - Drive the six endpoints over real HTTP (`httptest.NewServer`), asserting status codes
    and bodies: full CRUD flow, invalid CPF, invalid CNPJ, duplicate document on create and
    on update, pagination, inactive customer still queryable, deactivate is not a physical
    delete, unique index is what ultimately rejects a concurrent duplicate.
  - Each test creates customers with randomly generated valid CPFs/CNPJs (own local
    generator, not `shared/document`, to avoid a test suite that only proves the generator
    matches the validator) and cleans up its own rows after running, keeping tests
    independent without needing per-test transactions/truncation.

## 7. Traceability

Every decision above satisfies a specific `requirements.md` item; `tasks.md` breaks this
design into ordered implementation steps, each referencing the section here it implements.
