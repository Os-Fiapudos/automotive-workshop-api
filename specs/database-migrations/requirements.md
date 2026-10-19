# Requirements — Database Migration Tooling

Status: **Approved for implementation** (2026-10-19 — tool confirmed, see §7)
Jira: **FP-33** — "[Fase 2] Evolução versionada e segura do schema do banco" (planning id
F2-US08, manual requirement R11).
Touches: new `migrations/0001_init.*`, `docker-compose.yml`, `scripts/migrate.sh`,
`Makefile`, `.github/workflows/ci.yml`, `Dockerfile`, a new `cmd/bootstrap-admin`, the
`auth` slice, `internal/shared/config`, and documentation (`README.md`, `CLAUDE.md`,
`specs/architecture.md`, `docs/schema.sql` comments).

## 0. Revision note (2026-10-19)

The first version of this spec (PR #30) predates the FP-33 Jira story and the team's Fase 2
analysis (Confluence "Tech Challenge Fase 2 — Oficina Mecânica | Evolução, Infraestrutura e
Backlog", §2.1 and §15). That analysis flagged three problems with it — Compose would keep
creating the schema through `docker-entrypoint-initdb.d` while `migrate up` created the same
schema again, an existing Fase 1 database had no adoption path, and a freshly deployed
database had no user to log in with. The story's acceptance checklist closes them, so this
revision changes the original in these places (each is called out where it applies):

1. **Baseline content.** `0001_init` is the **Fase 1 schema**, not today's
   `docs/schema.sql`: migration `0002_service_order_requested_products` (FP-27) landed before
   the tooling did, so `docs/schema.sql` already contains it.
2. **Single initialization path.** `docker-entrypoint-initdb.d` no longer applies
   `docs/schema.sql` (the original BR2 kept it there).
3. **Fase 1 database adoption** (new), **failure/dirty-state handling** (new), **one
   migration chain for local, CI and Kubernetes** (new), **`down` only on disposable
   databases** (new).
4. **Administrative user bootstrap** (new). This adds application code — the original BR6
   said this feature would not touch any.

## 1. Context

Until this feature, `docs/schema.sql` was applied by Postgres only on the **initial
creation** of the Docker volume (`docker-entrypoint-initdb.d/01-schema.sql`). There was no
way to apply a schema change to a database that already holds data other than
`docker compose down -v`, which destroys it. Fase 2 needs one: the CI/CD requirement
("deploy do banco de dados") and the Kubernetes deployment both run against a persistent
Postgres (`specs/infrastructure-provisioning/`, `specs/kubernetes-deployment/`), and FP-27
already shipped the first incremental schema change (`migrations/0002_*`), applied by hand
until now (`CLAUDE.md` §14).

Team decision (2026-10-12): adopt a migration tool plus a Postgres instance that is not a
managed service (`GAPS-FASE-2.md` §7 item 9). The tool is confirmed in §7.

## 2. User stories

> As a developer, I want to apply traceable, ordered migrations without recreating the
> database, so that the system evolves while existing data is preserved. *(FP-33)*

> As whoever runs a database created during Fase 1, I want an explicit adoption procedure
> that proves my schema matches a known version and backs it up first, so that the
> migration history starts from where my database really is — without re-running the
> `CREATE`s and without trusting an unverified version number.

> As the deploy pipeline (CI/CD, Kubernetes), I need an administrative user on a freshly
> created database, created explicitly from secrets I hold, so that the smoke test and the
> demonstration can log in — the API has no other way to create one.

## 3. Business rules

1. **Ordered migration files.** Every schema change is a new, sequentially numbered pair
   `NNNN_description.up.sql` / `NNNN_description.down.sql` in `migrations/`. A migration
   that has been applied anywhere is never edited afterwards; a correction is a new
   migration.
2. **`docs/schema.sql` is the reference, not an initialization path.** It documents the
   full schema that results from applying every migration, and is updated in the same
   change that adds a migration. No environment executes it to create a database any more —
   migrations are the only initialization path. That the two never diverge is checked
   automatically (AC1), not left to discipline.
3. **Baseline = Fase 1.** `0001_init` reproduces exactly the schema a Fase 1 database has
   (`docs/schema.sql` as of commit `0e6f2ab`, 2026-08-26 — its last change before
   `0002`). A Fase 1 database therefore corresponds to version 1.
4. **The baseline never runs over an existing schema.** `0001_init` refuses to run on a
   database that already contains tables (other than the tool's own version table) and
   fails without changing anything.
5. **Adopting an existing database requires proof and a backup.** A database that already
   has the schema gets its version recorded only after (a) a backup was taken and verified
   readable, and (b) its schema was compared against the schema each migration version
   produces and matched exactly one of them. The version is then recorded without executing
   any migration. With no match, or more than one, nothing is recorded and the differences
   are shown. No migration version is ever recorded ("forced") without that validation.
6. **Repeatable, atomic, fail-closed.** Running the migrations again with nothing pending
   changes nothing and succeeds. Each migration is applied atomically: if it fails, the
   database is left exactly as it was before that migration, the failure is reported with a
   non-zero exit, and the database is marked as needing attention so that no later run —
   and no deploy step that depends on it — proceeds until it is recovered. Recovery is
   documented and never requires deleting data. To keep the atomic guarantee, a migration
   must not contain statements that cannot run inside a transaction.
7. **One chain everywhere.** Local commands, CI and the Kubernetes Job apply the same
   `migrations/` files with the same tool and the same pinned tool version.
8. **Local flow is explicit.** With Docker Compose, migrations run as their own step before
   the API starts, and the API does not start if they fail. Postgres' own initialization
   (`docker-entrypoint-initdb.d`) does not create the schema. The development seed
   (`docs/seed.sql`) runs after the migrations as its own step; a seed failure does not
   prevent the API from starting.
9. **Reverting is for disposable databases.** Every migration has a `.down.sql` that
   undoes it, used for local development and verification on throwaway databases. The
   project's commands refuse to revert without an explicit confirmation. In an environment
   whose data matters, a bad migration is handled with a backup and a new, corrective
   migration, not with `down`.
10. **Administrative user bootstrap.** An explicit command creates an administrative user
    from credentials supplied at run time (environment variables; a Kubernetes `Secret` in
    the cluster), never from values stored in Git.
    - The password is hashed with bcrypt inside the command; only the hash reaches the
      database, and neither the password nor the hash is ever printed (auth BR1/BR5).
    - It is idempotent: if a user with that e-mail already exists, nothing changes — in
      particular, the existing password is not replaced — and the command succeeds.
    - Invalid input fails with a non-zero exit before anything is written: the e-mail must be
      a plain address; the password must be 12 to 72 bytes long (72 is bcrypt's limit).
    - The user's display name is optional and defaults to "Administrator".
11. **The development seed stays local.** `docs/seed.sql` (which carries well-known
    development passwords) is for local/development databases and CI test fixtures. A
    deployed environment gets its administrative user from rule 10 instead.

## 4. Scope

In scope:
- `migrations/0001_init.up.sql` / `.down.sql` (BR3, BR4, BR9). `0002` already exists.
- The pinned tool, used as an external container image, not a Go module dependency (§7).
- `scripts/migrate.sh` (apply, inspect, create, revert with confirmation, adopt an existing
  database, self-verify) and `Makefile` shortcuts (BR5, BR6, BR9).
- Docker Compose: a migration step and a seed step, and no schema in
  `docker-entrypoint-initdb.d` (BR8).
- CI: the integration-test database is built by the migrations, and a job verifies the
  migration chain (BR2, BR6, BR7).
- The administrative bootstrap command and its packaging in the API image (BR10).
- Documentation: README, `CLAUDE.md` §5/§6/§7/§14, `specs/architecture.md`, and the
  integration contract for the Kubernetes Job and the CD pipeline (design §9).

Out of scope:
- **The Kubernetes manifests** (`k8s/` migration and bootstrap `Job`s, the `ConfigMap`,
  `Secret`) — FP-34 (`specs/kubernetes-deployment/`). This feature provides what they
  consume and documents the contract.
- **The CD deploy job** (running those Jobs in the pipeline, ordering them before the
  rollout) — FP-36 (`specs/ci-cd-pipeline/`).
- Container hardening beyond adding the bootstrap binary to the image — FP-32
  (`specs/containerization/`).
- Any schema change beyond the baseline (each feature writes its own migration).
- Automatic rollback on a failed deploy; zero-downtime/online schema changes; multiple
  databases or tenants.
- Rotating an existing user's password (BR10 never modifies an existing user).

## 5. Acceptance criteria

1. **Empty database.** Applying every migration to an empty database produces a schema
   identical to `docs/schema.sql` applied directly (compared with `pg_dump --schema-only`).
2. **Re-run.** Applying the migrations again reports no change, exits successfully, and
   modifies nothing.
3. **Round trip (disposable database).** Reverting every migration leaves no application
   table or type behind; applying them again yields the same schema as AC1.
4. **Fase 1 database with data.** On a database built the Fase 1 way (`0001` schema plus
   the seed, no version table): applying the migrations is refused by the baseline guard
   and changes nothing; the adoption procedure takes a backup, proves equivalence with
   version 1 and records it; applying the migrations then adds `0002`; every pre-existing
   row is still there.
5. **Non-equivalent existing database.** On an existing database whose schema matches no
   version (e.g. one created before the 2026-08-26 status rename), adoption refuses, shows
   the differences and records nothing.
6. **Controlled failure.** A failing migration makes the run exit non-zero, leaves the
   database unchanged by that migration, and blocks the next run; the documented recovery
   brings the database back to a clean, correct version without losing data.
7. **Compose.** `docker compose up -d` on a new volume runs the migrations, then the seed,
   then the API, which becomes reachable and accepts the seeded administrator's login. If the
   migration step fails, the API is not started.
8. **Bootstrap.** On a migrated database with no users, the command creates the
   administrator and a login with those credentials succeeds; running it again changes
   nothing and the original password still works; missing or invalid credentials make it
   exit non-zero without writing; its output never contains the password or the hash.
9. **CI.** The integration tests run against a database built by the migrations, and a CI
   job checks AC1–AC6 on every push and pull request.
10. **Documentation.** The README explains how to add, apply and verify migrations, how to
    adopt a Fase 1 database, how to recover from a failure, and how to bootstrap the
    administrator; `CLAUDE.md` §14 no longer lists the migration strategy as "to be
    defined"; the contract for FP-34/FP-36 is written down.
11. `go build ./...`, `go vet ./...` and `go test ./...` pass.

## 6. Out of scope

See §4.

## 7. Decisions

1. **Tool — `golang-migrate`, confirmed 2026-10-19**, pinned to **`v4.20.1`** (latest
   release at that date, published 2026-09-09; no breaking changes since the `v4.18.1` the
   first version of this spec named). Chosen over `goose` (needs a Go wrapper or its own
   CLI, and the Kubernetes/CI specs would need rewriting) and over plain `psql` scripts (no
   applied-version tracking, no down migrations, no protection against applying a script
   twice — BR6 could not be met). It runs from the official `migrate/migrate` container
   image, so nothing is added to `go.mod` (`CLAUDE.md` §12).
2. **Bootstrap — a Go command in the API image, decided 2026-10-19.** Chosen over a `psql`
   script hashing with pgcrypto's `crypt()` (the `docs/seed.sql` approach) because that
   would send the plaintext password to the database inside the SQL text, where a failed
   statement can end up in the server log. The command hashes in-process and sends only the
   hash, through a parameterized query.
3. **Local seed — a Compose step on every `up`, decided 2026-10-19.** Keeps today's
   behaviour (a new local environment is ready to log in after `docker compose up -d`);
   `docs/seed.sql` was already idempotent (`ON CONFLICT DO NOTHING`).
