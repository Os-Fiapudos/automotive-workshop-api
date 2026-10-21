# Tasks — `service-order` Clean Architecture Refactor

Each task references the `design.md` section it implements. Tasks are ordered — do not
start a later numbered section before the previous one's suite is green (`design.md` §5).

## 1. Spec traceability check

- [ ] `requirements.md` and `design.md` reviewed against the six existing
      `service-order-*` specs (`design.md` §1 file mapping) to confirm no behavior is
      dropped in the move.

## 2. Domain layer (`design.md` §5 step 1)

- [ ] Create `internal/features/service-order/domain/`; move `model.go` →
      `service_order.go`/`quote.go`/`history.go` split, `execution_model.go`,
      `stockusage_model.go`, `errors.go`, and their `*_test.go` files; rename package to
      `domain`.
- [ ] Update every remaining file in the old flat package to import and qualify
      `domain.*` types.
- [ ] `go build ./...`, `go vet ./...`, `go test ./...` green.

## 3. Use-case layer (`design.md` §5 step 2, §2)

- [ ] Create `internal/features/service-order/usecase/`; define `ports.go` (every
      interface in `design.md` §2).
- [ ] Move `service.go`, `quote_service.go`, `query_service.go`, `execution_service.go`,
      `stockusage_service.go`, `metrics_service.go` into `usecase/`, split by use case per
      `design.md` §1's file list; update each to depend on the narrow ports instead of
      `serviceOrderLookups`.
- [ ] Move/shrink `fake_repository_test.go` and the other `*_test.go` files accordingly —
      each test's fake implements only the ports its use case actually needs.
- [ ] `go build ./...`, `go vet ./...`, `go test ./...` green.

## 4. Postgres adapter (`design.md` §5 step 3)

- [ ] Create `internal/features/service-order/adapter/postgres/`; move `repository.go`,
      `quote_repository.go`, `query_repository.go`, `execution_repository.go`,
      `stockusage_repository.go`, `metrics_repository.go`.
- [ ] Add `assertions.go` with one `var _ usecase.<Port> = (*PostgresServiceOrderRepository)(nil)`
      per port interface from §2.
- [ ] `go build ./...`, `go vet ./...`, `go test ./...` green.

## 5. HTTP adapter (`design.md` §5 step 4, §3)

- [ ] Create `internal/features/service-order/adapter/httpapi/`; split `handler.go` into
      the per-use-case handler files listed in `design.md` §1; move every `*_dto.go` and
      `httpsupport.go`.
- [ ] Move `apierror` usage and `Validate()` methods out of `usecase/` into
      `adapter/httpapi/dto.go` (`design.md` §3).
- [ ] Consolidate status-code mapping into `adapter/httpapi/errors.go`'s
      `writeServiceError` (`design.md` §3).
- [ ] `go build ./...`, `go vet ./...`, `go test ./...` green.

## 6. Wiring (`design.md` §5 step 5)

- [ ] Update `cmd/api/main.go`'s imports/constructors for the new package paths — still a
      thin entrypoint (`CLAUDE.md` §9.4), no business logic added.
- [ ] `go build ./...`, `go vet ./...`, `go test ./...` green.

## 7. Layering enforcement check (`design.md` §7)

- [ ] Add a check (script or `go vet`-style tool, `scripts/` or a `_test.go` using
      `go/parser`) that fails if `domain/*.go` or `usecase/*.go` imports `net/http`,
      `encoding/json`, `internal/shared/apierror`, or `github.com/jackc/pgx/*`. Wire it into
      local verification (and CI, if cheap to add without a new CI job).

## 8. Documentation (`design.md` §6)

- [ ] `CLAUDE.md` §9.1 — add the Clean-Architecture-layering clause.
- [ ] `specs/architecture.md` — document `service-order`'s new internal layering,
      distinguishing it from the other features' flat layout.
- [ ] `internal/features/service-order/doc.go` (and each new sub-package's `doc.go`) —
      package comments describing each layer's responsibility and dependency direction.

## 9. Regression verification

- [ ] Every acceptance criterion in the six `service-order-*` specs listed in
      `requirements.md` §5 item 3, re-checked against the refactored code.
- [ ] `scripts/coverage.sh` against a real Postgres — `service-order` coverage no lower
      than its pre-refactor baseline.
- [ ] `go build ./...`, `go vet ./...`, `go test ./...` pass, both without `DATABASE_URL`
      and against a freshly recreated local Postgres volume.
- [ ] No file outside `internal/features/service-order/`, `CLAUDE.md`, and
      `specs/architecture.md` changed by this feature (confirm via `git diff --stat`).

## 10. F2-US01 additions (`requirements.md` §7, `design.md` §9)

- [ ] Inventory of `auth`, `customer`, `vehicle`, `product`, `service-catalog`,
      `service-order-tracking` (file and package level) and the before/after dependency map
      in `docs/architecture-dependency-map.md` (`requirements.md` §7.1).
- [ ] Move the repository port of `customer`, `vehicle`, `product` from `repository.go` to
      `service.go` (`requirements.md` §7.2) — suite green.
- [ ] Status update, quote notifier and Resend adapter placed per `requirements.md` §7.3.
- [ ] `scripts/coverage.sh` gates `service-order/...` as one aggregate (`requirements.md`
      §7.4); pre-refactor baseline recorded.
- [ ] `architecture_test.go` checks transitive dependencies with `go list -deps`
      (`requirements.md` §7.5); verified to fail when a forbidden import is introduced.
- [ ] `AGENTS.md` updated identically to `CLAUDE.md` (`requirements.md` §7.6).
- [ ] Collections (`docs/postman-collection.json`, `docs/insomnia-collection.json`,
      `bruno/`) still match the routes — no route, method or body changed.
