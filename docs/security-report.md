# Vulnerability and Quality Report — automotive-workshop-api

Deliverable of the "Verifiable Quality & Security" feature
([specs/quality-and-security/](../specs/quality-and-security/)), covering RNF06 (minimum
coverage), RNF08 (logs without sensitive data), and RNF09 (SAST and dependency analysis).

## 1. Scope and analyzed version

| Item | Value |
|------|-------|
| Repository | `automotive-workshop-api` |
| Branch | `feature/FP24-qualidade-e-seguranca` |
| Commit analyzed | `4ede2476d85fe7944a5e5e0172a9f7e9aafecbe3` (pre-fix baseline) |
| Analysis date | 2026-08-24 |
| Module Go version | 1.22 at analysis time; **raised to 1.25 on the same day** — see §6.1 |
| Analyzed | All Go source under `cmd/` and `internal/`, the full dependency tree in `go.mod`/`go.sum`, log call sites, API error responses, and versioned files for secrets |
| Not analyzed | Deployed infrastructure, the Postgres server configuration, container images beyond the Go build stage, and any environment outside this repository |

**Critical domains** (RNF06): Service Order and Quote (`internal/features/service-order`),
Stock (`internal/features/product`), plus Order Tracking
(`internal/features/service-order-tracking`), which hosts the improper-access scenario.

## 2. Tools

| Tool | Version | Date run | What it analyzes |
|------|---------|----------|------------------|
| `govulncheck` | v1.7.0 (vuln DB `https://vuln.go.dev`, updated 2026-08-21) | 2026-08-24 | Known vulnerabilities in the dependency tree and the Go standard library, reported only when the vulnerable symbol is actually reachable from this code |
| `gosec` | v2.28.0 | 2026-08-24 | Static analysis (SAST) of the Go source: injection, weak crypto, unsafe conversions, missing timeouts, unhandled errors |
| `go test` + `go tool cover` | Go 1.26.5 locally, Go 1.22 in CI | 2026-08-24 | Statement coverage per package |

Both scanners are pinned to exact versions and executed through `go run <module>@<version>`,
which resolves in its own module context: neither is added to this project's `go.mod`, so
the Go 1.22 build ceiling is unaffected.

`govulncheck` was run **twice, under two different toolchains**, because it reports standard
library vulnerabilities relative to the Go version running the analysis. The Go 1.22 run is
the one that reflects the deployed artifact, since `Dockerfile` builds on
`golang:1.22-alpine`.

## 3. Reproduction

```bash
# 1. Database (integration tests and coverage require it; the seed is mandatory,
#    not optional — without it the authentication tests fail and cascade)
docker compose up -d db
docker compose cp docs/schema.sql db:/tmp/schema.sql
docker compose cp docs/seed.sql   db:/tmp/seed.sql
docker compose exec db psql -U workshop -d automotive_workshop -f /tmp/schema.sql
docker compose exec db psql -U workshop -d automotive_workshop -f /tmp/seed.sql

export DATABASE_URL='postgres://workshop:workshop@localhost:5432/automotive_workshop?sslmode=disable'
export JWT_SECRET=dev-secret

# 2. Coverage (exits non-zero if any critical domain is below 80%)
scripts/coverage.sh
COVERAGE_HTML=1 scripts/coverage.sh   # also writes coverage/coverage.html

# 3. Security scan (writes security/govulncheck.json and security/gosec.json)
scripts/security-scan.sh

# 4. The standard-library picture as actually deployed (Go 1.22)
GOTOOLCHAIN=go1.22.12 go run golang.org/x/vuln/cmd/govulncheck@v1.7.0 ./...
```

The same commands run in CI on every push and pull request
([.github/workflows/ci.yml](../.github/workflows/ci.yml), jobs `coverage` and `security`),
publishing `coverage/` and `security/` as build artifacts.

## 4. Coverage results (RNF06)

Measured after the tests added by this feature, against a clean PostgreSQL 16 with schema
and seed applied. `-coverpkg` is what credits the integration tests in
`internal/handlers_test/` — a separate package — to the slice they exercise.

