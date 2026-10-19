# Design — Database Migration Tooling

Satisfies: `requirements.md` (revised 2026-10-19, FP-33). Tool: `golang-migrate` `v4.20.1`
(`requirements.md` §7.1).

## 1. Tool shape — a pinned container image, not a `go.mod` dependency

`golang-migrate` runs from the official image **`migrate/migrate:v4.20.1`** (Alpine-based,
`ENTRYPOINT ["migrate"]`), the same "pin an exact version, keep it out of `go.mod`" posture
`CLAUDE.md` §7 uses for `gosec`/`govulncheck`. Nothing is installed on the host beyond
Docker, which the project already requires. The pin lives in two files, kept in sync by a
comment in each:

- `scripts/migrate.sh` — `MIGRATE_IMAGE` default (used by the `Makefile`, CI, and anyone
  running the script by hand);
- `docker-compose.yml` — the `migrate` service's `image:`.

FP-34's Kubernetes `Job` uses the same tag (§9). Helper containers that need `psql`,
`pg_dump` or `pg_restore` use **`postgres:16-alpine`** (`PG_IMAGE`), the same major version
as the server (`CLAUDE.md` §2), so a dump is never taken with an older client than the
server.

Verified with this exact image before writing this design (2026-10-19):
- the `postgres` driver sends a migration file as one multi-statement query, which Postgres
  runs as a single implicit transaction — a failing statement rolls the whole file back;
- the tool then leaves `schema_migrations` at that version with `dirty = true`, and every
  later `up` exits 1 with `Dirty database version N. Fix and force version.`;
- `up` with nothing pending prints `no change` and exits 0;
- files mounted the way a Kubernetes `ConfigMap` volume mounts them (`..data/` directory plus
  symlinks) are read normally;
- `force` to "no version" must be written `force -- -1` (a bare `-1` is parsed as a flag).

## 2. `migrations/` layout (BR1, BR3, BR4)

```
migrations/
├── 0001_init.up.sql     → guard (BR4) + the Fase 1 schema (BR3)
├── 0001_init.down.sql   → drops everything 0001 creates, in reverse dependency order
├── 0002_service_order_requested_products.up.sql    (FP-27, already present)
└── 0002_service_order_requested_products.down.sql
```

### 2.1 `0001_init.up.sql`

- **Guard first.** A `DO` block raises an exception when `public` already has any table
  other than `schema_migrations`. The file is one transaction (§1), so the refusal changes
  nothing; the message names the adoption command (§5). Without the guard the baseline
  would *succeed* on a Fase 1 database — `docs/schema.sql` is written with
  `IF NOT EXISTS`/`duplicate_object` handlers throughout — and record version 1 with no
  validation at all, which BR5 forbids.
- **Then the Fase 1 schema**, i.e. `docs/schema.sql` at commit `0e6f2ab` (its last change
  before `0002`), statement for statement. Only the file header comment differs.

### 2.2 `0001_init.down.sql`

Drops, without `CASCADE` (an unexpected dependent object — e.g. `0002`'s table when `0002`
was not reverted first — must fail the revert rather than vanish silently): the
`updated_at` triggers' function, every table (children before parents), then every enum
type. `pgcrypto` is left in place: other objects may use it, and dropping an extension is
unnecessary for a clean re-apply (`CREATE EXTENSION IF NOT EXISTS`).

### 2.3 Rules for every future migration

- Created with `scripts/migrate.sh create <name>` (`-seq -digits 4`, so numbering stays
  `NNNN`).
- No explicit `BEGIN`/`COMMIT`, and nothing that cannot run in a transaction
  (`CREATE INDEX CONCURRENTLY`, `VACUUM`, …) — otherwise BR6's atomicity is lost.
- Never edit a migration that was applied anywhere; add a new one (BR1).
- Update `docs/schema.sql` in the same change (BR2); `scripts/migrate.sh verify` (§8) fails
  if they diverge.

## 3. `docs/schema.sql` (BR2)

Stays the full, human-readable schema of the latest version, but is **no longer mounted in
`docker-entrypoint-initdb.d`** and no environment executes it to build a database. It stays
executable on purpose: `verify` (§8) builds a reference database from it and compares that
database's `pg_dump --schema-only` with the one the migration chain produces. Its header
comments are updated to say so.

## 4. Local workflow (BR6–BR9)

### 4.1 Docker Compose

```
db (healthy) ──▶ migrate (one-shot, must exit 0) ──▶ api
                              └──────────────────▶ seed (one-shot, nothing waits on it)
```

