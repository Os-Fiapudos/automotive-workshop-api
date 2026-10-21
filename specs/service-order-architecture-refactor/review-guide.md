# Review guide — F2-US01 `service-order` Clean Architecture refactor

This file is for reviewers, human or AI. It holds the context a diff alone does not carry:
what must not change, what changed on purpose, where the risk is, how to verify, and what
not to flag. Read it before the diff.

**For AI-assisted review**, give the model this file, the three spec files next to it
(`requirements.md` §7 takes precedence over §1–§6, `design.md` §9 over §1–§7), and the diff
`git diff c6e4677...chore/F2-US01-clean-architecture-service-order`. Prompts are in §8.

## 1. What this change is

It is a structural refactor of `internal/features/service-order/` (story F2-US01, Fase 2
R01/R02) into `domain/`, `usecase/` and `adapter/{postgres,httpapi,notifier}/`, with
dependencies pointing inward. It is **not** a feature change: no route, status code, JSON
field, error code, SQL statement or business rule is supposed to change.

- Base: `c6e4677` (the `develop` commit this work started from). The PR targets
  `fase-2/clean-architecture`, a long-lived Fase 2 line created at that same commit and
  never merged into `develop`.
- One commit per step (the PR description lists them). Each refactor step builds and passes tests on its
  own: spec → domain → usecase → postgres → httpapi+notifier → architecture test →
  coverage gate → other modules → docs → guard tests → evidence. Reviewing commit by commit
  is easier than reviewing the squashed diff.
- Size: 104 files before this guide, roughly +4.7k/−3.1k lines. Most of it is moves: git
  detects 36 renames, with similarity between 50% and 99%.

## 2. Invariants — must hold, check them

| # | Invariant | Where to check |
| --- | --- | --- |
| I1 | `domain` imports only the standard library and `github.com/google/uuid` | `domain/*.go` imports; `architecture_test.go` |
| I2 | `usecase` imports only `domain`, the standard library, `uuid`, `internal/shared/document`, `internal/shared/trackingtoken`; never `net/http`, `encoding/json`, `pgx`, `apierror`, `httpx` or any `adapter/` | `usecase/*.go`; `architecture_test.go` |
| I3 | No adapter imports another adapter or another feature's package | `architecture_test.go` (`TestAdaptersDoNotDependOnEachOtherOrOnOtherFeatures`) |
| I4 | Ports are declared by their consumer (`usecase/ports.go`), each ≤ 4 methods; the 20-method `serviceOrderLookups` is gone | `usecase/ports.go`; `adapter/postgres/assertions.go` |
| I5 | Every SQL statement is byte-for-byte unchanged | `git diff -M c6e4677 -- '**/adapter/postgres/*.go'`: only package and type names, exported method names and comments should differ |
| I6 | The 19 routes, their methods and their JWT wrapping are unchanged; the customer-facing `/acompanhamento/...` routes and the two external-secret routes stay **unwrapped** | `adapter/httpapi/router.go` against old `handler.go` `RegisterRoutes` |
| I7 | Error → HTTP status/code mapping is unchanged | `adapter/httpapi/errors.go` against old `httpsupport.go` `writeServiceError` |
| I8 | DTO JSON tags are unchanged | `adapter/httpapi/*_dto.go` (renamed files, ≥78% similar) |
| I9 | Domain transition guards are unchanged: RECEIVED→IN_DIAGNOSIS→AWAITING_APPROVAL→IN_PROGRESS/CANCELED→COMPLETED→DELIVERED | `domain/service_order.go`; the methods were only exported (`startDiagnosis` → `StartDiagnosis`, ...) |
| I10 | Every test that existed before still exists (157 `func Test...` in the slice) | `git grep -h "^func Test" c6e4677 -- internal/features/service-order` against `HEAD` |
| I11 | The coverage gate covers every `service-order` sub-package and keeps the 80% threshold | `scripts/coverage.sh` (`GATED_PACKAGES`, `gate_of`) |

## 3. Intentional changes — expected, do not flag

- **The single `ServiceOrderService` is split** into `Opening`, `Lifecycle`, `Quotes`,
  `Executions`, `Queries`, `StockUsage` and `Metrics`. `UpdateStatusExternal` lives in
  `Lifecycle` because it dispatches onto the same `startDiagnosis`/`finalizeOrder`/
  `deliverOrder` cores as the admin endpoints (`design.md` §9.1).
- **`QuoteDependencies` struct.** `NewQuotes` takes a struct instead of positional
  parameters, because it has 7 collaborators.