| Critical domain | Package | Baseline | Current | Threshold | Result |
|-----------------|---------|----------|---------|-----------|--------|
| Service Order + Quote | `internal/features/service-order` | 80.7% | **80.7%** | 80% | PASS |
| Stock | `internal/features/product` | 71.7% | **88.9%** | 80% | PASS |
| Order Tracking | `internal/features/service-order-tracking` | 83.6% | **83.6%** | 80% | PASS |

Measured but not gated:

| Package | Coverage |
|---------|----------|
| `internal/features/auth` | 92.5% |
| `internal/features/customer` | 81.9% |
| `internal/features/service-catalog` | 92.0% |
| `internal/features/vehicle` | 80.9% |
| `internal/shared/apierror` | 90.0% |
| `internal/shared/database` | 57.1% |
| `internal/shared/document` | 93.5% |
| `internal/shared/httpx` | 80.0% |
| `internal/shared/middleware` | 100.0% |
| `internal/shared/token` | 92.3% |
| `internal/shared/trackingtoken` | 83.3% |
| `internal/shared/config` | 0.0% |
| `cmd/api` | 0.0% |

Enforcement: `scripts/coverage.sh` exits non-zero when any gated package is below the
threshold, and the CI `coverage` job runs it on every push, so a regression breaks the
build rather than being noticed later.

**Re-measured after merging `develop`** (stock usage for service orders, average execution
time metric, README update): service-order 80.0%, product 88.3%, tracking 83.6% — all still
passing, but service-order now sits exactly on the threshold, having lost 0.7pp to the
incoming code. The next feature that adds service-order statements without tests will break
the gate. `gosec` was re-run over the merged tree (91 files, 9,012 lines) and still reports
`Issues: 0`.

**Caveat, stated explicitly**: without `DATABASE_URL` the integration tests skip themselves
and the same three packages measure 26.8% / 20.8% / 6.6%. The 80% claim is only valid with
a database present. The script refuses to run without one, precisely so that a passing
number can never be produced from skipped tests.

## 5. Findings

Nine findings. Severity for dependency findings follows the advisory; severity for SAST
findings follows the `gosec` classification.

| ID | Severity | Component | Finding | Status |
|----|----------|-----------|---------|--------|
| SAST-01 | High | `internal/features/product/repository.go` | G115 — integer overflow in `int` → `rune` conversion | **Fixed** |
| SAST-02 | Medium | `cmd/api/main.go` | G114 — HTTP server without timeouts | **Fixed** |
| SAST-03 | Low | `cmd/api/main.go` | G104 — unhandled error on the health response | **Fixed** |
| DEP-01 | High | `github.com/jackc/pgx/v5@v5.7.4` | GO-2026-5004 — SQL injection via placeholder confusion with dollar-quoted string literals | **Fixed** — see §6.1 |
| DEP-02 | Medium | `golang.org/x/text@v0.22.0` | GO-2026-5970 — infinite loop on invalid input | **Fixed** — see §6.1 |
| STD-01 | High | Go standard library 1.22.12 | 26 reachable standard-library vulnerabilities, all fixed in Go 1.23–1.25 | **Fixed** — see §6.1 |
| CFG-01 | Medium | `docker-compose.yml`, `.env.example` | `sslmode=disable` on the database connection | **Not fixed** — residual risk R3 |
| CFG-02 | Low | `docs/openapi.yaml`, integration tests, compose defaults | Development credentials present in versioned files | **Accepted** — residual risk R4 |
| ARCH-01 | Medium | `cmd/api/main.go` | Customer and service-order-creation routes are unauthenticated; no role-based authorization anywhere | **Not fixed** — residual risks R5, R6 |

### SAST-01 — Integer overflow in placeholder construction (High, CWE-190)

`internal/features/product/repository.go`, function `strconvIdx`, used to build SQL
placeholders (`$1`, `$2`, …) for the dynamic `List` filter.

