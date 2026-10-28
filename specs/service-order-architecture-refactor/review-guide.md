# Review guide — F2-US01 `service-order` Clean Architecture refactor

This file is for reviewers, human or AI. It holds the context a diff alone does not carry:
what must not change, what changed on purpose, where the risk is, how to verify, and what
not to flag. Read it before the diff.

**For AI-assisted review**, give the model:
- this file;
- the three spec files next to it (`requirements.md` §7 takes precedence over §1–§6,
  `design.md` §9 over §1–§7);
- `docs/f2-us01-decision-log.md`;
- the diff `git diff origin/develop...chore/F2-US01-clean-architecture-service-order`.

Prompts are in §8.

## 1. What this change is

It is a structural refactor of `internal/features/service-order/` (story F2-US01, Fase 2
R01/R02) into `domain/`, `usecase/` and `adapter/{postgres,httpapi,notifier}/`, with
dependencies pointing inward. It is **not** a feature change: no route, status code, JSON
field, error code, SQL statement or business rule is supposed to change.

**Branch layout (decision log D13/D16).**
- The PR merges into **`develop`**. After the merge, `develop` keeps **all the logic it has
  today**, with `service-order` in Clean Architecture.
- The same logic in the original vertical-slice layout lives on in the long-lived branch
  **`vertical-slice-arch`**, created at `develop@6779547` and never merged back. Its
  behavior is the regression baseline: "after" must equal `vertical-slice-arch`.

**History.**
- The refactor commits started from `c6e4677`. One commit per step, each building and
  passing tests on its own: spec → domain → usecase → postgres → httpapi+notifier →
  architecture test → coverage gate → other modules → docs → guard tests → evidence.
- Merge commit **`1fe315c`** then brought `develop` up to `6779547` into the branch. That
  includes FP-31 (exact money), FP-34/35/36 (Kubernetes, IaC, CD) and the setup-go
  `check-latest` CI fix, with FP-31 reapplied onto the layers (§4.0).
- Review commit by commit. Give `1fe315c` the most attention.

## 2. Invariants — must hold, check them

The baseline is `develop@6779547` (= `vertical-slice-arch`) unless noted.

| # | Invariant | Where to check |
| --- | --- | --- |
| I1 | `domain` imports only the standard library, `github.com/google/uuid` and the `internal/shared/money` value type | `domain/*.go` imports; `architecture_test.go` |
| I2 | `usecase` imports only `domain`, the standard library, `uuid`, `internal/shared/{document,trackingtoken,money}`; never `net/http`, `encoding/json`, `pgx`, `apierror`, `httpx` or any `adapter/` | `usecase/*.go`; `architecture_test.go` |
| I3 | No adapter imports another adapter or another feature's package | `TestAdaptersDoNotDependOnEachOtherOrOnOtherFeatures` |
| I4 | Ports are declared by their consumer (`usecase/ports.go`), each ≤ 4 methods; the 20-method `serviceOrderLookups` is gone | `usecase/ports.go`; `adapter/postgres/assertions.go` |
| I5 | Every SQL statement is unchanged against `develop@6779547`, including FP-31's `ScanCents`/`NumericFromCents` conversions | old `*_repository.go` at `6779547` vs `adapter/postgres/*.go`: only package/type names, exported method names and comments may differ |
| I6 | The 19 routes, their methods and their JWT wrapping are unchanged; the `/acompanhamento/...` routes and the two external-secret routes stay **unwrapped** | `adapter/httpapi/router.go` vs old `handler.go` `RegisterRoutes` |
| I7 | Error → HTTP status/code mapping is unchanged, including FP-31's `ErrQuoteAmountOutOfRange` → 400 | `adapter/httpapi/errors.go` vs old `httpsupport.go` `writeServiceError` |
| I8 | DTO JSON tags and money types are unchanged (`money.Cents` renders as exact decimals) | `adapter/httpapi/*_dto.go` |
| I9 | Domain transition guards are unchanged: RECEIVED→IN_DIAGNOSIS→AWAITING_APPROVAL→IN_PROGRESS/CANCELED→COMPLETED→DELIVERED | `domain/service_order.go`; the methods were only exported |
| I10 | Every test that exists on `develop@6779547` still exists: 331 `func Test...` in the slice and `internal/handlers_test` | `git grep -h "^func Test" 6779547 -- internal/features/service-order internal/handlers_test` vs `HEAD` |
| I11 | The coverage gate covers every `service-order` sub-package and keeps the 80% threshold | `scripts/coverage.sh` (`GATED_PACKAGES`, `gate_of`) |
| I12 | Quote money is exact and identical: items priced as UnitPrice × Quantity in cents, total summed with the NUMERIC(12,2) bound | `domain/quote.go` (`PriceQuoteItems`, `CalculateTotal`), `usecase/quotes.go` (`ComposeQuote`) |

