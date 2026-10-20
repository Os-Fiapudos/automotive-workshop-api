# Tasks — Database Migration Tooling

Jira FP-33. Each task names the `design.md` section it implements.

## 1. Spec confirmation

- [x] Tool confirmed — `golang-migrate` `v4.20.1` (`requirements.md` §7.1, 2026-10-19).
- [x] Bootstrap approach and local seed flow decided (`requirements.md` §7.2, §7.3).
- [x] Spec revised to cover the FP-33 acceptance checklist (`requirements.md` §0).

## 2. Migrations (`design.md` §2)

- [x] `migrations/0001_init.up.sql` — guard + `docs/schema.sql` at `0e6f2ab`.
- [x] `migrations/0001_init.down.sql` — reverse-order drops, no `CASCADE`, keep `pgcrypto`.
- [x] `docs/schema.sql` header/enum comments — reference only, applied by migrations
      (§3).

## 3. Tooling (`design.md` §4)

- [x] `scripts/migrate.sh` — `up`, `version`, `create`, `down` (confirmation), `baseline`,
      `verify`.
- [x] `docker-compose.yml` — drop the init mounts; `migrate` and `seed` services; `api`
      waits for `migrate`.
- [x] `Makefile` — `migrate-*` and `bootstrap-admin` targets; `schema`/`reset` adjusted.
- [x] `.gitignore` — `backups/`.

## 4. Bootstrap (`design.md` §7)

- [x] `auth.Repository.CreateIfAbsent`.
- [x] `auth.BootstrapAdmin` + unit tests.
- [x] `config.LoadAdminBootstrap` + unit tests.
- [x] `cmd/bootstrap-admin` (with `doc.go`).
- [x] `Dockerfile` — build and copy `/bootstrap-admin`.
- [x] Integration test `internal/handlers_test/auth_bootstrap_test.go`.

## 5. CI (`design.md` §8)

- [x] `coverage` job applies migrations instead of `docs/schema.sql`.
- [x] New `migrations` job running `scripts/migrate.sh verify`.

## 6. Documentation (`design.md` §10)

- [x] `README.md` — "Migrações do banco de dados" and the updated Compose/Makefile text.
- [x] `CLAUDE.md` §3, §5, §6, §7, §13, §14.
- [x] `specs/architecture.md`.
- [x] `.env.example`, `internal/handlers_test` package comment, `scripts/coverage.sh` help.

## 7. Verification (`design.md` §11)

- [x] `scripts/migrate.sh verify` passes locally (AC1–AC6).
- [x] Compose on an empty volume: migrate → seed → API healthy → seeded login (AC7).
- [x] Compose over a Fase 1 volume built from `0e6f2ab` with extra data: guard → `baseline`
      → `up`, data preserved (AC4).
- [x] Bootstrap against a migrated database with no users (AC8).
- [x] `go build ./...`, `go vet ./...`, `go test ./...` — also with `DATABASE_URL` set
      (AC11).
- [x] Re-check `requirements.md` §5 AC1–AC11.