- **`NewQuotes(nil notifier)` falls back to `discardNotifier`.** It is equivalent to the old
  nil → `NoOpQuoteNotifier` default. `NoOpQuoteNotifier` itself moved to
  `adapter/notifier`, where `cmd/api` wires it.
- **Raw inputs move to `usecase`.** `CreateInput`, `QuoteItemInput`, `StockUsageItem`,
  `ListFilter`, `MetricsFilter` and the read models `CustomerRef`/`VehicleRef`/
  `ServiceRef`/`ProductRef` (previously unexported `customerRef`, ...) are use-case types,
  not domain types (`design.md` §9.1, §4).
- **History descriptions are exported from `domain`.** `HistoryDescription` and its
  constants are shared by the Postgres adapter and the test fakes.
- **New `RegisterRoutes` signature.** It is now `httpapi.RegisterRoutes(mux, httpapi.UseCases{...},
  requireAuth)`. This is an internal Go API, not an HTTP contract.
- **`cmd/api/wiring.go`** gains `buildServiceOrderUseCases`, and
  `internal/handlers_test/service_order_wiring_test.go` mirrors it for the integration tests.
- **Fakes.**
  - `usecase/fake_test.go` keeps one in-memory fake that implements every port, so
    multi-step flows share one consistent store (`design.md` §9.4).
  - `usecase/services_test.go` embeds all use cases, so the old tests keep calling
    `service.X(...)`.
  - `adapter/httpapi/fake_test.go` is a new, smaller fake that implements only what the
    tested routes need. Use cases the HTTP tests do not exercise get `nil` ports.
- **Other modules.** In `customer`, `vehicle` and `product`, the repository interface moves
  from `repository.go` to `service.go`. Same package, so behavior is unchanged
  (`requirements.md` §7.2).
- **New tests:**
  - `architecture_test.go` (3 tests).
  - `internal/handlers_test/service_order_repository_guards_test.go`.
  - `TestSendQuoteWithoutNotifierStillSends`.
  - `TestNoOpQuoteNotifierAcceptsEverything`.
  - `TestBuildServiceOrderUseCases`.

## 4. Risk hot spots — review these closely

1. **`adapter/httpapi/*_handler.go`.** These files were produced by splitting the old
   675-line `handler.go` per use case with a script. Confirm that no handler body was lost
   or duplicated, and that each handler calls the use case group it did before (for
   example, `finalizeOrder` → `useCases.Lifecycle.FinalizeOrder`). `router.go` must still
   register all 19.
2. **`usecase/lifecycle.go` and `usecase/executions.go`.** Old `execution_service.go` was
   split between them. `FinalizeOrder` and `ensureRequiredExecutionsComplete` (BR5) now live
   in `Lifecycle`, and must still read the quote through `QuoteReader` and the executions
   through `ExecutionLister`.
3. **Composition.** Each constructor in `cmd/api/wiring.go` must receive the right port,
   and the two external secrets must not be swapped: the quote-decision secret goes to
   `Quotes`, the status-update secret goes to `Lifecycle`. `TestBuildServiceOrderUseCases`
   asserts this.
4. **The `architecture_test.go` traversal rule.** It follows imports only out of this
   module's own packages. Standard-library and third-party packages are leaves: `uuid`
   imports `encoding/json` internally, and that must not fail the domain check. Verify
   that this does not open a loophole. A forbidden import reached *through our own code*
   still fails; this was tested with direct, transitive and non-allowlisted-library
   violations.
5. **The `scripts/coverage.sh` awk.**
   - `gate_of` matches `/...` entries on a `/` boundary, so `service-order/...` must not
     capture `service-order-tracking`.
   - An entry with no data must fail rather than pass.
   - Packages inside a prefix gate must not be listed again as "not gated".
6. **`service_order_repository_guards_test.go`.** It hands the repository a stale copy of
   the order to hit the `UPDATE ... WHERE status = ...` guards. It creates its own
   customer, vehicle, service and order, and cleans them up. Check that the cleanup order
   respects the foreign keys: the order is deleted first.

## 5. Out of scope — do not flag as defects of this PR

- **Package-level layering of `auth`, `customer`, `vehicle`, `product`,
  `service-catalog` and `service-order-tracking`.** It is explicitly recorded technical
  debt (`requirements.md` §7.2, `specs/architecture.md` decision 22,
  `docs/architecture-dependency-map.md`).