## 3. Intentional changes — expected, do not flag

- **The single `ServiceOrderService` is split** into `Opening`, `Lifecycle`, `Quotes`,
  `Executions`, `Queries`, `StockUsage` and `Metrics`. `UpdateStatusExternal` lives in
  `Lifecycle` because it dispatches onto the same `startDiagnosis`/`finalizeOrder`/
  `deliverOrder` cores as the admin endpoints (`design.md` §9.1).
- **`QuoteDependencies` struct.** `NewQuotes` takes a struct instead of positional
  parameters, because it has 7 collaborators.
- **`NewQuotes(nil notifier)` falls back to `discardNotifier`.** It is equivalent to the old
  nil → `NoOpQuoteNotifier` default. `NoOpQuoteNotifier` moved to `adapter/notifier`.
- **Raw inputs and read models move to `usecase`.** `CreateInput`, `QuoteItemInput`,
  `StockUsageItem`, `ListFilter`, `MetricsFilter` and `CustomerRef`/`VehicleRef`/
  `ServiceRef`/`ProductRef` (previously unexported `customerRef`, ...) are use-case types.
- **Domain functions and transitions are exported**, because their caller is now in another
  package: `StartDiagnosis` (was `startDiagnosis`), ..., `ValidateQuoteItems`,
  `CalculateTotal`, and FP-31's **`PriceQuoteItems`** (was `priceQuoteItems`).
  `HistoryDescription` and its constants are exported from `domain` too.
- **New `RegisterRoutes` signature.** It is now `httpapi.RegisterRoutes(mux, httpapi.UseCases{...},
  requireAuth)`. This is an internal Go API, not an HTTP contract.
- **`cmd/api/wiring.go`** gains `buildServiceOrderUseCases`, and
  `internal/handlers_test/service_order_wiring_test.go` mirrors it for the integration tests.
- **Fakes.**
  - `usecase/fake_test.go` keeps one in-memory fake that implements every port.
  - `usecase/services_test.go` embeds all use cases, so the old tests keep calling
    `service.X(...)`.
  - `adapter/httpapi/fake_test.go` is a smaller fake with only what the tested routes
    need. Its amounts are in cents: `10000` is R$ 100,00.
- **FP-31's `TestComposeQuoteExactTotalsWithQuantities` is split.**
  - The arithmetic assertions stay in `usecase/quotes_test.go`.
  - Its JSON assertions (`"unitPrice":19.9,"total":59.7}` and `"totalAmount":60.4,`) move
    unchanged to `adapter/httpapi/dto_test.go`, as `TestQuoteResponseRendersExactTotals`,
    because DTOs live in the HTTP adapter now.
- **`internal/shared/money` is allowed in `domain`/`usecase`** as a value-type leaf (decision
  A, `requirements.md` §7.7). `TestValueTypePackagesStayInfrastructureFree` guards it.
- **Other modules.** In `customer`, `vehicle` and `product`, the repository interface moves
  from `repository.go` to `service.go`. Same package, so behavior is unchanged
  (`requirements.md` §7.2).
- **New tests (8):**
  - 4 in `architecture_test.go`;
  - `TestRepositoryTransitionGuardsRejectStaleOrders`;
  - `TestSendQuoteWithoutNotifierStillSends`;
  - `TestNoOpQuoteNotifierAcceptsEverything`;
  - `TestQuoteResponseRendersExactTotals`.

  `cmd/api` also gains `TestBuildServiceOrderUseCases`.

## 4. Risk hot spots — review these closely

