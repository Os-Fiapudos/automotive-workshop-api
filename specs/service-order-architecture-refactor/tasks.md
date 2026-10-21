# Tasks — `service-order` Clean Architecture Refactor

Each task references the `design.md` section it implements. Tasks are ordered — do not
start a later numbered section before the previous one's suite is green (`design.md` §5).

## 1. Spec traceability check

- [x] `requirements.md` and `design.md` reviewed against the six existing
      `service-order-*` specs (`design.md` §1 file mapping) to confirm no behavior is
      dropped in the move.

## 2. Domain layer (`design.md` §5 step 1)

- [x] Create `internal/features/service-order/domain/`; move `model.go` →
      `service_order.go`/`quote.go`/`history.go` split, `execution_model.go`,
      `stockusage_model.go`, `errors.go`, and their `*_test.go` files; rename package to
      `domain`.
- [x] Update every remaining file in the old flat package to import and qualify
      `domain.*` types.
- [x] `go build ./...`, `go vet ./...`, `go test ./...` green.

## 3. Use-case layer (`design.md` §5 step 2, §2)

- [x] Create `internal/features/service-order/usecase/`; define `ports.go` (every
      interface in `design.md` §2).
- [x] Move `service.go`, `quote_service.go`, `query_service.go`, `execution_service.go`,
      `stockusage_service.go`, `metrics_service.go` into `usecase/`, split by use case per
      `design.md` §1's file list; update each to depend on the narrow ports instead of
      `serviceOrderLookups`.
- [x] Move/shrink `fake_repository_test.go` and the other `*_test.go` files accordingly —
      each test's fake implements only the ports its use case actually needs.
- [x] `go build ./...`, `go vet ./...`, `go test ./...` green.

## 4. Postgres adapter (`design.md` §5 step 3)

- [x] Create `internal/features/service-order/adapter/postgres/`; move `repository.go`,
      `quote_repository.go`, `query_repository.go`, `execution_repository.go`,
      `stockusage_repository.go`, `metrics_repository.go`.
- [x] Add `assertions.go` with one `var _ usecase.<Port> = (*PostgresServiceOrderRepository)(nil)`
      per port interface from §2.
- [x] `go build ./...`, `go vet ./...`, `go test ./...` green.

## 5. HTTP adapter (`design.md` §5 step 4, §3)

- [x] Create `internal/features/service-order/adapter/httpapi/`; split `handler.go` into
      the per-use-case handler files listed in `design.md` §1; move every `*_dto.go` and
      `httpsupport.go`.
- [x] Move `apierror` usage and `Validate()` methods out of `usecase/` into
      `adapter/httpapi/dto.go` (`design.md` §3).
- [x] Consolidate status-code mapping into `adapter/httpapi/errors.go`'s
      `writeServiceError` (`design.md` §3).
- [x] `go build ./...`, `go vet ./...`, `go test ./...` green.

## 6. Wiring (`design.md` §5 step 5)

- [x] Update `cmd/api/main.go`'s imports/constructors for the new package paths — still a
      thin entrypoint (`CLAUDE.md` §9.4), no business logic added.
- [x] `go build ./...`, `go vet ./...`, `go test ./...` green.

## 7. Layering enforcement check (`design.md` §7)

- [x] Add a check (script or `go vet`-style tool, `scripts/` or a `_test.go` using
      `go/parser`) that fails if `domain/*.go` or `usecase/*.go` imports `net/http`,
      `encoding/json`, `internal/shared/apierror`, or `github.com/jackc/pgx/*`. Wire it into
      local verification (and CI, if cheap to add without a new CI job).

## 8. Documentation (`design.md` §6)

- [x] `CLAUDE.md` §9.1 — add the Clean-Architecture-layering clause.
- [x] `specs/architecture.md` — document `service-order`'s new internal layering,
      distinguishing it from the other features' flat layout.
- [x] `internal/features/service-order/doc.go` (and each new sub-package's `doc.go`) —
      package comments describing each layer's responsibility and dependency direction.

## 9. Regression verification

- [ ] Every acceptance criterion in the six `service-order-*` specs listed in
      `requirements.md` §5 item 3, re-checked against the refactored code.
      **Unit level done**: the 157 pre-refactor tests are all present and green (plus 3
      new architecture tests); routes, JWT wrapping, JSON tags, error-to-status mapping and
      handler status codes were diffed against `afeeb1e` and are identical. **Pending**: the
      integration suite in `internal/handlers_test/` against Postgres (CI).
- [ ] `scripts/coverage.sh` against a real Postgres — `service-order` coverage no lower
      than its pre-refactor baseline.
      **Pending: CI `coverage` job.** Unit-only reference measured locally: 41.1%
      (664/1617) before, 41.3% (671/1625) after. The with-database baseline is the
      `coverage` artifact of CI on `develop` at `c6e4677`, the commit this branch starts from.
- [ ] `go build ./...`, `go vet ./...`, `go test ./...` pass, both without `DATABASE_URL`
      and against a freshly recreated local Postgres volume.
      **Done without `DATABASE_URL` (all green); pending: the run against Postgres (CI
      `coverage` job on the PR).**
- [x] ~~No file outside `internal/features/service-order/`, `CLAUDE.md`, and
      `specs/architecture.md` changed by this feature (confirm via `git diff --stat`).~~
      Superseded by `requirements.md` §7.2: outside the slice only `cmd/api`,
      `internal/handlers_test/` (composition and import paths), `scripts/coverage.sh`, the
      port move in `customer`/`vehicle`/`product`, and documentation changed.

## 10. F2-US01 additions (`requirements.md` §7, `design.md` §9)

- [x] Inventory of `auth`, `customer`, `vehicle`, `product`, `service-catalog`,
      `service-order-tracking` (file and package level) and the before/after dependency map
      in `docs/architecture-dependency-map.md` (`requirements.md` §7.1).
- [x] Move the repository port of `customer`, `vehicle`, `product` from `repository.go` to
      `service.go` (`requirements.md` §7.2) — suite green.
- [x] Status update, quote notifier and Resend adapter placed per `requirements.md` §7.3.
- [x] `scripts/coverage.sh` gates `service-order/...` as one aggregate (`requirements.md`
      §7.4); pre-refactor baseline recorded (see §9).
- [x] `architecture_test.go` checks transitive dependencies with `go list -deps`
      (`requirements.md` §7.5); verified to fail when a forbidden import is introduced.
- [x] `AGENTS.md` updated identically to `CLAUDE.md` (`requirements.md` §7.6).
- [x] Collections (`docs/postman-collection.json`, `docs/insomnia-collection.json`,
      `bruno/`) still match the routes — no route, method or body changed.