- **Impact**: the code built the placeholder with `string(rune('0' + *idx))`, valid only for
  a single digit, with a hand-rolled fallback for values ≥ 10. `gosec` flags the conversion
  as an unchecked `int` → `rune` cast: an out-of-range index would produce an arbitrary
  rune rather than a digit, corrupting the generated SQL. Not reachable today (the filter
  never reaches ten parameters), which is why the fallback branch had zero test coverage.
- **Recommendation**: use `strconv.Itoa`.
- **Fix applied**: `strconvIdx` now returns `strconv.Itoa(*idx)`, and the hand-rolled
  `convertIntToString` helper it depended on was deleted.
- **Evidence**: re-running `gosec` v2.28.0 over the whole module reports `Issues: 0`
  (`security/gosec.json`, `"found": 0`). `go build`, `go vet`, and `go test ./...` pass.

### SAST-02 — HTTP server without timeouts (Medium, CWE-676)

`cmd/api/main.go`: the server was started with `http.ListenAndServe`, which applies no read,
write, or idle timeout.

- **Impact**: a client that opens a connection and sends headers slowly (or never) holds a
  connection and its goroutine indefinitely. Enough such connections exhaust server
  resources — the classic slowloris denial-of-service pattern. No authentication is needed
  to attempt it.
- **Recommendation**: construct an explicit `http.Server` with `ReadHeaderTimeout`,
  `ReadTimeout`, `WriteTimeout`, and `IdleTimeout`.
- **Fix applied**: the server is now built explicitly with 10s header, 30s read, 30s write,
  and 120s idle timeouts.
- **Evidence**: `gosec` reports zero issues; the API still serves `/health` and the full
  integration suite (which drives every route over real HTTP) passes.

### SAST-03 — Unhandled error on the health response (Low, CWE-703)

`cmd/api/main.go`: the `/health` handler discarded the error from `json.Encoder.Encode`.

- **Impact**: minimal — a failed write to a disconnected client passed silently. It is
  nonetheless the kind of silent discard `CLAUDE.md` §15 prohibits.
- **Recommendation**: log the error, as `internal/shared/httpx` already does.
- **Fix applied**: the encode error is now logged (`log.Printf`), with no client-visible
  change.
- **Evidence**: `gosec` reports zero issues.

### DEP-01 — SQL injection in pgx v5.7.4 (High)

`GO-2026-5004` — "SQL Injection via placeholder confusion with dollar quoted string
literals in github.com/jackc/pgx". Reachable trace reported by `govulncheck`:
`internal/features/service-order/quote_repository.go:315` → `pgxpool.Pool.Query` →
`sanitize.SanitizeSQL`.

- **Impact**: pgx's SQL sanitizer can confuse a `$n` placeholder with a dollar-quoted
  string literal, so a crafted string value can escape parameterization and alter the
  executed statement. This project uses parameterized queries throughout and never
  concatenates input into SQL, which narrows exposure considerably, but the flaw is inside
  the driver's own sanitizer, below the level application code controls — it cannot be
  ruled out from application review alone.
- **Recommendation**: upgrade to `github.com/jackc/pgx/v5` v5.9.2 or later.
- **Initially blocked, then fixed**: pgx v5.9.2 declares `go 1.25.0`, and the OSV advisory
  lists no backport, so the upgrade was impossible while the project was pinned to Go 1.22.
  That pin was raised the same day (§6.1) and pgx moved to v5.10.0.

### DEP-02 — Infinite loop in golang.org/x/text v0.22.0 (Medium)

`GO-2026-5970`. Reachable through `internal/shared/database/postgres.go:13` →
`pgxpool.New` → `norm.Form.Properties` / `Span` / `Transform`.

- **Impact**: malformed input reaching Unicode normalization can cause an infinite loop,
  hanging the goroutine that processes it. The reachable path is connection setup rather
  than request handling, which limits exposure to attacker-controlled input.
- **Recommendation**: upgrade `golang.org/x/text` to v0.39.0 or later.
- **Initially blocked, then fixed**: v0.39.0 declares `go 1.25.0` — same ceiling as DEP-01,
  lifted by the Go 1.25 upgrade (§6.1). `golang.org/x/text` moved to v0.41.0.

