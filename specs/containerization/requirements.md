# Requirements — Containerization Hardening

Status: **Approved for implementation** (2026-10-19 — revised to the FP-32 story, see §0)
Jira: **FP-32** — "[Fase 2] Ambiente Docker reproduzível e imagem da aplicação revisada"
(planning id F2-US06, manual requirement R08). Depends on **FP-33**
(`specs/database-migrations/`), which it builds on.
Touches: `Dockerfile`, a new `.dockerignore`, `docker-compose.yml`, `.env.example`, a new
`scripts/smoke.sh`, `Makefile`, and documentation (`README.md`, `CLAUDE.md`,
`specs/architecture.md`).

## 0. Revision note (2026-10-19)

The first version of this spec (PR #30) predates the FP-32 Jira story, FP-33's migration
flow and the team's Fase 2 analysis (Confluence "Tech Challenge Fase 2 — Oficina Mecânica |
Evolução, Infraestrutura e Backlog", §2.1). The story asks for runtime versions "chosen and
verified during implementation, without copying old pins without review", so this revision
changes the original in these places (each is called out where it applies):

1. **Versions reviewed, not copied.** Build stage Go `1.25` → `1.26` (the line `go.mod`, CI
   and `CLAUDE.md` §2 moved to on 2026-10-18); runtime `alpine:3.20` → `alpine:3.24` (latest
   stable, `3.24.2`, on 2026-10-19). Compose's helper images leave `:latest` (§6).
2. **Resend is an optional pair.** The original design made `RESEND_API_KEY` a required,
   fail-fast variable; that contradicts `specs/service-order-email-notification/` and
   `config.Load`, and the Confluence analysis (§2.1, "Resend obrigatório/opcional") settles it
   as optional. `EXTERNAL_STATUS_UPDATE_SECRET`, listed by the original, is not read by any
   implemented code, so it is not added (the original itself said "whichever … have landed").
3. **Integration with FP-33 (new).** The image keeps shipping `/bootstrap-admin`; the build
   context never includes `backups/` (FP-33's baseline dumps hold real data); Compose keeps a
   single schema-initialization path.
4. **Smoke test and restart (new).** A repeatable check of health, login and an
   authenticated service-order opening + query, before and after a restart.
5. **Health check without `curl`** and **an allowlist `.dockerignore`** (design changes,
   reasons in §6).

## 1. Context

Source: `SOAT - Fase 2 - Tech challenge.pdf` p.3, "Garantir a aplicação containerizada via
Docker, com: Dockerfile atualizado; docker-compose para desenvolvimento local."

State of the `Dockerfile` when this feature starts (after FP-33): it `COPY`s the whole
project before `go build` — so any edit invalidates the dependency-download layer, and
`.env`, `backups/` dumps and anything else in the directory are sent as build context —,
the final stage is an unpinned `FROM alpine`, runs as `root`, and has no `HEALTHCHECK`.

State of `docker-compose.yml`: FP-33 already made the `migrate` service the only path that
creates the schema, with the API waiting for it. The helper images (`adminer`,
`swagger-ui`) float on `:latest`, and the default host ports of Adminer and Swagger UI
(`8085`, `8083`) disagree with `.env.example`, `README.md` and `CLAUDE.md` (`8081`,
`8082`) — invisible when `.env` exists, wrong when it does not.

## 2. User story

> As a developer, I want to run the solution locally with Docker, so that I can reproduce and
> validate the environment without scattered manual configuration. *(FP-32)*

> As whoever builds and deploys the API image (a developer, or the CI/CD pipeline in
> `specs/ci-cd-pipeline/`), I want it to build quickly on repeated builds, run as a non-root
> user and report its own health, so that iteration is fast, the container follows baseline
> container-security practice, and Kubernetes (`specs/kubernetes-deployment/`) can rely on
> it.

## 3. Business rules

1. **Multi-stage.** The build stage uses `golang:1.26-alpine` — the same Go line as `go.mod`
   and CI (`CLAUDE.md` §2). The final image contains only the compiled `/api` and
   `/bootstrap-admin` (FP-33) binaries on a minimal base — no toolchain, no source tree.
2. **Dependency cache.** `go.mod`/`go.sum` are copied and `go mod download` runs before the
   source is copied, so dependencies are re-downloaded only when those two files change.
3. **Explicit, reviewed versions.** The final base image is pinned to a specific Alpine
   minor; Compose's helper images are pinned to explicit versions — no `:latest`. Raising a
   version is a deliberate, reviewed change.
4. **Non-root.** The final image runs as a non-root user identified by a **numeric** UID/GID,
   so a Kubernetes `runAsNonRoot` check can verify it without a name lookup. The binaries
   are not writable by that user.
5. **Health check.** The image declares a `HEALTHCHECK` calling `GET /health` on the port
   the API listens on (`PORT`, default `8080`); a non-2xx response, a refused connection or
   a timeout counts as a failure.
6. **Build context.** Only what the build needs is sent: `go.mod`, `go.sum`, `cmd/` and
   `internal/`. Never `.env`, `backups/`, Terraform state or variables, Kubernetes secrets,
   `.git`, or coverage/security outputs — including files added to the repository later.
7. **Compose.** From a fresh clone, `cp .env.example .env && docker compose up -d` starts
   `db` → `migrate` → `api`, plus `seed`, `adminer` and `swagger-ui`; the API starts only
   after migrations succeed (FP-33) and reaches `healthy`. Swagger UI serves
   `docs/openapi.yaml`; the API collections (`bruno/`, `docs/postman-collection.json`,
   `docs/insomnia-collection.json`) are documented. The startup order, what each service
   waits for, and every host port are documented.
8. **Secrets.** Required secrets (`JWT_SECRET`, `EXTERNAL_NOTIFICATION_SECRET`) make
   `docker compose` stop with a message naming the variable when absent, and the API keeps
   failing fast on its own. `RESEND_API_KEY`/`RESEND_FROM_EMAIL` stay an optional pair:
   neither set disables the e-mail, a key without a sender stops the API at startup.
   `ADMIN_*` are read only by `/bootstrap-admin`. The database credentials keep their
   development defaults (local only).
9. **One initialization path.** In Compose the schema is created only by the `migrate`
   service (FP-33); Postgres' `docker-entrypoint-initdb.d` creates nothing, and this feature
   must not bring that back. A pre-existing volume is adopted only through
   `make migrate-baseline` (FP-33), never by running the baseline over a schema created
   some other way.
10. **No application behavior change.** No Go code changes; environment variables, the
    listening port and routes stay as they are.
11. **Smoke test.** A repeatable check against a running stack: `GET /health` returns 200,
    login returns 200 with a token, an authenticated `POST /api/v1/service-orders` returns
    201 and `GET /api/v1/service-orders/{id}` returns 200. After a restart (`docker compose
    restart`, and `down` + `up` without `-v`), the service order created before the restart
    is still returned — data lives in the named volume.
12. **`.env.example`** lists every variable `docker-compose.yml` references, with no real
    secret (`CLAUDE.md` §13).

## 4. Scope

In scope: `Dockerfile`, a new `.dockerignore`, `docker-compose.yml`, `.env.example`, a new
`scripts/smoke.sh` and its `make smoke` target, and the documentation that describes them.

Out of scope: Kubernetes manifests (FP-34), building/pushing the image in CI (FP-36),
Terraform (FP-35), any Go code, the Postgres major version.

## 5. Acceptance criteria

1. A second `docker build` after editing only a source file reuses the cached
   `go mod download` layer.
2. The image's configured user is `10001:10001`, and a process started from it runs as
   `uid=10001`.
3. `docker inspect` shows a `Healthcheck` against `/health`; the `api` container goes from
   `starting` to `healthy`.
4. A file such as `.env` or `backups/*` present in the working directory is not part of the
   build context.
5. `docker compose up -d` on an empty volume: `migrate` and `seed` exit 0, `api` becomes
   `healthy`, Swagger UI serves the OpenAPI document, Adminer answers.
6. The smoke test passes on that fresh stack.
7. After `docker compose restart`, and again after `docker compose down` + `up -d` (no
   `-v`): `api` is `healthy`, the smoke test passes, and the service order created in AC6 is
   still returned.
8. Without `JWT_SECRET` (or `EXTERNAL_NOTIFICATION_SECRET`), `docker compose` refuses to
   start and names the variable; with `RESEND_API_KEY` but no `RESEND_FROM_EMAIL` the API
   exits with a clear message; with neither Resend variable the API starts.
9. `/bootstrap-admin` still runs from the image as the non-root user.
10. `go build ./...`, `go vet ./...` and `go test ./...` are unaffected.
11. Every variable referenced by `docker-compose.yml` appears in `.env.example`.

## 6. Decisions (2026-10-19)

1. **Runtime base: `alpine:3.24`**, not distroless/`scratch`. A shell-less image cannot run
   the `HEALTHCHECK` without a health sub-command in the Go binary — an application change
   (BR10) — and Alpine keeps a shell for `docker exec` debugging. It already ships the CA
   bundle the Resend HTTPS call needs.
2. **Health check with BusyBox `wget`**, already in Alpine, instead of installing `curl`
   (the original design): one package fewer in the image, same result — verified on
   `alpine:3.24` that `wget` exits non-zero on HTTP 500 and on a refused connection.
3. **UID/GID `10001`**: numeric (BR4) and outside the range of system users.
4. **Allowlist `.dockerignore`** (exclude everything, re-include the four paths the build
   needs) instead of the original denylist: new sensitive files (`backups/`, Terraform
   state, kubeconfigs) stay out without anyone remembering to list them.
5. **Helper image versions**: `adminer:6.1.1` and `swaggerapi/swagger-ui:v5.33.1` — the most
   recent releases at least two weeks old on 2026-10-19; `postgres:16-alpine` keeps the
   major pinned (the schema targets PostgreSQL 16) and takes patch updates;
   `migrate/migrate:v4.20.1` stays as FP-33 pinned it.
6. **Smoke test uses the seed's fixed IDs** (customer, vehicle, service) by default, so it
   targets the local stack; the IDs and credentials can be overridden by environment
   variables. The CD smoke test in the cluster, where the seed never runs (FP-33 BR11),
   belongs to FP-36.
