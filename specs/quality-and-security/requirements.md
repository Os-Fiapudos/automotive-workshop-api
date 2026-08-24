# Requirements Specification: Verifiable Quality & Security

## 0. Nature of this specification

This is a **cross-cutting** specification, not a business vertical slice. Unlike every other
folder under `specs/`, it does **not** map to an `internal/features/<feature>/` package: it
produces no new endpoint, no new domain entity, and no new runtime behavior. What it
produces is verification infrastructure (coverage measurement, CI jobs, security scanning)
plus one delivery artifact (the vulnerability report).

`specs/README.md` states that `<feature>/` *generally* corresponds to a vertical slice —
this specification is a deliberate, documented exception to that expectation, kept under
`specs/` because the SDD flow (requirements → design → tasks) still applies to it.

## 1. Feature Purpose

**User story**

> As the team responsible for the product, I want to run automated tests and vulnerability
> analysis, so that we deliver a trustworthy MVP and can demonstrate the quality of the
> solution.

Concretely, this feature must make three claims about the system *verifiable by a third
party running one documented command*, rather than asserted:

1. The critical domains are covered by automated tests to at least 80%.
2. Logs and error responses leak no sensitive data.
3. The code and its dependencies have been scanned for vulnerabilities, and every finding
   is either fixed with evidence or accepted with a justified residual risk.

## 2. Non-Functional Requirements addressed

- **RNF06 (Minimum coverage)**: at least 80% statement coverage in the critical domains.
- **RNF08 (Logs without sensitive data)**: no password, password hash, JWT, tracking token,
  or database connection string appears in logs or in API error responses.
- **RNF09 (SAST and/or dependency analysis)**: static analysis of the Go code and analysis
  of the dependency tree, executed automatically.

Mandatory deliverable: **vulnerability report**.

## 3. Critical domains (scope of RNF06)

The ticket names three critical domains. They map to Go packages as follows:

| Critical domain    | Go package                                  | Rationale |
|--------------------|---------------------------------------------|-----------|
| Service Order      | `internal/features/service-order`           | Status lifecycle, execution, delivery |
| Quote              | `internal/features/service-order`           | Quote composition/sending/decision live in the same slice |
| Stock              | `internal/features/product`                 | Balance, adjustments, insufficient-stock rule |

One additional package is included in the gate:

| Extra domain       | Go package                                  | Rationale |
|--------------------|---------------------------------------------|-----------|
| Order tracking     | `internal/features/service-order-tracking`  | Hosts the "improper access to tracking" scenario the ticket lists as mandatory, and is already above the threshold |

Every other package (`auth`, `customer`, `vehicle`, `service-catalog`, `internal/shared/*`,
`cmd/api`) is **measured and reported but not gated**. Widening the gate is a separate
decision, not part of this delivery.

### 3.1 Measured baseline (2026-08-24)

Measured with a clean PostgreSQL 16 database (`docs/schema.sql` + `docs/seed.sql` applied)
and `go test ./... -coverpkg=<package>`, so that the integration tests in
`internal/handlers_test/` — a separate package — count towards the slice they exercise:

| Package                                    | Baseline | Target | Gap |
|--------------------------------------------|----------|--------|-----|
| `internal/features/service-order`           | 80.7%    | 80%    | met |
| `internal/features/product`                 | 71.7%    | 80%    | −8.3pp |
| `internal/features/service-order-tracking`  | 83.6%    | 80%    | met |

Without `DATABASE_URL` the integration tests skip themselves and the same packages report
26.8% / 20.8% / 6.6%. **The 80% claim is only meaningful against a real database** — this is
the single most important constraint on the design.

## 4. Business rules (BR)

- **BR-Q1**: Coverage of every gated package must be greater than or equal to 80%. The
  measurement must fail loudly (non-zero exit) below the threshold, both locally and in CI.
- **BR-Q2**: The coverage measurement must be reproducible from a single documented command,
  producing identical results in a clean environment.
- **BR-Q3**: The security scan must record, for each execution, the tool name, the tool
  version, and the date of the analysis.
- **BR-Q4**: Every finding must state severity, impact, and recommendation.
- **BR-Q5**: A fixed finding must carry evidence of the fix (commit and/or the diff of what
  changed). An unfixed finding must carry a justification and its residual risk.
- **BR-Q6**: No secret may be versioned in the repository. Credentials that exist only for
  local development must be explicitly identified as such in the report.
- **BR-Q7**: No log statement and no API error response may contain a password, a password
  hash, a JWT, a tracking token, or a database connection string — extending BR5 from
  `specs/auth/requirements.md` from logs to error responses as well.
- **BR-Q8**: Verification tooling must not enter the Go module. No `tools.go`, no `go get`
  of a scanner: `go.mod` must remain unchanged by this feature, so the Go 1.22 ceiling
  documented in `CLAUDE.md` §2 is preserved.

## 5. Mandatory test scenarios

The ticket lists ten minimum scenarios. Nine already have tests; the mapping below is the
traceability evidence, and the last row is the only genuine gap.