### STD-01 — 26 reachable standard-library vulnerabilities under Go 1.22 (High, aggregate)

`govulncheck` run under `GOTOOLCHAIN=go1.22.12` — the toolchain the `Dockerfile` and CI
actually build with — reports **28 reachable vulnerabilities**: DEP-01, DEP-02, and 26 in
the Go standard library itself. The same scan under Go 1.26.5 reports 6. The difference is
entirely the toolchain.

The standard-library findings cluster in `crypto/x509` (7), `crypto/tls` (6), `net/url` (3),
`encoding/asn1` (2), plus `net/http`, `net/textproto`, `net`, `os`, `syscall`,
`encoding/xml`, `encoding/pem`, and `net/http/internal`. They include request smuggling via
invalid chunked data (`GO-2025-3563`, reachable from every JSON-decoding handler) and
several certificate-parsing and TLS-handshake denial-of-service issues.

- **Impact**: an API built on Go 1.22.12 ships every one of these unpatched. `crypto/tls`
  and `net/http` issues are directly exposed to any client that can reach the service.
- **Recommendation**: raise the project's Go version to 1.25 (currently the newest
  supported line), in `go.mod`, `.github/workflows/ci.yml`, and `Dockerfile` together. That
  single change also unblocks DEP-01 and DEP-02, taking the reachable count from 28 to
  approximately zero.
- **Initially out of scope, then decided and applied**: raising the Go version is a
  project-wide decision, so it was reported as the highest-value follow-up rather than done
  unilaterally. The team took that decision on 2026-08-24, after the CI `security` job made
  the finding concrete by failing. See §6.1.

### CFG-01 — `sslmode=disable` (Medium)

The connection string used by compose, `.env.example`, and the tests disables TLS between
the API and PostgreSQL.

- **Impact**: correct for a local Docker network; in a deployed environment it exposes
  credentials and query traffic to anyone able to observe the network path.
- **Recommendation**: require `sslmode=require` (or stricter) in any non-local environment
  and document it as a deployment precondition. Not changed here because the local
  environment is the only one this repository configures.

### CFG-02 — Development credentials in versioned files (Low, accepted)

`admin123` appears in `docs/openapi.yaml` (as an example payload) and in the integration
tests; `workshop:workshop` appears in the compose defaults and `.env.example`.

- **Impact**: none against a deployed system as long as these values exist nowhere but local
  development. They are documented fixtures, not secrets — the integration tests need
  deterministic credentials, and `docs/seed.sql` creates that user explicitly for local use.
- **Recommendation**: never reuse these values in a deployed environment; keep real secrets
  in `.env`, which is git-ignored.

### ARCH-01 — Unauthenticated routes and absent authorization (Medium)

`cmd/api/main.go` registers the customer routes and the service-order creation route without
`middleware.RequireAuth`, and no role or permission check exists anywhere: a valid token
reaches every protected route.

- **Impact**: customer personal data (name, document, phone) is readable and writable
  without authentication. Any authenticated user can perform any administrative operation.
- **Recommendation**: wrap the customer routes in `RequireAuth` per the convention in
  `specs/auth/design.md` §7, and specify a role model before the MVP is exposed publicly.
- **Why not fixed here**: both are open decisions recorded in `CLAUDE.md` §17 and §13, with
  cross-feature behavioral impact. Resolving them silently inside a quality-and-security
  ticket is exactly what the project's SDD rules prohibit. See residual risks R5 and R6.

## 6. Fixed findings — evidence

| ID | Fix | Verification |
|----|-----|--------------|
| SAST-01 | `strconvIdx` uses `strconv.Itoa`; dead `convertIntToString` helper removed (`internal/features/product/repository.go`) | `gosec` v2.28.0: `Issues: 0`; `go build ./...`, `go vet ./...`, `go test ./...` pass; `product` coverage rose from 71.7% to 88.9% |
| SAST-02 | Explicit `http.Server` with header/read/write/idle timeouts (`cmd/api/main.go`) | `gosec` v2.28.0: `Issues: 0`; full integration suite still exercises every route over real HTTP |
| SAST-03 | Encode error logged in the `/health` handler (`cmd/api/main.go`) | `gosec` v2.28.0: `Issues: 0` |

