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

- [x] Every acceptance criterion in the six `service-order-*` specs listed in
      `requirements.md` §5 item 3, re-checked against the refactored code.
      The 157 pre-refactor tests are all present and green (plus 3 architecture tests);
      routes, JWT wrapping, JSON tags, error-to-status mapping and handler status codes are
      identical to `afeeb1e`. The full integration suite passed against a fresh local
      Postgres (2026-10-21). `TestRepositoryTransitionGuardsRejectStaleOrders` was added:
      the SQL status guards (RNF07) were only reached by chance through the concurrent
      HTTP test, and are now exercised deterministically.
- [x] `scripts/coverage.sh` against a real Postgres — `service-order` coverage no lower
      than its pre-refactor baseline.
      Fresh local Postgres, same script: **before** (`c6e4677`) 85.5% (1383/1617),
      stable across 3 runs; **after** 85.9% (1396/1625). A first after-run gave 85.4%
      (1387/1625): the refactor adds 8 statements, and the SQL decision guard and the nil
      notifier default were only covered by chance. Deterministic tests now cover them, with
      no change to the gate.
- [x] `go build ./...`, `go vet ./...`, `go test ./...` pass, both without `DATABASE_URL`
      and against a freshly recreated local Postgres volume.
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
      The Postman collection (50 requests, newman 6.2.1) was run on a fresh database against
      the API image built from `c6e4677` and from this branch. All 50 status codes and
      response-body shapes are identical. `make smoke` passed on this branch's image.
      Pre-existing collection issue, out of scope: it deactivates the customer before
      creating the vehicle (`409 CUSTOMER_INACTIVE`), so its service-order requests run
      with empty ids on both versions.

## 11. Integration with `develop` (2026-10-28, `requirements.md` §7.7)

- [x] `develop@6779547` preserved as `vertical-slice-arch`. It is the regression baseline
      (decision log D13/D16).
- [x] `develop` merged into the branch (`1fe315c`). Every FP-31 change is reapplied onto the
      layers (`review-guide.md` §4.0): the same 55 SQL statements and the same
      7 × `ScanCents` + 5 × `NumericFromCents` as `develop`.
- [x] `internal/shared/money` allowed as a value-type leaf, guarded by
      `TestValueTypePackagesStayInfrastructureFree`. Verified to fail on `money → net/http`,
      `domain → encoding/json` and `usecase → domain → net/http`.
- [x] All 331 tests of `develop` (slice + `handlers_test`) present and green, plus 8 new.
- [x] Coverage, fresh Postgres, Go 1.26.10 on both sides: 82.7% (1143/1382) on
      `vertical-slice-arch` → **83.4%** (1159/1390) on the branch.
- [x] `make smoke` on the built image. Postman collection as shipped: 50/50 identical status
      and full bodies. Full-flow regression copy: 50/50 identical status; bodies identical
      except a wall-clock duration. The quote total is 309.70 on both.
- [x] `govulncheck`: 0 on both. `gosec`: 6 → 0 is an analyzer-scope artifact, not a fix
      (decision log D17).
- [ ] Push, change the base of PR #40 to `develop`, CI green on the PR (decision log A/B).
- [ ] Review, then merge with a merge commit (decision log C). Remove
      `fase-2/clean-architecture` (decision log D).
