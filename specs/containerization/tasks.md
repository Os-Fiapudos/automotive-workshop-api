# Tasks — Containerization Hardening

Jira FP-32. Each task names the `design.md` section it implements.

## 1. Spec

- [x] Spec revised to the FP-32 story and FP-33's flow (`requirements.md` §0, §6).

## 2. Image (`design.md` §1, §2)

- [x] `Dockerfile` — `go mod download` layer, explicit source copy, `CGO_ENABLED=0`, both
      binaries, `alpine:3.24`, UID/GID `10001`, `HEALTHCHECK` with `wget`.
- [x] `.dockerignore` — allowlist (`go.mod`, `go.sum`, `cmd`, `internal`), tests excluded.

## 3. Compose and env (`design.md` §3, §4)

- [x] `docker-compose.yml` — header (order, ports), pinned `adminer`/`swagger-ui`, default
      ports `8081`/`8082`.
- [x] `.env.example` — header (required vs optional), host-port label.

## 4. Smoke test (`design.md` §5)

- [x] `scripts/smoke.sh`.
- [x] `Makefile` — `smoke` target.

## 5. Documentation (`design.md` §6)

- [x] `README.md`, `CLAUDE.md`, `specs/architecture.md`.

## 6. Verification (`design.md` §7)

- [x] AC1 — dependency layer cached on a source-only rebuild.
- [x] AC2, AC9 — user `10001:10001`; `/bootstrap-admin` runs as it.
- [x] AC4 — `.env`/`backups/` not in the build context.
- [x] AC3, AC5, AC6 — fresh `up`: `migrate`/`seed` exit 0, `api` healthy, Swagger/Adminer up,
      smoke passes.
- [x] AC7 — after `restart` and after `down` + `up`: healthy, smoke passes, earlier order
      still there.
- [x] AC8 — missing required secret, Resend key without sender, neither Resend variable.
- [x] AC10 — `go build`/`vet`/`test`.
- [x] AC11 — every Compose variable in `.env.example`.