Before: `gosec` reported 3 issues (1 High, 1 Medium, 1 Low) over 84 files / 8,184 lines.
After: 0 issues over 84 files / 8,183 lines (`security/gosec.json`), and still 0 over the
merged tree (91 files / 9,012 lines).

### 6.1 Go 1.25 upgrade — DEP-01, DEP-02 and STD-01

Applied on 2026-08-24, after the CI `security` job failed on DEP-01 and DEP-02 and made the
trade-off concrete. Every one of these three findings had the same single blocker: the Go
1.22 pin. Raising it resolves all three at once.

| Change | From | To |
|--------|------|-----|
| `go.mod` Go directive | `go 1.22.0` | `go 1.25.0` |
| CI `build` and `coverage` jobs | `go-version: "1.22"` | `go-version: "1.25"` |
| `Dockerfile` build image | `golang:1.22-alpine` | `golang:1.25-alpine` |
| `github.com/jackc/pgx/v5` | v5.7.4 | v5.10.0 (advisory fixed in 5.9.2) |
| `golang.org/x/text` | v0.22.0 | v0.41.0 (advisory fixed in 0.39.0) |
| `golang.org/x/crypto` | v0.31.0 | v0.55.0 |
| `golang.org/x/sync` (indirect) | v0.11.0 | v0.22.0 |

**Evidence**:

- `govulncheck` v1.7.0 under `GOTOOLCHAIN=go1.25.14` — the toolchain `actions/setup-go`
  with `go-version: "1.25"` resolves to — reports **`No vulnerabilities found`**, against 28
  reachable before. The one remaining advisory sits in a module that is required but never
  called.
- `gosec` v2.28.0 over the upgraded tree: `Issues: 0`.
- `go build ./...`, `go vet ./...` and `go test ./...` all pass; pgx v5.10.0 needed no code
  change.
- `scripts/coverage.sh`: 80.0% / 88.3% / 83.6%, gate green — the upgrade moved no coverage
  figure.

**Note on standard-library findings and toolchain drift**: `govulncheck` reports standard
library advisories relative to the toolchain running it. `actions/setup-go` with
`go-version: "1.25"` always installs the newest 1.25.x patch, so the job self-heals shortly
after each Go security release. In the window between a release and its availability, the
job will go red — correctly, since the binary being built does carry the unpatched library.

## 7. Residual risks

| ID | Risk | Justification | Residual risk if not acted on |
|----|------|---------------|-------------------------------|
| ~~R1~~ | ~~DEP-01, DEP-02 unpatched~~ | **Resolved 2026-08-24** by the Go 1.25 upgrade (§6.1) | None — both modules are on fixed versions |
| ~~R2~~ | ~~26 standard-library vulnerabilities (STD-01)~~ | **Resolved 2026-08-24** by the Go 1.25 upgrade (§6.1) | Residual only as toolchain drift: the CI job goes red between a Go security release and its availability in `setup-go`, which is the correct signal |
| R3 | `sslmode=disable` (CFG-01) | The repository configures only the local environment | Credentials and query traffic in clear text if the setting reaches a deployed environment |
| R4 | Development credentials versioned (CFG-02) | Documented dev-only fixtures; tests need deterministic credentials | Account takeover if the same values are ever reused outside local development |
| R5 | Customer and order-creation routes unauthenticated (ARCH-01) | Open decision, `CLAUDE.md` §17.2 | Personal data readable and writable without authentication |
| R6 | No role-based authorization (ARCH-01) | Not specified for the MVP, `CLAUDE.md` §13 | Any valid token performs any administrative operation |
| R7 | No migration tool | Out of scope; `CLAUDE.md` §14 records it as undefined | `docs/schema.sql` is entirely `CREATE TABLE IF NOT EXISTS`, so it cannot alter an existing database. Observed in practice during this analysis: a local volume missing `service_order_tracking_tokens` and the `quotes.version` / `sent_at` / `sent_version` columns produced 52 integration-test failures. CI is unaffected (fresh database per run); shared environments would silently drift |
| R8 | Two competing error envelopes (`httpx` vs `apierror`) | Open decision, `CLAUDE.md` §17.1 | Inconsistent error contract across features. No data exposure — both envelopes were verified to emit generic messages |