- `db`: the two `docker-entrypoint-initdb.d` mounts are removed (BR8).
- `migrate`: `migrate/migrate:v4.20.1`, `./migrations` mounted read-only, command
  `-path=/migrations -database postgres://…@db:5432/…?sslmode=disable up`,
  `depends_on: db: service_healthy`, no restart policy.
- `seed`: `postgres:16-alpine` running `psql -v ON_ERROR_STOP=1 -f docs/seed.sql` against
  `db`, `depends_on: migrate: service_completed_successfully`, no restart policy. It runs on
  every `docker compose up` (`requirements.md` §7.3); `docs/seed.sql` is a single
  transaction with `ON CONFLICT (id) DO NOTHING`, so a re-run adds back only sample rows that
  were deleted, and a conflict on another unique key fails the seed as a whole without
  touching the API.
- `api`: `depends_on` gains `migrate: service_completed_successfully`, so a failed migration
  makes `docker compose up` fail with `service "migrate" didn't complete successfully` and
  the API is never started (AC7).

An existing Fase 1 volume therefore stops at the `migrate` step on the first `up` after this
change (the guard, §2.1), with the message pointing at §5.

### 4.2 `scripts/migrate.sh`

One script, matching `scripts/coverage.sh`/`scripts/security-scan.sh` (bash,
`set -euo pipefail`, usage in the header). Every tool runs in a container.

| Command | What it does | Needs |
| --- | --- | --- |
| `up [N]` | applies all (or the next N) pending migrations | `DATABASE_URL` |
| `version` | prints the current version and dirty flag | `DATABASE_URL` |
| `create <name>` | creates the next `NNNN_<name>.{up,down}.sql` pair, owned by the caller | — |
| `down <N\|all>` | reverts; refuses unless `MIGRATE_CONFIRM_DOWN=yes` (BR9) | `DATABASE_URL` |
| `baseline` | adopts an existing database (§5) | `DATABASE_URL` |
| `verify` | self-contained checks on throwaway containers (§8) | — |

Environment: `DATABASE_URL`; `MIGRATE_NETWORK` (Docker network the tool containers join to
reach `DATABASE_URL`, default `host`); `MIGRATIONS_DIR` (default `migrations`);
`BACKUP_DIR` (default `backups`, git-ignored); `MIGRATE_IMAGE`, `PG_IMAGE` (pins, §1).
There is deliberately **no raw `force`** command: the only way the script records a version
is `baseline`, after validation (BR5). The `migrate` CLI itself remains available for an
operator who needs it.

### 4.3 `Makefile`

`migrate-up`, `migrate-version`, `migrate-create name=…`, `migrate-down` (one step, needs
`MIGRATE_CONFIRM_DOWN=yes`), `migrate-baseline`, `migrate-verify`, `bootstrap-admin`. The
targets that talk to the Compose database join the `db` container's network and use
`db:5432`, so they work the same on Linux, macOS and Windows (no `--network host`, which
Docker Desktop does not enable by default). `make schema` (drop and recreate) now re-applies
the migrations instead of the removed init file; `make reset` no longer runs the seed
separately (Compose does).

## 5. Adopting an existing database — `scripts/migrate.sh baseline` (BR5, AC4, AC5)

1. **Current state.** If `schema_migrations` exists and is clean, the database is already
   managed: print its version, exit 0, change nothing. If `public` has no table at all,
   print that there is nothing to adopt (use `up`) and exit 1.
2. **Backup.** `pg_dump -Fc` to `BACKUP_DIR/<database>-<UTC timestamp>.dump`; check the file
   is non-empty and that `pg_restore --list` can read it. Any failure stops here.
3. **Target fingerprint.** `pg_dump --schema-only --no-owner --no-privileges
   --exclude-table=public.schema_migrations`, normalized by dropping comment lines, blank
   lines and `\restrict`/`\unrestrict` lines (random per dump since Postgres 16.10).
4. **Candidate fingerprints.** A throwaway `postgres:16-alpine` container; the same dump for
   version 0 (empty) and then after `goto V` for every version in `MIGRATIONS_DIR`.
5. **Decision.** Exactly one candidate equal to the target → record it on the target
   (`force V`, or `force -- -1` for version 0) and print the next step (`up`). Zero or
   several → print a unified diff against the closest candidate (fewest differing lines),
   keep the backup, record nothing, exit 1.

Because it records "the version this schema provably is", the same command is also the
recovery path after a failed migration (§6) and after the guard refused a Fase 1 database
(§4.1) — in both cases `schema_migrations` is dirty and step 1 lets it through.

A database created before the 2026-08-26 status rename has Portuguese `service_order_status`
labels, matches no version, and is refused (AC5); `docs/schema.sql` documents the
`ALTER TYPE … RENAME VALUE` statements that bring it to version 1 in place.

