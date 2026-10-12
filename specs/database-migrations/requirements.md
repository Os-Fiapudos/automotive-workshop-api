# Requirements — Database Migration Tooling

Status: **Proposed** (tool choice needs final confirmation — see §7)
Touches: new `migrations/` directory, `docs/schema.sql`/`docs/seed.sql` (relationship
clarified, not rewritten), `go.mod`, `Makefile`/`scripts/`.

## 1. Context

Source: `CLAUDE.md` §14 ("**To be defined**: incremental migration strategy for when the
schema needs to evolve after the volume already exists in shared/production environments")
and `GAPS-FASE-2.md` §3.2/§7 item 9. Today `docs/schema.sql` is applied by Postgres only on
the **initial creation** of the Docker volume
(`docker-entrypoint-initdb.d/01-schema.sql`) — there is no way to apply a schema change to
an already-running database (local-with-existing-volume, or, after Fase 2, the Kubernetes
cluster's `StatefulSet`-backed Postgres, `specs/infrastructure-provisioning/`) other than
`docker compose down -v && docker compose up -d`, which destroys all data. Fase 2's
CI/CD requirement ("Deploy do banco de dados") and Kubernetes deployment both need a real
way to apply schema changes to a persistent database without data loss.

Team decision (2026-10-12): adopt a migration tool plus a Postgres `StatefulSet` (not a
managed database) — see `GAPS-FASE-2.md` §7 item 9. The tool itself (`golang-migrate` vs.
`goose` vs. a plain `psql`-script Job) was not decided; this spec's working choice is
**`golang-migrate`** (§7), the most widely adopted of the three and the one that needs the
least custom tooling (a single static CLI binary, usable from a `Dockerfile`/K8s `Job`
without adding Go code to this project at all — see `design.md` §1).

## 2. User story

> As whoever deploys a schema change to a running environment (a developer locally, or the
> CI/CD pipeline / Kubernetes Job in Fase 2),
> I want an explicit, ordered set of migration files I can apply to an existing database
> without losing data,
> so that `docker compose down -v` is no longer the only way to pick up a schema change.

## 3. Business rules

1. Every schema change from this point forward is expressed as a new, sequentially
   numbered migration file pair (`NNNN_description.up.sql` / `NNNN_description.down.sql`),
   not as an in-place edit of `docs/schema.sql`.
2. `docs/schema.sql` keeps its existing role as the **current, full-schema reference**
   (`CLAUDE.md` §8/§14 — human-readable documentation of the schema as it stands today) and
   keeps being what `docker-entrypoint-initdb.d` applies on a brand-new local volume,
   unchanged. It is kept in sync with the migration history by construction: migration
   `0001` *is* today's `docs/schema.sql` verbatim (§4), and every migration after it is
   manually mirrored into `docs/schema.sql` in the same commit that adds the migration — an
   explicitly accepted, documented duplication (not a hidden one), same posture
   `CLAUDE.md` §8 already takes for the `httpx`/`apierror` duplication.
3. Applying all migrations to a brand-new, empty database produces a schema identical to
   `docs/schema.sql` applied directly — the two paths must never diverge.
4. Every migration is reversible (`.down.sql` undoes exactly what `.up.sql` does) — needed
   for local development rollback; production rollback-by-migration is out of scope (§6).
5. Migrations are idempotent to *run* (the tool tracks which have already been applied, in
   a `schema_migrations` table it manages itself) but each individual `.up.sql`/`.down.sql`
   file is not required to be idempotent on its own (`golang-migrate`'s own model — each
   migration runs exactly once, ever, per database).
6. No application code (`internal/...`) changes — this feature only adds tooling and files
   around the existing schema; `docs/entities.md` is unaffected (it already documents the
   domain, independent of how the schema gets applied).

## 4. Scope

In scope:
- `migrations/0001_init.up.sql` / `.down.sql` — `0001_init.up.sql` is `docs/schema.sql`'s
  current content; `0001_init.down.sql` drops everything it creates, in reverse order.
- Installing `golang-migrate`'s CLI (not as a Go module dependency — see `design.md` §1) for
  local use and for the CI/CD and Kubernetes Job that run it (`specs/ci-cd-pipeline/`,
  `specs/kubernetes-deployment/`).
- A `Makefile` target or `scripts/migrate.sh` wrapping the common commands (`up`, `down`,
  `create <name>`), so a developer does not need to remember `golang-migrate`'s full CLI
  invocation.
- Documentation (README, `CLAUDE.md` §14) of the new workflow: how to add a migration, how
  to apply it locally, and the `docs/schema.sql` mirroring rule (§3 item 2).

Out of scope:
- Any actual schema *change* beyond the `0001_init` baseline — every schema addition this
  Fase 2 effort needs (`service_order_requested_products`, any table a future feature in
  this plan adds) becomes its own numbered migration, written when that feature is
  implemented, not pre-written here.
- Automatic, hands-off production rollback — `.down.sql` files exist for local development
  and for a deliberate, manually-triggered rollback; this feature does not build automatic
  rollback-on-failure into the deploy pipeline (`specs/ci-cd-pipeline/` may add that later,
  not assumed here).
- Rewriting `docs/seed.sql` as a migration — it stays a separate, idempotent
  (`ON CONFLICT DO NOTHING`) script applied after the schema, exactly as today
  (`CLAUDE.md` §5/§14), run manually or by the same Job/script that already knows to run it
  second.

## 5. Acceptance criteria

1. `migrations/0001_init.up.sql` applied to an empty Postgres database produces a schema
   identical to `docker compose up -d`'s current `docs/schema.sql`-driven initialization
   (diffed via `pg_dump --schema-only` on both, or an equivalent comparison).
2. `migrations/0001_init.down.sql` applied after `0001_init.up.sql` leaves the database with
   no application tables/types/extensions remaining.
3. Running the migration tool a second time with no new migration files is a no-op (reports
   "no change," does not error, does not reapply `0001_init`).
4. A local `Makefile`/`scripts/migrate.sh` command applies pending migrations against
   `DATABASE_URL` with one command, documented in the README.
5. `CLAUDE.md` §14's "to be defined: incremental migration strategy" is resolved — updated
   to describe this workflow instead of listing it as open.
6. `go build ./...`, `go vet ./...`, `go test ./...` unaffected (no Go source changes).

## 6. Out of scope

- Automatic rollback orchestration in CI/CD on a failed deploy.
- Zero-downtime / online-schema-change tooling (e.g. expand-contract patterns for
  high-traffic tables) — this project's current scale and Fase 2's explicit requirements
  don't call for it.
- Any multi-database or per-tenant migration strategy.

## 7. Open question — confirm before implementation

**Tool choice.** `golang-migrate` is this spec's working choice (§1) over `goose` (a Go-
native alternative requiring a small Go wrapper or its own CLI) and a plain `psql`-script
Job (no tooling at all, but no "already applied" tracking, no down-migrations, and no
protection against applying the same script twice). `golang-migrate`'s CLI is a single
static binary (available as a Docker image,
`migrate/migrate`), which fits this project's "don't add a Go module dependency unless
necessary" posture (`CLAUDE.md` §12) particularly well — it is used as an external tool
(local binary or container image), not a `go.mod` import, so no new entry is needed in
`go.mod`'s dependency list at all. **Flagged per SDD (`CLAUDE.md` §17) for explicit
confirmation before `tasks.md` is executed** — if a different tool is chosen, only this
section and `design.md` §1's tool-specific commands change.
