# Tasks — Service Order Opening

Each task references the `design.md` section it implements. Implement in order — later
tasks depend on earlier ones.

## 1. Schema and docs (`design.md` §2, §3.1)

- [ ] Add `vehicle_status` enum and `vehicles.status` column (`DEFAULT 'ACTIVE'`) to
      `docs/schema.sql`, with `COMMENT ON COLUMN`.
- [ ] Add `service_order_requested_services` table + `ix_..._service_id` index to
      `docs/schema.sql`.
- [ ] Update `docs/entities.md`: `Vehicle.status` field, `ServiceOrder.requestedServices`
      field, `VehicleStatus` enum section.
- [ ] Update `docs/seed.sql`: add `status` to every `INSERT INTO vehicles`; keep at least
      one vehicle `INACTIVE` to exercise the rejection path in tests/manual verification.

## 2. Spec traceability check

- [ ] Re-read `requirements.md` and `design.md` once schema changes are drafted; confirm no
      acceptance-criteria item is left unaddressed by a design decision.

## 3. Domain layer (`design.md` §1.2, §2.3)

- [ ] `internal/features/service-order/doc.go` — package comment.
- [ ] `internal/features/service-order/model.go` — `ServiceOrder`, `Status`,
      `NewServiceOrder`.
- [ ] `internal/features/service-order/model_test.go` — always `RECEIVED`, no alternate
      constructor path.

## 4. Errors (`design.md` §1.3)

- [ ] `internal/features/service-order/errors.go` — sentinel errors listed in
      `requirements.md`/`design.md` §1.3.

## 5. Persistence (`design.md` §1.4, §3.2, §3.3, §3.4)

- [ ] `internal/features/service-order/repository.go`:
      `ServiceOrderRepository`/`serviceOrderLookups` interfaces, `customerRef`/`vehicleRef`
      structs, `PostgresServiceOrderRepository` implementing both, transactional `Create`.

## 6. Application layer (`design.md` §1.3)

- [ ] `internal/features/service-order/service.go` — `CreateInput`,
      `ServiceOrderService.Create` orchestrating resolution → validation → construction →
      persistence.
- [ ] `internal/features/service-order/service_test.go` — in-memory fake covering every
      case in `design.md` §5's unit-test bullet.

## 7. API layer (`design.md` §1.5, §4)

- [ ] `internal/features/service-order/dto.go` — `CreateRequest` (+ `Validate()`),
      `Response` (+ nested customer/vehicle/service summaries), `toResponse`.
- [ ] `internal/features/service-order/httpsupport.go` — JSON decode helper, service-error
      → HTTP status mapping (table in `design.md` §1.5).
- [ ] `internal/features/service-order/handler.go` — `RegisterRoutes`, `create` handler.

## 8. Wiring (`design.md` — implied by architecture, `CLAUDE.md` §9.4)

- [ ] `cmd/api/main.go` — construct `PostgresServiceOrderRepository`,
      `ServiceOrderService`, call `serviceorder.RegisterRoutes(router, service)`.

## 9. Integration tests (`design.md` §5)

- [ ] `internal/handlers_test/service_order_test.go` — success (by id and by
      document/plate), every error case from `design.md` §1.5's table, and the
      transactional-rollback case (`design.md` §3.4).

## 10. Verification

- [ ] `go build ./...`, `go vet ./...`, `go test ./...` pass locally.
- [ ] `docker compose down -v && docker compose up -d`, reseed, manual `curl` against
      `POST /api/v1/service-orders` for a success case and each rejection case using
      `docs/seed.sql` fixtures.
- [ ] `DATABASE_URL=... go test ./...` (integration tests actually exercised, not skipped).
- [ ] Re-check every acceptance-criteria checkbox in `requirements.md` §6 against the
      implementation before calling the feature done.
