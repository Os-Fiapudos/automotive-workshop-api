# Design — Database Migration Tooling

Satisfies: `requirements.md` (all sections). Assumes `golang-migrate` per §7's working
choice.

## 1. Tool shape — CLI/container, not a `go.mod` dependency

`golang-migrate` ships a static CLI binary and an official Docker image
(`migrate/migrate:<version>`, pinned to an exact tag — same "pin exact versions, run via
`go run <module>@<version>` or, here, a pinned container image, so it never enters
`go.mod`" posture `CLAUDE.md` §7 already uses for `gosec`/`govulncheck`). Local developers
install the CLI (documented in the README, e.g. `brew install golang-migrate` /
the project's `scripts/` wrapper calling the pinned Docker image so no host install is
required at all — the Docker-image route is preferred, since it needs nothing beyond Docker,
which this project already requires). CI/CD and the Kubernetes migration Job (§5) both use
the pinned `migrate/migrate` image directly, exactly like `gosec`'s CI job already pins a
tool version rather than trusting `latest`.

## 2. `migrations/` layout

```
migrations/
├── 0001_init.up.sql     → docs/schema.sql's current content, verbatim
└── 0001_init.down.sql   → DROP everything 0001_init.up.sql creates, in reverse dependency
                             order (tables referencing other tables first, then the
                             referenced tables, then the enum types, then the pgcrypto
                             extension left in place — dropping an extension other database
                             objects might still use is unnecessary and riskier than leaving
                             it)
```

Every future schema change (a new feature's table, a new column) adds
`migrations/NNNN_<description>.up.sql`/`.down.sql`, `NNNN` being the next sequential number
— `golang-migrate`'s own convention, requiring no project-specific numbering scheme beyond
"increment by one."

## 3. `docs/schema.sql` mirroring (`requirements.md` BR2)

`docs/schema.sql` is not deleted or replaced by `migrations/` — it keeps being what
`docker-entrypoint-initdb.d/01-schema.sql` applies on a brand-new local volume (unchanged
behavior, `CLAUDE.md` §5/§14), and keeps being the single human-readable reference a reader
opens to see "the schema, as of today." The rule going forward: a PR that adds
`migrations/NNNN_*.sql` also updates `docs/schema.sql` to the resulting full schema, in the
same commit — this is a manual, explicitly documented duplication (`requirements.md` BR2),
not an automatic generation step, because this project has no schema-dump tooling wired in
yet and adding one is a bigger decision than this feature's scope. `tasks.md` includes a
checklist item for this on every future migration, and `CLAUDE.md` §14's updated text (§6
below) states the rule explicitly so it is not forgotten.

## 4. Local workflow

`Makefile` (new, or `scripts/migrate.sh` if the project prefers no `Makefile` — match
whatever convention `scripts/coverage.sh`/`scripts/security-scan.sh` already set):

```makefile
MIGRATE_IMAGE := migrate/migrate:v4.18.1

migrate-up:
	docker run --rm --network host -v $(PWD)/migrations:/migrations $(MIGRATE_IMAGE) \
	    -path=/migrations -database "$$DATABASE_URL" up

migrate-down:
	docker run --rm --network host -v $(PWD)/migrations:/migrations $(MIGRATE_IMAGE) \
	    -path=/migrations -database "$$DATABASE_URL" down 1

migrate-create:
	docker run --rm -v $(PWD)/migrations:/migrations $(MIGRATE_IMAGE) \
	    create -ext sql -dir /migrations -seq $(name)
```

(`--network host` so the container can reach `localhost:5432` the same way a locally-run Go
process does; on a non-Linux Docker Desktop host, `host.docker.internal` is substituted in
`DATABASE_URL` instead — documented in the README's troubleshooting note, not a tooling
difference.)

## 5. CI/CD and Kubernetes integration (cross-references, not implemented by this feature)

- `specs/ci-cd-pipeline/`'s deploy step runs `migrate ... up` (the pinned image, §1) against
  the target database before/as part of rolling out a new API image — referenced there, not
  duplicated here.
- `specs/kubernetes-deployment/`'s migration `Job` manifest mounts `migrations/` (via a
  `ConfigMap` or, more simply, bakes `migrations/` into a small dedicated image built from
  the pinned `migrate/migrate` base — decided in that spec) and runs once per deploy,
  before the API `Deployment`'s rollout — referenced there, not duplicated here.

## 6. Documentation updates

- `CLAUDE.md` §14: replace "**To be defined**: incremental migration strategy..." with a
  short description of this workflow (golang-migrate, `migrations/`, the `docs/schema.sql`
  mirroring rule) and a pointer to this spec.
- `README.md`: a "Database migrations" subsection — how to add one, how to apply one
  locally, linking the `Makefile` targets (§4).

## 7. Testing strategy

- No Go test changes (no Go code touched). Verification is operational:
  `migrate-up` against a fresh empty Postgres, `pg_dump --schema-only` compared against a
  `pg_dump --schema-only` of a `docker compose up -d`-initialized database (`docs/schema.sql`
  path) — byte-identical modulo comment/ordering noise (`requirements.md` AC1).
  `migrate-down` then `migrate-up` again — confirm the resulting schema is identical to a
  single `migrate-up` (AC2/AC3).

## 8. Traceability

| Requirement | Design section |
| --- | --- |
| BR1 (sequential migration files) | §2 |
| BR2 (`docs/schema.sql` mirrored, duplication documented) | §3 |
| BR3 (migrations ≡ `docs/schema.sql` from empty) | §2, §7 |
| BR4 (reversible) | §2 |
| BR5 (tool tracks applied state) | §1 |
| BR6 (no application code change) | §1–§4 (tooling/docs only) |
| AC1–AC6 | §2, §3, §4, §6, §7 |
