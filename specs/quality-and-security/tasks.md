# Implementation Tasks: Verifiable Quality & Security

Ordered. Each task cites the `design.md` section it implements. Tasks 1–3 must land before
4–6: the report (task 9) quotes numbers that only exist once measurement and scanning run.

## Phase 1 — Measurement

- [x] **T1. `scripts/coverage.sh`** (design §2)
      Per-package `-coverpkg` pass over the gated packages, threshold check, aligned table,
      non-zero exit on failure. Hard-fails when `DATABASE_URL` is unset (§2.2).
      `COVERAGE_HTML=1` and `COVERAGE_THRESHOLD` honored. Gated list as a single array (§2.3).
      Verify: run against a clean local database and confirm it reproduces
      service-order 80.7% / product 71.7% / tracking 83.6% (requirements §3.1).

- [x] **T2. `.gitignore`** (design §7)
      Add `coverage/` and `security/`.

- [x] **T3. CI job `coverage`** (design §3.1)
      `postgres:16` service with health check, `schema.sql` + `seed.sql` applied with
      `ON_ERROR_STOP=1`, `scripts/coverage.sh`, artifact upload with `if: always()`.
      The existing `build` job is not modified.
      Verify: the job is red on the current tree (product at 71.7%) — that red is the proof
      the gate works, and T5 turns it green.

## Phase 2 — Closing the gaps

- [x] **T4. `internal/handlers_test/sensitive_data_test.go`** (design §4.2)
      Five error paths, one shared forbidden-substring assertion helper, `t.Skip` without
      `DATABASE_URL`, `testify` assertions. Closes scenario 10 (requirements §5).

- [x] **T5. Product coverage to ≥80%** (design §5)
      5a. `internal/features/product/service_test.go`: `GetByCode` found/not-found,
          `UpdateDetails` partial-field branches, deactivation blocked by `ErrProductInUse`,
          each `Validate` failure branch.
      5b. `internal/handlers_test/product_test.go`: 404 on every route for an unknown id,
          malformed UUID and malformed JSON returning 400, list filter/pagination branches,
          plus repository-level coverage of `FindByCode` and `IsUsedInQuotesOrOrders` —
          neither is reachable over HTTP, since no route exposes lookup by code or physical
          deletion.
      Verify: `scripts/coverage.sh` reports `product` ≥80% and exits 0.
      Constraint: if it lands short, close the gap with more behavior tests — never by
      narrowing `-coverpkg` or excluding files (§5).

## Phase 3 — Security analysis

- [x] **T6. `scripts/security-scan.sh`** (design §3.3)
      `govulncheck` and `gosec` via `go run <module>@<pinned-version>`, JSON into
      `security/`, tool versions printed to stdout.
      Verified: both tools declare `go 1.25` and cannot be built by Go 1.22, so they are
      built by whatever toolchain runs them (Go 1.25 in CI) while the module itself stays on
      1.22 — `go run <module>@<version>` resolves in its own module context, so `go.mod` is
      untouched (`CLAUDE.md` §2, BR-Q8).

- [x] **T7. CI job `security`** (design §3.2)
      `govulncheck@v1.7.0` and `gosec@v2.28.0` pinned exactly — never `@latest`.
      `gosec` with `-no-fail` for the baseline run; `govulncheck` fails the job on findings.
      JSON artifacts uploaded. `go.mod` and `go.sum` must be unchanged by this task (BR-Q8).

- [x] **T8. Triage and fix** (design §6, requirements §4 BR-Q5)
      Fix what is safe: dependency bumps that respect the Go 1.22 ceiling, low-risk `gosec`
      findings. Record the commit for each fix as evidence. Anything architectural goes to
      residual risk instead — do not touch the `CLAUDE.md` §17 open decisions.
      Verify after each fix: `go build ./...`, `go vet ./...`, `go test ./...`.

## Phase 4 — Deliverable and documentation

- [x] **T9. `docs/security-report.md`** (design §6)
      All ten sections. Tool/version/date table, reproduction commands, coverage table,
      findings with severity/impact/recommendation, fixes with evidence, residual risks
      (the six in §6.1 plus whatever triage adds), secrets review, log and error-response
      review, conclusion.