| # | Scenario | Status | Test |
|---|----------|--------|------|
| 1 | CPF/CNPJ and license plate validation | covered | `internal/shared/document/{cpf,cnpj,document}_test.go`; `internal/features/vehicle/plate_test.go`; `TestCreateRejectsInvalidCPF`, `TestCreateAcceptsAlphanumericCNPJ`, `TestVehicleCreateRejectsInvalidPlate` |
| 2 | Quote calculation | covered | `TestCalculateTotalSumsItems`, `TestCalculateTotalManyItemsNoDrift`, `TestCalculateTotalEmpty` |
| 3 | Price snapshot | covered | `TestComposeQuoteCatalogChangeDoesNotAffectPersistedItem`, `TestServiceOrderComposeQuoteSnapshotSurvivesCatalogChange` |
| 4 | Service order transitions | covered | `TestStartDiagnosisRejectsNonRecebida`, `TestFinalizeRejectsNonEmExecucao`, `TestDeliverRejectsNonFinalizada`, `TestServiceOrderDetailByIDFullLifecycle` |
| 5 | Approval and rejection | covered | `TestApproveQuoteFromAguardandoAprovacao`, `TestRejectQuoteFromAguardandoAprovacao`, `TestQuoteDecisionApproveFullFlow`, `TestQuoteDecisionRejectFullFlow`, `TestApproveThenRejectSameQuoteFails` |
| 6 | Negative balance blocking | covered | `TestProductApplyStockAdjustment`, `TestStockAdjustmentFlow` (409 `INSUFFICIENT_STOCK`) |
| 7 | Transactional stock write-off | covered | `TestStockAdjustmentConcurrency`, `TestServiceOrderCreateRollsBackOnPartialFailure` |
| 8 | Improper access to tracking | covered | `TestTrackingMissingToken`, `TestTrackingWrongToken`, `TestTrackingCrossOrderToken`, `TestTrackingRevokedToken` |
| 9 | Invalid or expired JWT | covered | `TestMeWithoutToken`, `TestMeWithInvalidToken`, `TestMeWithExpiredToken`, `TestCatalogRoutesRequireAuthentication` |
| 10 | No sensitive data in error responses | **gap** | to be written — see BR-Q7 |

## 6. Acceptance criteria

Mirrors the ticket's acceptance checklist one-to-one.

- **AC01**: Unit tests exist for service order, quote, and stock. *(satisfied by scenarios
  2, 4, 5, 6 above; verified, not re-implemented)*
- **AC02**: Integration tests exist for the main flows. *(satisfied by
  `internal/handlers_test/`; verified)*
- **AC03**: Coverage of the critical domains is at or above 80%, enforced automatically.
- **AC04**: The test command is documented in the README, including the database and seed
  prerequisites.
- **AC05**: The coverage report can be reproduced by a third party from that documentation.
- **AC06**: A SAST and/or dependency scan has been executed.
- **AC07**: The report states tool, date, and analyzed version.
- **AC08**: Every vulnerability states severity, impact, and recommendation.
- **AC09**: Fixed findings carry evidence of the fix.
- **AC10**: Unfixed findings carry justification and residual risk.
- **AC11**: No secrets are versioned.
- **AC12**: Logs and responses were reviewed against data leakage, and that review is backed
  by an automated test.
- **AC13**: The report is ready for inclusion in the final delivery PDF.

## 7. Out of scope

Explicitly **not** part of this delivery, to keep it from turning into a refactor:

- Resolving the open decisions in `CLAUDE.md` §17 (`httpx` vs `apierror`; wrapping the
  customer routes in `RequireAuth`). They are reported as residual risk instead.
- Adopting a migration tool. The absence of one is reported as residual risk.
- Adopting `golangci-lint` or any linter beyond what RNF09 requires.
- Raising coverage of non-gated packages.
- Updating `CLAUDE.md`'s stale feature inventory (it describes three features; eight exist)
  and `specs/README.md`'s claim that no specification exists yet. Both are real, both belong
  to their own ticket.

## 8. Ambiguities resolved before design

Recorded per `specs/README.md` §1 — these were decided with the requester, not assumed:

1. **How to measure and enforce 80%** → PostgreSQL service container in CI, measurement with
   `-coverpkg`, build fails below the threshold.
2. **Which tools for RNF09** → `govulncheck` (dependencies) and `gosec` (SAST). Trivy,
   CodeQL, and Dependabot were considered and declined for this delivery.
3. **Where the deliverable lives** → `docs/security-report.md`, versioned, alongside the
   other project documents.
4. **Whether findings get fixed** → yes, when the fix is safe and respects the Go 1.22
   ceiling; anything architectural becomes justified residual risk.

## 9. Decided during implementation (2026-08-24)

Recorded here rather than left implicit in the code:

- **`cmd/api/main.go` is in scope for SAST fixes.** Two of the three `gosec` findings
  (missing server timeouts, unhandled encode error) live there. Both were fixed: they change
  no route, no wiring, and no response body. The file remains out of scope for anything else.
- **Every dependency finding is blocked by the Go 1.22 ceiling.** The fixed versions of both
  vulnerable modules (`pgx` v5.9.2, `x/text` v0.39.0) declare `go 1.25`, and the advisory for
  the pgx SQL-injection issue lists no backport. Under BR-Q8 and `CLAUDE.md` §2 they cannot
  be applied here, so they became residual risks R1/R2 in `docs/security-report.md` with
  raising the project's Go version recorded as the recommended follow-up. That upgrade stays
  out of scope (§7): it changes the build for every feature and is the team's decision.
