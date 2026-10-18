# Tasks — Database Migration Tooling

## 1. Spec confirmation

- [ ] Confirm `requirements.md` §7's tool choice (`golang-migrate`) before proceeding —
      `Status: Proposed` becomes `Approved for implementation` once confirmed.

## 2. Baseline migration (`design.md` §2)

- [ ] `migrations/0001_init.up.sql` — copy of `docs/schema.sql`'s current content.
- [ ] `migrations/0001_init.down.sql` — reverse-order `DROP`s for everything `0001_init.up.sql`
      creates.

## 3. Local tooling (`design.md` §1, §4)

- [ ] Pin an exact `migrate/migrate` image tag (`design.md` §4's `MIGRATE_IMAGE`).
- [ ] Add `Makefile` (or `scripts/migrate.sh`) targets: `migrate-up`, `migrate-down`,
      `migrate-create`.

## 4. Verification (`design.md` §7)

- [ ] `migrate-up` against a fresh empty Postgres; `pg_dump --schema-only` compared against
      a `docker compose up -d`-initialized database — confirm equivalence
      (`requirements.md` AC1).
- [ ] `migrate-down` then `migrate-up` again — confirm the resulting schema matches a single
      `migrate-up` (AC2/AC3).
- [ ] Run `migrate-up` twice in a row — confirm the second run is a no-op (AC3).

## 5. Documentation (`design.md` §6)

- [ ] `CLAUDE.md` §14 — replace the "to be defined" note with the resolved workflow.
- [ ] `README.md` — "Database migrations" subsection.

## 6. Verification

- [ ] `go build ./...`, `go vet ./...`, `go test ./...` unaffected.
- [ ] Re-check `requirements.md` §5 AC1–AC6.
