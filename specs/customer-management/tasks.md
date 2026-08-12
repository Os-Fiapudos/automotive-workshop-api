# Tasks — Customer Management

Ordered implementation checklist. Each task references the `design.md` section it
implements. Check items off as they land; do not start a task before the ones it depends on.

## Domain & schema sync (design.md §2, §3.1)

- [x] 1. Update `docs/entities.md` — add `documentType` and `status` fields to `Customer`,
      add `CustomerStatus`/`CustomerDocumentType` enum tables.
- [x] 2. Update `docs/schema.sql` — add `customer_document_type`/`customer_status` enums and
      the two new `customers` columns (see design.md §3.1 for the exact DDL).
- [x] 3. Update `docs/seed.sql` — `customers` insert now supplies `document_type`/`status`
      and stores normalized, check-digit-valid CPF/CNPJ values.

## Dependencies (design.md §1.4, requirements.md §8)

- [x] 4. `go get github.com/jackc/pgx/v5`, `go get github.com/google/uuid`,
      `go get github.com/stretchr/testify`; `go mod tidy`. (Go itself had to be installed
      on this machine first — see design.md §0 — which bumped `go.mod`'s `go` directive
      from 1.22 to 1.25.0, a dependency-driven, documented change.)

## Shared packages (design.md §1.5)

- [x] 5. `internal/shared/document/` — `Normalize`, `DetectType`, `ValidateCPF`,
      `ValidateCNPJ`, `New(raw string) (Document, error)` + unit tests (design.md §6).
- [x] 6. `internal/shared/apierror/` — error envelope type, typed constructors
      (`NotFound`, `Conflict`, `Validation`, `BadRequest`), `Write(w, err)` helper.
- [x] 7. `internal/shared/config/` — `Load()` reading `DATABASE_URL`/`PORT` from env.
- [x] 8. `internal/shared/database/` — `NewPool(ctx, databaseURL) (*pgxpool.Pool, error)`.

## Customer feature (design.md §1.1–§1.3, §4, §5)

- [x] 9. `internal/features/customer/model.go` — `Customer`, `Document`, `Status` types,
      `NewCustomer`, `Deactivate`, `ChangeDocument` methods.
- [x] 10. `internal/features/customer/errors.go` — `ErrNotFound`, `ErrDuplicateDocument`,
      `ErrInvalidDocument`.
- [x] 11. `internal/features/customer/repository.go` — `CustomerRepository` interface +
      `PostgresCustomerRepository` (pgx-backed) implementation (design.md §3.2).
- [x] 12. `internal/features/customer/service.go` — `CustomerService` with
      `Create/Get/GetByDocument/List/Update/Deactivate`.
- [x] 13. `internal/features/customer/dto.go` — request/response DTOs (design.md §4.7).
- [x] 14. `internal/features/customer/handler.go` — one handler per endpoint + validation +
      `RegisterRoutes(mux, service)` (design.md §4, §5).

## Wiring (design.md §1.5)

- [x] 15. `cmd/api/main.go` — load config, open pool, build repository/service, build
      `*http.ServeMux`, register `/health` and `customer.RegisterRoutes`, keep `main.go`
      thin per `CLAUDE.md` §9.4.

## Tests (design.md §6)

- [x] 16. `internal/features/customer/model_test.go`, `service_test.go`
      (`fake_repository_test.go` in-memory fake).
- [x] 17. `internal/handlers_test/customer_test.go` (real HTTP + real Postgres via
      docker-compose; skips gracefully without a reachable database — verified both ways).

## Documentation

- [x] 18. `docs/openapi.yaml` — document all six endpoints, schemas, pagination, error
      envelope, examples (design.md §4).
- [x] 19. Update `README.md` — run instructions now require `DATABASE_URL`, plus new Stack/
      API/Tests sections and an updated project-structure diagram.
- [x] 20. Update `specs/architecture.md` to reflect the now-implemented customer feature,
      persistence layer, router choice, and error format (per its own "keep in sync" rule).

## Validation (requirements.md §6)

- [x] 21. `go build ./...`, `go vet ./...`, `go test ./...`, and `gofmt -l .` all pass —
      verified both with a live docker-compose Postgres (full integration suite exercised,
      plus a manual `curl` smoke test of every endpoint) and without one (skip path).
- [x] 22. Walked `requirements.md` §6 acceptance criteria one by one — see the final report
      delivered to the requester; all items are met.