**Recommended order of action**: R1 and R2 were the top two and are now resolved — raising
Go to 1.25 removed all 28 reachable vulnerabilities in one change. **R5 is now the highest
open risk**: it is the only remaining finding that exposes personal data with no attacker
sophistication at all.

## 8. Secrets review (AC11)

Method: `git ls-files` filtered for `.env`, `secret`, and `credential`; full-tree grep for
the known development credentials; `gosec`'s credential rules (G101 hardcoded credentials)
over all 84 Go files.

Results:

- No `.env` file is versioned; `.gitignore` excludes it, along with `bin/`, and now
  `coverage/` and `security/`.
- The only versioned environment file is `.env.example`, which carries placeholder values.
- No API key, private key, JWT signing secret, or production credential is present in the
  repository.
- `gosec` reported no G101 (hardcoded credentials) finding.
- The development credentials described in CFG-02 are present and accepted as documented
  fixtures.

Conclusion: no secret is versioned.

## 9. Log and error-response review (RNF08, AC12)

**Method**: manual audit of every log call site and every error-mapping function, followed
by an automated regression test.

**Logs** — six call sites: `auth/handler.go:54,77`, `service-catalog/handler.go:191`,
`httpx/respond.go:24`, and `cmd/api/main.go:47,53,145` (plus the new health-encode log from
SAST-03). Every one logs an error value or a port number. None logs a request body, a
password, a hash, or a token. `auth/handler.go:53` carries an explicit comment recording
that the error it logs carries no credential (BR5, `specs/auth/requirements.md`).

**Error responses** — each feature maps its sentinel errors explicitly and falls through to
a generic envelope (`apierror.Internal("unexpected error")`, or `httpx.Error` for `auth`).
No driver error, SQL fragment, or connection string reaches a client. The `err.Error()`
values that do appear in responses are domain validation messages such as "unit price
cannot be negative" — no user data, no infrastructure detail.

**Automated enforcement**: `internal/handlers_test/sensitive_data_test.go` drives six error
paths — failed login, missing token, tampered token, improper tracking access, an internal
500, and a direct comparison against the administrator's stored bcrypt hash read from the
database — and asserts that no response contains a password, a bcrypt hash, a JWT, a
tracking token, a connection string, or a SQL fragment. The audit above is therefore a
regression test, not a one-time review.

## 10. Conclusion

The three requirements this report covers are met and verifiable:

- **RNF06**: every critical domain is at or above 80% (80.7% / 88.9% / 83.6%), enforced by
  `scripts/coverage.sh` in CI so a regression breaks the build.
- **RNF08**: no sensitive data reaches logs or error responses, verified by audit and held
  by an automated test.
- **RNF09**: SAST and dependency analysis run on every push, pinned to exact tool versions,
  publishing their raw output as artifacts.

All three SAST findings were fixed with evidence; `gosec` reports zero issues.

The dependency and standard-library findings shared a single blocker — **the Go 1.22 pin
blocked every available fix**, accounting for all 28 reachable vulnerabilities. This was
first reported as the recommended follow-up rather than done unilaterally, since changing
the project's Go version is a decision for the team. The team took that decision the same
day, after the CI `security` job failed on it, and the upgrade to Go 1.25 was applied
(§6.1): `govulncheck` now reports **no vulnerabilities** under the toolchain CI builds with.

What remains open is not a scanner finding but an architectural one: **the customer routes
and the service-order creation route are still unauthenticated (R5), and there is no
role-based authorization anywhere (R6)**. Both are recorded open decisions in `CLAUDE.md`
§17 and §13. R5 is now the highest open risk in this report — personal data reachable
without a token needs no sophistication to exploit, and closing it is a one-line change per
route once the decision is made.
