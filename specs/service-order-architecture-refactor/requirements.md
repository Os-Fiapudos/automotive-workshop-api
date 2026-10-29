# Requirements — `service-order` Clean Architecture Refactor

Status: **Approved for implementation** — **revised 2026-10-21** for story F2-US01 (see §7;
that section takes precedence over anything earlier in this file that it contradicts).
Feature folder: `internal/features/service-order/` (internal reorganization only — no new
top-level feature folder; other features only receive the narrow, file-level fix in §7.2).

## 1. Context

Source: Fase 2 Tech Challenge (`SOAT - Fase 2 - Tech challenge.pdf` p.3, "Refatorar o código
da fase 1 aplicando: Clean Code ... Clean Architecture ou Arquitetura Hexagonal") and
`GAPS-FASE-2.md` §3.1/§5, which documents the concrete pain points in the current code:

1. `internal/features/service-order/` is one flat package with ~35 files; `handler.go` alone
   is 580 lines registering 17 routes.
2. `serviceOrderLookups` (`repository.go`) is a single 20-method interface mixing customer,
   vehicle, product, service, execution, metrics, and tracking-token lookups — every
   consumer depends on all 20 methods even when it only calls one.
3. Domain types (`model.go`) live in the same package as the SQL (`repository.go`,
   `*_repository.go`) and the HTTP layer (`handler.go`, `*_dto.go`); nothing stops a domain
   method from being written to call `pgx` or `net/http` directly, even though none does
   today.
4. `dto.go` imports `internal/shared/apierror` — an HTTP-layer concern reachable from a file
   that also defines the request/response shapes closest to the domain.

Team decisions (recorded during this repo's Fase 2 planning conversation, 2026-10-12):
**Clean Architecture** (not Hexagonal — the PDF accepts either; the team chose Clean
Architecture) is the chosen approach, and **scope is `service-order` only** for Fase 2 — the
other implemented features (`auth`, `customer`, `vehicle`, `service-catalog`, `product`)
keep their current flat-package layout and are not touched by this feature, unless a future
decision extends the refactor to them.

This feature changes **no runtime behavior** — every existing HTTP contract, status code,
and acceptance criterion from `specs/service-order-opening/`,
`specs/service-order-diagnosis-quote/`, `specs/service-order-execution/`,
`specs/service-order-query/`, `specs/service-order-quote-decision/`,
`specs/service-order-stock-usage/`, and `specs/service-order-metrics/` must continue to
hold exactly as specified. It is a structural refactor, not a new capability.

## 2. User story

> As a maintainer of `automotive-workshop-api`,
> I want `internal/features/service-order/`'s domain, use cases, and adapters (HTTP,
> Postgres) separated into their own layers with dependencies pointing inward,
> so that a change to the database or the HTTP framework cannot force a change to business
> rules, and so that the package stops growing as one 580-line file per concern.

## 3. Business rules (architectural invariants this refactor must establish)

1. The domain layer (`domain/`) has no import of `net/http`, `pgx`, `encoding/json`, or any
   other framework/infrastructure package — only the Go standard library (beyond that),
   `github.com/google/uuid`, and `time`.
2. The application/use-case layer depends only on the domain layer and on **port**
   interfaces it defines itself (not on any concrete Postgres or HTTP type).
3. Adapters (`adapter/postgres/`, `adapter/httpapi/`) depend on the domain and application
   layers, never the reverse — a domain or use-case type must never reference an adapter
   type.
4. Port interfaces are small and consumer-defined (interface segregation) — no interface in
   the refactored package exceeds roughly 5 methods grouped by a single real consumer need
   (customer lookups, vehicle lookups, product lookups, etc.), replacing the one 20-method
   `serviceOrderLookups` interface.
5. HTTP-layer concerns (`apierror`, status-code mapping, JSON (de)serialization) live
   exclusively in `adapter/httpapi/` — no domain or use-case file imports
   `internal/shared/apierror` or `net/http`.
6. This still satisfies `CLAUDE.md` §9.1 ("one feature = one complete vertical slice," no
   cross-cutting technical packages like a global `internal/handlers/`) — the layering is
   **internal** to `internal/features/service-order/`, not a return to organizing by
   technical layer across features. `CLAUDE.md` §9.1 itself is updated (§6 below) to state
   this explicitly, since its current wording ("handler, service, repository, and model
   together") predates this feature and would otherwise read as contradicting it.
7. No cross-feature import is introduced (`CLAUDE.md` §9.2 unchanged) — the package-private
   `customerRef`/`vehicleRef`/`productRef`/`serviceRef` read models and their raw-SQL
   lookups against other features' tables are preserved, just relocated (`design.md` §4).
8. Every behavior currently tested stays tested and green — this refactor is not permitted
   to reduce test coverage, change an HTTP response shape, or alter a status-transition rule
   (`requirements.md` of the six specs listed in §1 remain the acceptance bar; this
   feature's own acceptance criteria, §6, add nothing new).

## 4. Scope

In scope:
- Reorganizing `internal/features/service-order/`'s existing code into the layered package
  structure in `design.md` §1.
- Splitting `serviceOrderLookups` into narrower port interfaces.
- Moving `apierror` usage and status-code mapping out of the domain/application files into
  the HTTP adapter.
- Splitting `handler.go` by use case, mirroring the split `dto.go`/`quote_dto.go`/
  `query_dto.go`/etc. already use for request/response shapes.
- Updating `cmd/api/main.go`'s wiring for the new constructors (still a thin entrypoint,
  `CLAUDE.md` §9.4).
- Updating `CLAUDE.md` §9.1 and `specs/architecture.md` to document the new, officially
  sanctioned internal layering for this feature.

Out of scope:
- Any other feature's package (`auth`, `customer`, `vehicle`, `service-catalog`,
  `product`) — unchanged.
- Any new business capability, endpoint, or behavior change — covered by the other Fase 2
  specs (`specs/service-order-status-update/`, `specs/monetary-value-precision/`, the
  `service-order-opening`/`service-order-quote-decision` additions, etc.), not this one.
- Cross-feature deduplication (`httpsupport.go` per feature, the tracking-token header
  constant copied across packages) — `GAPS-FASE-2.md` §5 item 6 flags this, but fixing it
  would touch other features' packages, outside this refactor's `service-order`-only scope.
- Fixing `float64` for money — tracked separately (`specs/monetary-value-precision/`), so
  the two refactors don't conflict mid-flight; this feature's domain types keep `float64`
  as-is and the money-precision feature applies on top of the new layout afterward.
- Renaming the Portuguese `/acompanhamento/.../orcamento/aprovar`/`reprovar` routes — a
  separate, still-open decision (`CLAUDE.md` §8), not part of this structural refactor.

## 5. Acceptance criteria

1. `internal/features/service-order/` matches the package layout in `design.md` §1.
2. `go vet ./...` reports no import of `net/http`/`pgx`/`encoding/json` inside the domain
   package, and no import of `apierror`/`net/http` inside the application package
   (enforced by a short import-check task in `tasks.md`, since Go has no built-in
   layered-architecture linter in this project — `CLAUDE.md` §7 notes no general linter is
   configured).
3. Every acceptance criterion in `specs/service-order-opening/requirements.md`,
   `specs/service-order-diagnosis-quote/requirements.md`,
   `specs/service-order-execution/requirements.md`,
   `specs/service-order-query/requirements.md`,
   `specs/service-order-quote-decision/requirements.md`,
   `specs/service-order-stock-usage/requirements.md`, and
   `specs/service-order-metrics/requirements.md` still passes, unchanged in behavior.
4. `go build ./...`, `go vet ./...`, `go test ./...` pass at every intermediate step listed
   in `tasks.md`, not only at the end — this refactor is staged precisely so the suite never
   goes red for more than one task.
5. `scripts/coverage.sh`, run against a real Postgres, reports statement coverage for
   `service-order` no lower than it was immediately before this refactor started.
6. `CLAUDE.md` §9.1 and `specs/architecture.md` are updated to describe the new internal
   layering as a sanctioned pattern for a feature that has adopted Clean Architecture,
   without altering the vertical-slice rule itself.
7. No other feature's package (`auth`, `customer`, `vehicle`, `service-catalog`, `product`)
   has any file changed by this feature.

## 6. Open questions resolved before design

- **One PR or staged?** Staged, by layer (domain → ports/application → adapters), each
  stage keeping `go build`/`go vet`/`go test` green — see `design.md` §5 and `tasks.md`.
  Chosen over a single big-bang rewrite because a 35-file package is too large to safely
  review or bisect as one change, and `specs/README.md`'s own process ("suite verde a cada
  passo," per the team's own `GAPS-FASE-2.md` §6 step 2) already recommends this.
- **New Go package names**: `domain`, `usecase`, and `adapter/postgres`/`adapter/httpapi`
  (not `http`, which would shadow the standard library package when imported unqualified) —
  see `design.md` §1 for the full tree and import-path reasoning.

## 7. Revision 2026-10-21 — story F2-US01

Story F2-US01 (Fase 2, mandatory; requirements R01/R02 of the Fase 2 manual) asks for more
than this spec originally covered: an inventory of the other modules, an explicit decision
about the violations it finds, a coverage gate that cannot be bypassed by the new paths, and
an automated check that also covers **transitive** dependencies. The team decided the points
below on 2026-10-21, before implementation started. They are recorded here instead of
silently stretching §1–§6.

### 7.1 Inventory of the other modules (story checklist item 3)

`auth`, `customer`, `vehicle`, `product`, `service-catalog` and `service-order-tracking` are
inventoried file by file and package by package; the result, with the before/after
dependency map, lives in `docs/architecture-dependency-map.md`. Two kinds of violation are
distinguished:

- **File level** — a business-rule file (`model.go`, `service.go`, `errors.go`) importing
  infrastructure, or a port declared in an adapter file instead of in its consumer.
- **Package level** — the dependency rule evaluated per Go package, which is the only unit
  Go (and therefore an automated check) can resolve. Every flat module violates it by
  construction: its handler (`net/http`), repository (`pgx`) and domain share one package.

### 7.2 Decision: "middle ground" scope

- `service-order` is refactored completely (§1–§6, plus §7.4–§7.6).
- In the other modules, **file-level** violations are fixed. The inventory found exactly
  one kind: the repository port of `customer`, `vehicle` and `product` is declared in
  `repository.go` (the Postgres adapter) instead of in `service.go`, its only consumer. The
  declaration moves to `service.go` — same package, no behavior change. `auth`,
  `service-catalog` and `service-order-tracking` already declare their ports in the
  consumer. A `dto.go` importing `apierror` is **not** a violation: request/response shapes
  are HTTP-layer code.
- **Package-level** layering of those six modules is explicitly **out of scope for F2-US01**
  and recorded as technical debt in `specs/architecture.md` and in the dependency map. The
  original AC7 ("no other feature's package has any file changed") is replaced by: the only
  change to another feature is the port move above.

### 7.3 Files that appeared after this spec was written

`status_update_service.go` / `status_update_dto.go` (specs/service-order-status-update/),
`quote_notifier.go` and `resend_notifier.go` (specs/service-order-email-notification/) are
part of the refactor: status update → `usecase/` + `adapter/httpapi/`; the notifier port →
`usecase/`; `NoOpQuoteNotifier` and `ResendQuoteNotifier` → `adapter/notifier/` (the Resend
adapter imports `net/http` and `encoding/json`, so it can never live in `usecase/`). Their
specs join the regression bar of §5 item 3, together with the tracking protection of the
customer-facing decision routes (specs/service-order-tracking/).

### 7.4 Coverage gate (story checklist item 5)

`scripts/coverage.sh` gates `service-order` as one **aggregate** over every package under
`internal/features/service-order/...` (the feature root plus all its sub-packages), at the
inherited 80% threshold. The gate is expressed as a prefix, not a list of folders, so a
sub-package created later is included automatically. Each sub-package's own figure is
printed as information only. AC5 (no lower than the pre-refactor figure) still applies to
the aggregate.

### 7.5 Automated dependency check (story checklist item 6)

A Go test (`internal/features/service-order/architecture_test.go`) resolves the
**transitive** dependencies of `domain` and `usecase` with `go list -deps` and fails if they
reach `net/http`, `encoding/json`, `internal/shared/apierror`, `internal/shared/httpx`,
`github.com/jackc/pgx`, an `adapter/` package, or another feature's package; it also fails
if `domain` reaches `usecase`. Because it is a test, `go test ./...` runs it — including in
CI's `build` job — with no new CI job. This replaces the grep of direct imports in
`design.md` §7 and supersedes AC2's "go vet reports" wording.

### 7.6 Documentation

Besides `CLAUDE.md` §9.1 and `specs/architecture.md` (§6 AC6), `AGENTS.md` (the same
instructions for other agents) is updated identically, and the dependency map of §7.1 is
added under `docs/`.

### 7.7 Revision 2026-10-28: integration with FP-31 and the `money` value type

The team chose on 2026-10-28 to merge this refactor into `develop`. Before that,
`develop@6779547` was preserved as the long-lived branch `vertical-slice-arch`
(`docs/f2-us01-decision-log.md`). By then `develop` carried FP-31
(`specs/monetary-value-precision/`), which replaced `float64` money with
`internal/shared/money.Cents` across `service-order`. That reversed the order §4 assumed:
money precision landed first, and this refactor absorbs it.

- FP-31's changes are reapplied onto the layers, with no behavior change of their own:
  - `domain/quote.go`: `QuoteItem`/`Quote` in `money.Cents`, `PriceQuoteItems`,
    `CalculateTotal` returning `(money.Cents, error)`, `ErrQuoteAmountOutOfRange`.
  - `usecase/`: `ComposeQuote` prices the items and then sums them with bounds checks.
    `ServiceRef.Price`, `ProductRef.UnitPrice` and `QuoteNotification.TotalAmount` use
    `money.Cents`.
  - `adapter/postgres`: NUMERIC conversions through `database.ScanCents` and
    `database.NumericFromCents`.
  - `adapter/httpapi`: DTOs in `money.Cents`, and the `ErrQuoteAmountOutOfRange` → 400
    mapping.
- **BR1/BR2 exception (decision A, 2026-10-28):** `domain` and `usecase` may import
  `internal/shared/money`. It is a pure value type: it imports only the standard library,
  including `encoding/json`, because `Cents` marshals itself.
  - `architecture_test.go` treats it as a leaf, like `github.com/google/uuid`, so its own
    `encoding/json` import does not count against the inner layers.
  - `TestValueTypePackagesStayInfrastructureFree` fails if the package ever imports
    anything outside the standard library, or a networking, database or process package.
  - The rejected alternative (B) was `int64` cents in the domain with conversion in the
    adapters. It would have meant more conversion code and a larger integration diff for
    the same guarantee.
- The regression bar of §5 item 3 now includes `specs/monetary-value-precision/`. Its
  baseline is `develop@6779547`, not `c6e4677`.