## 6. Failure and dirty-state recovery (BR6, AC6)

Runbook (README):
1. The failing migration N rolled back as a whole (§1), so the database is still at N−1,
   marked dirty. Every `up` refuses, and so does every deploy step that waits on it.
2. Fix the cause. If N was never applied successfully anywhere, fix the file; otherwise
   leave it and add a corrective migration.
3. `scripts/migrate.sh baseline` — backs up, proves the schema equals version N−1, records
   N−1 clean.
4. `scripts/migrate.sh up`.

No step deletes data. If `baseline` finds no match — only possible if a migration broke the
transaction rule (§2.3) — restore from the backup or repair by hand until it does.

## 7. Administrative user bootstrap (BR10, BR11)

### 7.1 Code

- `internal/features/auth/repository.go` — `CreateIfAbsent(ctx, name, email, passwordHash)
  (code int64, created bool, err error)`:
  `INSERT INTO users (name, email, password_hash) VALUES ($1, $2, $3) ON CONFLICT (email)
  DO NOTHING RETURNING code` (`users.email` is `UNIQUE`). No returned row means the e-mail
  already existed.
- `internal/features/auth/bootstrap.go` — `BootstrapAdmin(ctx, AdminCreator,
  AdminBootstrapInput) (AdminBootstrapResult, error)`: trims and validates the e-mail
  (`net/mail.ParseAddress`, and the parsed address must equal the input — no display name),
  checks the password length (12–72 bytes), defaults the name to `Administrator`, hashes
  with `bcrypt.DefaultCost` (10, the cost `docs/seed.sql` uses), calls `CreateIfAbsent`.
  `ErrInvalidAdminEmail`/`ErrInvalidAdminPassword` never include the input. It lives in the
  `auth` slice because `auth` owns the `users` table; it is a function with its own
  one-method interface rather than a `Service` method, since it needs neither `UserFinder`
  nor `TokenIssuer`.
- `internal/shared/config/config.go` — `LoadAdminBootstrap() (AdminBootstrapConfig, error)`
  reads `DATABASE_URL`, `ADMIN_EMAIL`, `ADMIN_PASSWORD` (required) and `ADMIN_NAME`
  (optional). It does not read or require `JWT_SECRET`: the command issues no token.
- `cmd/bootstrap-admin/main.go` — thin entrypoint (`CLAUDE.md` §9.4): load config, open the
  pool (`internal/shared/database`), call `auth.BootstrapAdmin`, log only the outcome
  (`created (code N)` or `already exists — left unchanged`), exit 1 on any error. A missing
  `users` table surfaces as an error that says to run the migrations first.

### 7.2 Packaging and use

- `Dockerfile` builds both binaries (`/api`, `/bootstrap-admin`) into the same image; the
  default command stays `/api`. FP-32 owns the rest of the Dockerfile.
- Local: `make bootstrap-admin` runs `/bootstrap-admin` in a one-off `api` container with
  `ADMIN_EMAIL`/`ADMIN_PASSWORD`/`ADMIN_NAME` taken from the caller's environment — not
  needed for everyday development, where the seed already provides users.
- Cluster: a `Job` running the API image with command `/bootstrap-admin` (§9).

## 8. CI and `verify` (BR2, BR6, BR7, AC9)