0. **Merge commit `1fe315c` (FP-31 onto the layers).** It resolved 11 conflicts by hand.
   Check that every FP-31 change landed:

   | FP-31 change (flat file on `6779547`) | Lands in |
   | --- | --- |
   | `ErrQuoteAmountOutOfRange` (`errors.go`) | `domain/errors.go` |
   | `QuoteItem`/`Quote` in `money.Cents`, `priceQuoteItems`, `calculateTotal` → `(Cents, error)`, `quoteAmountError` (`model.go`) | `domain/quote.go` |
   | `ErrQuoteAmountOutOfRange` → 400 (`httpsupport.go`) | `adapter/httpapi/errors.go` |
   | Cents DTOs (`quote_dto.go`) | `adapter/httpapi/quote_dto.go` |
   | `QuoteNotification.TotalAmount` (`quote_notifier.go`) | `usecase/ports.go` |
   | `serviceRef.Price`, `SaveQuote(total money.Cents)` (`repository.go`) | `usecase/refs.go`, `usecase/ports.go` |
   | `productRef.UnitPrice`, 7 × `ScanCents` + 5 × `NumericFromCents` (`quote_repository.go`) | `usecase/refs.go`, `adapter/postgres/quote_repository.go`. Verified: the same 55 SQL statements and the same 7 + 5 conversions on both sides |
   | `ComposeQuote` prices then sums; `resolveQuoteItems` stops computing `Total` (`quote_service.go`) | `usecase/quotes.go` |
   | `TotalAmount.String()` in the e-mail (`resend_notifier.go`) | `adapter/notifier/resend.go` |

1. **`adapter/httpapi/*_handler.go`.** These files were produced by splitting the old
   675-line `handler.go` per use case with a script. Confirm that no handler body was lost
   or duplicated, and that each handler calls the same use case as before. `router.go` must
   still register all 19 routes.
2. **`usecase/lifecycle.go` and `usecase/executions.go`.** `FinalizeOrder` and
   `ensureRequiredExecutionsComplete` (BR5) live in `Lifecycle`, and read through
   `QuoteReader` and `ExecutionLister`.
3. **Composition.** Each constructor in `cmd/api/wiring.go` must receive the right port, and
   the two external secrets must not be swapped: the quote secret goes to `Quotes`, the
   status secret goes to `Lifecycle`. `TestBuildServiceOrderUseCases` asserts this.
4. **The `architecture_test.go` traversal rule.**
   - It follows imports only out of this module's own packages, except the
     `valueTypePackages` (`shared/money`).
   - Standard-library and third-party packages are leaves. `uuid` and `money` both import
     `encoding/json` internally.
   - Verified to fail on: a direct `domain → encoding/json`, a transitive
     `usecase → domain → net/http`, a library outside the allowlist, and `money → net/http`.
5. **The `scripts/coverage.sh` awk.**
   - `gate_of` matches `/...` entries on a `/` boundary, so `service-order/...` does not
     capture `service-order-tracking`.
   - An entry with no data fails.
   - Packages inside a prefix gate are not listed again as "not gated".
6. **`service_order_repository_guards_test.go`.** It hands the repository a stale copy of
   the order to hit the `UPDATE ... WHERE status = ...` guards, and cleans up its own
   fixtures (the order first, because of the foreign keys).

## 5. Out of scope — do not flag as defects of this PR

- **Package-level layering of the six flat modules.** It is recorded debt (`requirements.md`
  §7.2, `specs/architecture.md` decision 22, `docs/architecture-dependency-map.md`).
- **Portuguese `/acompanhamento/.../orcamento/aprovar|reprovar` routes** and **the two error
  envelopes**: open decisions (`CLAUDE.md` §8, §17).
- **Duplicated `httpsupport.go` and tracking-header constant across features**
  (`requirements.md` §4).
- **Pre-existing issues**, separate tasks (decision log P5–P7):
  - The Postman collection deactivates the customer before creating the vehicle, and
    hard-codes plate `ABC1D23`, which the seed already uses. Its service-order requests
    therefore run with empty ids on every version.
  - The Compose `migrate` service can fail once on a brand-new volume.
  - `specs/architecture.md` mojibake.
  - `AGENTS.md` lagging `CLAUDE.md`.

## 6. How to verify