- [x] **T10. `README.md`** (design §7)
      Correct the Tests section: the seed is a prerequisite for the integration tests to
      pass, not just the database. Document `scripts/coverage.sh` and
      `scripts/security-scan.sh` with their environment variables.

- [x] **T11. `CLAUDE.md` §7** (design §7)
      Replace "no additional linter is configured … to be defined" with what is now
      configured. Keep the `golangci-lint` question open.

## Phase 5 — Review

- [x] **T12. Verify against the acceptance criteria** (requirements §6)
      Walk AC01–AC13 one by one and record the evidence for each. Confirm nothing outside
      requirements §7's scope was changed — in particular `go.mod`, `specs/architecture.md`,
      and every `internal/features/*` file not listed in T4/T5/T8. `cmd/api/main.go` was
      originally listed here as must-not-change; it is in scope for the two SAST fixes only
      (requirements §9), which touch no route and no response body.
      Confirm `go build ./...`, `go vet ./...`, `go test ./...` pass, and that
      `scripts/coverage.sh` exits 0 against a clean database.


## Completion notes (2026-08-24)

- T1 measures with one instrumented run plus per-package aggregation instead of one run per
  package; both methods verified to agree (design §2.1).
- T6/T7 run the scanners via `go run <module>@<version>` (`govulncheck@v1.7.0`,
  `gosec@v2.28.0`) instead of the vendors' GitHub Actions; the scanner CI job uses Go 1.25
  because both tools require it to build, while the module stays on 1.22 (design §3.2).
- T5 result: `product` 71.7% → 88.9%. All three gated packages pass.
- T8 result: all three `gosec` findings fixed (`Issues: 0` after). No dependency finding
  could be fixed under the Go 1.22 ceiling — both fixed versions require Go 1.25 and the pgx
  advisory has no backport, so they became residual risks R1/R2 with the Go upgrade recorded
  as the recommended follow-up (requirements §9).

## T12 — Acceptance criteria evidence (2026-08-24)

| AC | Evidence |
|----|----------|
| AC01 | Unit tests for service order/quote (`service-order/*_test.go`), stock (`product/{model,service,dto}_test.go`) |
| AC02 | `internal/handlers_test/` — 8 files covering every main flow |
| AC03 | `scripts/coverage.sh`: 80.7% / 88.9% / 83.6%, exit 0; exit 1 when any gated package is below threshold |
| AC04 | `README.md` — Tests, Coverage, and Security scan sections, including the seed prerequisite |
| AC05 | `docs/security-report.md` §3, commands runnable from a clean checkout |
| AC06 | `govulncheck` v1.7.0 and `gosec` v2.28.0 executed; CI job `security` repeats them per push |
| AC07 | `docs/security-report.md` §1–§2 — commit, branch, date, tool versions, vuln DB date |
| AC08 | `docs/security-report.md` §5 — nine findings, each with severity, impact, recommendation |
| AC09 | `docs/security-report.md` §6 — three SAST fixes; `gosec` re-run reports `Issues: 0` |
| AC10 | `docs/security-report.md` §7 — eight residual risks (R1–R8) with justification |
| AC11 | `docs/security-report.md` §8 — no secret versioned; `.env` git-ignored; no G101 finding |
| AC12 | `docs/security-report.md` §9 + `internal/handlers_test/sensitive_data_test.go` |
| AC13 | `docs/security-report.md` — self-contained Markdown, ready to export |

Scope check: `go.mod`/`go.sum` unchanged (`git diff` empty), `specs/architecture.md`
unchanged, no route or business behavior changed. `cmd/api/main.go` changed only by the two
SAST fixes (requirements §9).

Verification commands: `go build ./...`, `go vet ./...` pass; `go test ./...` passes both
with a database (13 packages ok) and without one (integration tests skip, exit 0);
`scripts/coverage.sh` exits 0 with a database and 1 without `DATABASE_URL`.
