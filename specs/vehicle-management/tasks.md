# Tasks — Vehicle Management

Ordered implementation checklist. Each task references the `design.md` section it
implements. Check items off as they land; do not start a task before the ones it depends on.

> **Note**: during implementation, the customer-scoped listing route changed from the
> originally-specified `GET /api/v1/customers/{customerId}/vehicles` to
> `GET /api/v1/vehicles/customer/{customerId}` — the former panics Go's `http.ServeMux` at
> startup (genuinely ambiguous against customer's pre-existing `.../customers/document/{document}`
> route). See `requirements.md` §3.1 and `design.md` §1.5 for the full account.

## Domain & schema sync (design.md §2, §3.1)

- [x] 1. Update `docs/entities.md` — add `status` field to `Vehicle`, add a `VehicleStatus`
      enum table (mirrors `CustomerStatus`).
- [x] 2. Update `docs/schema.sql` — add the `vehicle_status` enum and the new `vehicles.status`
      column (see design.md §3.1 for the exact DDL).
- [x] 3. Update `docs/seed.sql` — `vehicles` insert now supplies `status` explicitly.

## Vehicle feature (design.md §1.1–§1.3, §2, §4, §5)

- [x] 4. `internal/features/vehicle/doc.go` — package comment.
- [x] 5. `internal/features/vehicle/plate.go` — `Normalize`, `Validate` (legacy + Mercosul) +
      unit tests (design.md §1.2, §6).
- [x] 6. `internal/features/vehicle/model.go` — `Vehicle`, `Status` types, `NewVehicle`,
      `Deactivate` methods (design.md §2).
- [x] 7. `internal/features/vehicle/errors.go` — `ErrNotFound`, `ErrDuplicatePlate`,
      `ErrInvalidPlate`, `ErrCustomerNotFound`, `ErrCustomerInactive`.
- [x] 8. `internal/features/vehicle/repository.go` — `VehicleRepository` interface +
      `PostgresVehicleRepository` (pgx-backed) implementation (design.md §3.2).
- [x] 9. `internal/features/vehicle/service.go` — `VehicleService` with
      `Create/Get/GetByPlate/List/ListByCustomer/Update/Deactivate`, plus the `CustomerLookup`
      interface it declares (design.md §1.3).
- [x] 10. `internal/features/vehicle/dto.go` — request/response DTOs (design.md §4.8).
- [x] 11. `internal/features/vehicle/handler.go` + `httpsupport.go` — one handler per
      endpoint + validation + `RegisterRoutes(mux, service)` (design.md §4, §5).

## Wiring (design.md §1.3, §1.5)

- [x] 12. `cmd/api/main.go` — build `vehicle.NewPostgresVehicleRepository`/
      `vehicle.NewVehicleService`, the `customerLookupAdapter` wrapping the existing
      `*customer.CustomerService` (design.md §1.3), register all seven vehicle routes wrapped
      in the existing `requireAuth` middleware (design.md §1.5), keep `main.go` thin per
      `CLAUDE.md` §9.4.

## Tests (design.md §6)

- [x] 13. `internal/features/vehicle/plate_test.go`, `model_test.go`, `service_test.go`
      (`fake_repository_test.go` in-memory fake + fake `CustomerLookup`).
- [x] 14. `internal/handlers_test/vehicle_test.go` (real HTTP + real Postgres via
      docker-compose; skips gracefully without a reachable database; asserts `401` without a
      token on every route).

## Documentation

- [x] 15. `docs/openapi.yaml` — document all seven endpoints, schemas, pagination, error
      envelope, `bearerAuth` security scheme, examples (design.md §4).
- [x] 16. Update `README.md` — new Vehicle Management endpoints under the existing API
      section, updated project-structure diagram.
- [x] 17. Update `specs/architecture.md` to reflect the now-implemented vehicle feature.

## Validation (requirements.md §7)

- [x] 18. `go build ./...`, `go vet ./...`, `go test ./...`, and `gofmt -l .` all pass —
      verified both with a live docker-compose Postgres (full integration suite exercised,
      plus a manual `curl` smoke test of every endpoint, including the `401` case) and without
      one (skip path).
- [ ] 19. Walk `requirements.md` §7 acceptance criteria one by one and report the status of
      each, per the requester's requested final step.