```bash
# Use the CI toolchain: local Go 1.27 counts coverage statements differently (§7)
export GOTOOLCHAIN=go1.26.10

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

Behavioral regression against `vertical-slice-arch`:
1. Run the same Postman collection with newman on a fresh database against each version's
   image.
2. Compare the status codes and the full bodies, masking only UUIDs, timestamps and tokens.
3. For the full service-order flow, use a regression-only copy that moves the
   deactivate/delete requests to the end and uses a plate the seed does not already have
   (§7).

## 7. Evidence (2026-10-28, local Postgres 16, fresh volume each run, Go 1.26.10)

| Check | `vertical-slice-arch` (`develop@6779547`) | This branch (`1fe315c`) |
| --- | --- | --- |
| `service-order` coverage, same script | 82.7% (1143/1382) | **83.4%** (1159/1390), aggregate of 5 sub-packages |
| Sub-packages (branch) | — | domain 100%, usecase 89.2%, httpapi 82.7%, postgres 76.1%, notifier 91.3% |
| `product` / `service-order-tracking` | 88.3% / 83.6% | 88.3% / 83.6% |
| Integration suite (`internal/handlers_test`) | green | green |
| Tests in slice + `handlers_test` | 331 | 331 (all present) + 8 new |
| `make smoke` on the built image | — | ok |
| Postman collection as shipped (50 requests) | reference | identical status **and full bodies** (50/50) |
| Full-flow regression copy (50 requests; open → diagnose → quote → send → approve → execute → finalize → deliver → track, plus the invalid attempts) | reference | identical status (50/50); bodies identical except `averageDurationMinutes` of the execution created during the run, which is wall-clock time |
| Quote money in that flow | 1 × 209.90 + 2 × 49.90 = **309.70** | **309.70**, same items and `sentVersion` |
| `govulncheck` v1.7.0 | 0 vulnerabilities | 0 vulnerabilities |
| `gosec` v2.28.0 (CI runs it report-only) | 6 findings | 0 findings: **an analyzer-scope artifact, not a fix** (below) |

**`gosec` 6 → 0 is not an improvement.** The 6 findings on the base are taint-analysis
rules:
- 2 × G704 SSRF, on the Resend request;
- 4 × G706 log injection, on the quote-notification `log.Printf` lines.

gosec's taint tracking stays within one package. In the flat package, the HTTP source
and those sinks were in the same package; after the split they are not, so the analyzer
stops connecting them. The flagged lines are byte-for-byte the same code. In substance
they are false positives: the logs format only `order.Code` (an `int64`), and the
Resend URL is a constant.

**Coverage numbers are only comparable under one toolchain.** The same code counts 1639
service-order statements under Go 1.27 and 1390 under Go 1.26.10. The 2026-10-21 figures
in `tasks.md` (85.5% → 85.9%, base `c6e4677`) were measured with Go 1.27. Today's figures
use Go 1.26.10, the CI toolchain, on both sides.

**Not yet verified:**
- The CI run on the PR after the base changes to `develop`.
- The Insomnia and Bruno collections. The routes are identical, so the risk is low.

## 8. Suggested prompts for an AI reviewer

1. "Using §2 of the review guide, check each invariant I1–I12 against the diff and report
   any violation with file and line. Do not report items listed in §3 or §5."
2. "For merge commit `1fe315c`, check each row of the §4.0 table: is the FP-31 change from
   the flat file on `6779547` present, unchanged in meaning, in the layered file? Report
   anything missing or altered."
3. "Compare the old `handler.go` (at `6779547`) with `adapter/httpapi/*_handler.go` and
   `router.go`. List any handler whose body, status code, error mapping or use-case call
   differs."
4. "Compare every SQL string and every Scan/argument conversion in the old
   `*_repository.go` at `6779547` with `adapter/postgres/*.go`, and report any difference
   other than identifier renames."
5. "Review `architecture_test.go`: can `net/http`, `pgx` or `apierror` reach `domain` or
   `usecase` without the test failing? Consider transitive paths through `internal/shared`
   and through the `valueTypePackages` exception."
6. "Review `scripts/coverage.sh`: can a `service-order` package escape the aggregate gate,
   or can the gate pass with no data?"
7. "Check `cmd/api/wiring.go` and `internal/handlers_test/service_order_wiring_test.go` for
   a port wired to the wrong argument or swapped external secrets."