- **`float64` money**, handled by `specs/monetary-value-precision/`.
- **Portuguese `/acompanhamento/.../orcamento/aprovar|reprovar` routes**, an open decision
  (`CLAUDE.md` §8).
- **Two error envelopes (`httpx` vs `apierror`)**, an open decision (`CLAUDE.md` §8, §17).
- **Duplicated `httpsupport.go` and tracking-header constant across features**
  (`requirements.md` §4).
- **Pre-existing issues found while verifying**, to be handled as separate tasks:
  - The Postman collection deactivates the customer before creating the vehicle
    (`409 CUSTOMER_INACTIVE`), so its service-order requests run with empty ids. This
    happens on the old and the new API alike.
  - On a brand-new Docker volume, the Compose `migrate` service can fail once ("connection
    refused") because the `db` healthcheck passes during `initdb`. Re-running `up` fixes it.
  - `specs/architecture.md` has mojibake (`â€”`) from before this branch. The text added
    here is ASCII to avoid mixing.
  - `AGENTS.md` already lagged behind `CLAUDE.md` (Go 1.25, older infra). This PR applies
    the same architecture edits to both; it does not resync the rest.

## 6. How to verify

```bash
# Unit + architecture tests (no database needed)
go build ./... && go vet ./... && go test ./...

# Integration + coverage gate against a real Postgres
docker compose up -d db migrate seed        # re-run once if migrate fails on a new volume
export DATABASE_URL='postgres://workshop:workshop@localhost:5432/automotive_workshop?sslmode=disable'
export JWT_SECRET=dev-secret
scripts/coverage.sh

# End-to-end on the real image
docker compose up -d --build api && make smoke

# Prove the architecture check bites: this must FAIL, then delete the file
printf 'package domain\n\nimport _ "net/http"\n' > internal/features/service-order/domain/zz.go
go test ./internal/features/service-order/
rm internal/features/service-order/domain/zz.go
```

## 7. Evidence already collected (2026-10-21, local Postgres 16, fresh volume each run)

| Check | Before (`c6e4677`) | After (this branch) |
| --- | --- | --- |
| `service-order` coverage, same script | 85.5% (1383/1617), same in 3 runs | **85.9%** (1396/1625), aggregate of 5 sub-packages |
| Sub-packages (after) | — | domain 100%, usecase 89.6%, httpapi 87.1%, postgres 79.6%, notifier 93.3% |
| `product` / `service-order-tracking` | 89.9% / 85.1% | 89.9% / 85.1% |
| Integration suite (`internal/handlers_test`) | green | green |
| `make smoke` on the built image | — | ok |
| Postman collection, 50 requests, newman 6.2.1 | reference | identical status codes and response-body shapes on all 50 |
| Routes / JWT wrapping / JSON tags / error mapping / handler statuses | reference | textually identical (diffed) |
| Pre-existing tests | 157 | 157 present + new ones, all green |

**About the coverage history.** The first after-run gave 85.4%. The refactor adds 8
statements, mostly constructors. Investigation showed that the `DecideQuote` SQL guard
had only been covered by chance, through the concurrent HTTP test's scheduling, and the
nil-notifier default had not been covered at all. Deterministic tests were added for
both. The gate was not touched.

**Not yet verified:**
- The CI run on the PR, in particular the `security` and `migrations` jobs.
- The Insomnia and Bruno collections. Only Postman was executed; the routes are
  identical, so the risk is low.
- The full test suite under Go 1.26. Local tests ran on Go 1.27; the Docker build proved
  the code compiles on 1.26.

## 8. Suggested prompts for an AI reviewer

1. "Using §2 of the review guide, check each invariant I1–I11 against the diff and report
   any violation with file and line. Do not report items listed in §3 or §5."
2. "Compare the old `handler.go` (at `c6e4677`) with `adapter/httpapi/*_handler.go` and
   `router.go`. List any handler whose body, status code, error mapping or use-case call
   differs."
3. "Compare every SQL string in old `*_repository.go` with `adapter/postgres/*.go` and
   report any difference other than identifier renames."
4. "Review `architecture_test.go`: can an import of `net/http`, `pgx` or `apierror` reach
   `domain` or `usecase` without the test failing? Consider transitive paths through
   `internal/shared`."
5. "Review `scripts/coverage.sh`: can a `service-order` package escape the aggregate gate,
   or can the gate pass with no data?"
6. "Check `cmd/api/wiring.go` and `internal/handlers_test/service_order_wiring_test.go` for
   a port wired to the wrong argument or swapped external secrets."