- **`coverage` job:** replaces `psql -f docs/schema.sql` with `scripts/migrate.sh up`
  (`MIGRATE_NETWORK=host` reaches the job's Postgres service on `localhost:5432`), then the
  seed as before — the integration tests now run on a migrated database.
- **New `migrations` job:** `scripts/migrate.sh verify`, which starts its own throwaway
  Postgres and checks, failing on the first mismatch:
  1. **AC1** — `up` on an empty database ≡ `docs/schema.sql` applied to another one;
  2. **AC2** — a second `up` prints `no change`, exits 0, and the schema is unchanged;
  3. **AC3** — `down all` leaves no table but `schema_migrations` and no enum type; `up`
     again ≡ reference;
  4. **AC4** — a Fase 1-style database (`0001_init.up.sql` run directly with `psql`, which
     records no version, plus `docs/seed.sql`): `up` is refused by the guard and changes
     nothing; `baseline` records version 1; `up` reaches the latest version; the schema
     equals the reference; every table's row count is unchanged;
  5. **AC5** — the same database with one status label renamed back to Portuguese:
     `baseline` exits 1 and records nothing;
  6. **AC6** — the chain plus a deliberately failing extra migration: `up` exits non-zero,
     the failing migration's table does not exist, the next `up` is refused (dirty);
     `baseline` with the real chain records the last good version and `up` reports
     `no change`.

`verify` never touches `DATABASE_URL`. It runs the same commands a developer and the
deploy pipeline run (BR7).

## 9. Contract for FP-34 (Kubernetes) and FP-36 (CD)

What this feature provides, and what the manifests/pipeline are expected to do with it:

| Item | Value |
| --- | --- |
| Migration image | `migrate/migrate:v4.20.1` (the `v4.18.1` still written in `specs/kubernetes-deployment/design.md` §6 and `specs/ci-cd-pipeline/design.md` is superseded) |
| Migration args | `["-path=/migrations", "-database", "$(DATABASE_URL)", "up"]` |
| Migration files | `ConfigMap` generated from `migrations/` (flat directory; the `ConfigMap` symlink layout was verified, §1) — regenerate whenever a migration is added |
| `DATABASE_URL` | from the API `Secret` (`automotive-workshop-api-secrets`) |
| Migration `Job` | unique name per version/run; `backoffLimit: 0` (a retry cannot succeed — a failed migration leaves the database dirty, §6); wait for `condition=complete`; on failure, stop — no rollout |
| Bootstrap `Job` | API image, same tag as the `Deployment`; `command: ["/bootstrap-admin"]`; env `DATABASE_URL`, `ADMIN_EMAIL`, `ADMIN_PASSWORD` (and optional `ADMIN_NAME`) from a `Secret`; runs after the migration `Job` completes and before the smoke test; idempotent, so it can run on every deploy |
| Seed | `docs/seed.sql` is **never** applied in the cluster (BR11) |
| Order | migration `Job` complete → bootstrap `Job` complete → `Deployment`/`Service`/`HPA` → rollout → smoke test (login with `ADMIN_EMAIL`/`ADMIN_PASSWORD`) |
| Existing cluster database | adopt it with `scripts/migrate.sh baseline` through a port-forward before the first migration `Job`, never by forcing a version |

## 10. Documentation

- `README.md` — "Migrações do banco de dados": how the Compose flow works now; adding,
  applying and verifying a migration; adopting a Fase 1 database; recovering from a failure;
  bootstrapping the administrator; reverting only on disposable databases.
- `CLAUDE.md` — §3 (layout), §5 (run), §6 (tests on a migrated database), §7 (the new CI
  job), §13 (bootstrap credentials), §14 (replaces "to be defined" with this workflow).
- `specs/architecture.md` — the second entrypoint and the migration-based persistence
  setup.
- `.env.example` — `ADMIN_*` documented, commented out (no default credential).
- `.gitignore` — `backups/`.
- `internal/handlers_test` package comment and `scripts/coverage.sh`'s help text — the test
  database is built with migrations.

## 11. Testing strategy

- **Go unit tests** — `auth/bootstrap_test.go` (fake `AdminCreator`): creation with a
  hash that verifies against the password and the default name; existing e-mail →
  `Created == false`; invalid e-mails and passwords rejected before the creator is called;
  creator errors propagated; no error message contains the password.
  `config/config_test.go`: `LoadAdminBootstrap` required/optional variables.
- **Go integration test** — `internal/handlers_test/auth_bootstrap_test.go` (skips without
  `DATABASE_URL`, stdlib only like the other auth tests): bootstrap a unique e-mail on the
  real database, log in through the auth handler, bootstrap again with another password →
  unchanged and the first password still logs in; the user is deleted on cleanup.
- **Operational** — `scripts/migrate.sh verify` (§8) locally and in CI; plus, recorded as
  FP-33 evidence: Compose from an empty volume, Compose over a real Fase 1 volume built from
  commit `0e6f2ab` with extra data (guard → `baseline` → `up`, data preserved), and the
  bootstrap command against a migrated database with no seed.

## 12. Traceability

| Requirement | Design |
| --- | --- |
| BR1 ordered files | §2, §2.3 |
| BR2 schema.sql reference, checked | §3, §8 |
| BR3 baseline = Fase 1 | §2.1 |
| BR4 guard | §2.1 |
| BR5 adoption with proof and backup | §5 |
| BR6 repeatable, atomic, fail-closed | §1, §6, §8 |
| BR7 one chain | §1, §8, §9 |
| BR8 local flow | §4.1 |
| BR9 down only with confirmation | §4.2, §4.3 |
| BR10 bootstrap | §7, §9 |
| BR11 seed stays local | §4.1, §9 |
| AC1–AC6, AC9 | §8 |
| AC7 | §4.1 |
| AC8 | §7, §11 |
| AC10 | §10 |
| AC11 | §11 |
