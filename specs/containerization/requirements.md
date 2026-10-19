# Requirements — Containerization Hardening

Status: **Approved for implementation**
Touches: `Dockerfile`, `docker-compose.yml`, new `.dockerignore`.

## 1. Context

Source: `SOAT - Fase 2 - Tech challenge.pdf` p.3, "Garantir a aplicação containerizada via
Docker, com: Dockerfile atualizado; docker-compose para desenvolvimento local." The current
`Dockerfile` (`GAPS-FASE-2.md` §3.2) is 8 lines: it `COPY`s the entire project before
running `go build` (so any source change invalidates Docker's layer cache all the way back
to dependency download), runs as the image's default root user, and has no `HEALTHCHECK`.
This feature hardens it without changing the application's runtime behavior.

## 2. User story

> As whoever builds and deploys this API's container image (a developer locally, or the
> CI/CD pipeline in `specs/ci-cd-pipeline/`),
> I want the image to build quickly on repeated builds, run as a non-root user, and report
> its own health,
> so that iteration is fast, the container follows baseline container-security practice, and
> Kubernetes (`specs/kubernetes-deployment/`) can use a real liveness/readiness probe
> instead of just "the process is still running."

## 3. Business rules

1. `go.mod`/`go.sum` are copied and `go mod download` runs **before** the rest of the source
   is copied, so Docker only re-downloads dependencies when `go.mod`/`go.sum` change, not on
   every source edit.
2. The final image runs as a non-root, unprivileged user — never the image's default root.
3. The final image declares a `HEALTHCHECK` that calls `GET /health` (the endpoint already
   implemented, `CLAUDE.md` §1) and reports unhealthy on a non-2xx response or a timeout.
4. The build stays multi-stage (already true today) — the final image contains only the
   compiled binary and whatever a minimal base image needs to run it, not the Go toolchain
   or source tree.
5. No application behavior, environment variable, or exposed port changes — this is a build-
   and-runtime-hygiene feature, not a functional one.
6. `docker-compose.yml` keeps working for local development exactly as today (`db`,
   `adminer`, `api`, `swagger-ui`), extended only with whatever new environment variables
   other Fase 2 features require (`EXTERNAL_NOTIFICATION_SECRET`,
   `EXTERNAL_STATUS_UPDATE_SECRET`, `RESEND_API_KEY` — see
   `specs/service-order-quote-decision/`, `specs/service-order-status-update/`) and,
   correspondingly, `.env.example`.

## 4. Scope

In scope: `Dockerfile`, a new `.dockerignore`, `docker-compose.yml`'s `api` service
environment list, `.env.example`.

Out of scope: any Kubernetes manifest (`specs/kubernetes-deployment/`), any CI pipeline step
that builds/pushes this image (`specs/ci-cd-pipeline/`), any change to `cmd/api/main.go` or
any feature package.

## 5. Acceptance criteria

1. `docker build .` with only a source-file change (no `go.mod`/`go.sum` change) reuses the
   cached dependency-download layer.
2. `docker run <image> whoami` (or equivalent) reports a non-root user.
3. `docker inspect <image>` shows a `Healthcheck` configured against `/health`; a container
   with the API not yet listening reports `starting`/`unhealthy` as appropriate, and
   `healthy` once `/health` responds 200.
4. The image still runs and serves `/health`/the full API exactly as before this change,
   verified against `docker compose up` locally.
5. `docker-compose.yml`'s `api` service documents (via `${VAR:?...}` fail-fast, the existing
   pattern for `JWT_SECRET`) every new required secret introduced by other Fase 2 features
   that have landed by the time this feature is implemented.
6. `.env.example` lists every variable `docker-compose.yml` references, with no real secret
   value (existing convention, `CLAUDE.md` §13).

## 6. Out of scope

- Multi-architecture builds (`linux/amd64`+`linux/arm64`) — not requested by the PDF; add
  only if the chosen Kubernetes target (`specs/kubernetes-deployment/`,
  `specs/infrastructure-provisioning/`) turns out to need it.
- Image vulnerability scanning as part of the build — already covered by `gosec`/
  `govulncheck` in CI (`CLAUDE.md` §7); a container-image scanner (e.g. Trivy) is a
  possible future addition, not required by the PDF's explicit Docker requirements.
