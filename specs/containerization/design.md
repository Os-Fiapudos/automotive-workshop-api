# Design — Containerization Hardening

Satisfies: `requirements.md` (all sections).

## 1. `Dockerfile`

```dockerfile
FROM golang:1.25-alpine AS build
WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -o /bin/api ./cmd/api

FROM alpine:3.20
RUN addgroup -S app && adduser -S app -G app \
    && apk add --no-cache ca-certificates curl
COPY --from=build /bin/api /api
USER app
EXPOSE 8080
HEALTHCHECK --interval=10s --timeout=3s --start-period=5s --retries=3 \
    CMD curl -f http://localhost:8080/health || exit 1
CMD ["/api"]
```

- `COPY go.mod go.sum ./` + `go mod download` before `COPY . .` is the one change that fixes
  `requirements.md` BR1 — every later layer is cached until `go.mod`/`go.sum` actually
  change.
- `CGO_ENABLED=0` keeps the binary statically linked, which is what lets it run on
  `alpine`'s musl libc without a C toolchain in the final stage — already implicitly true
  today (no `cgo` import exists in this codebase), made explicit so it stays true.
- `addgroup`/`adduser` + `USER app` satisfies BR2 — the final image's `ENTRYPOINT`/`CMD`
  never runs as `root`.
- `apk add curl` is the one new package in the final stage, needed only so the
  `HEALTHCHECK`'s `CMD` can make an HTTP request without the Go binary needing to expose a
  CLI health-check mode; this is a small, well-known, non-root-exploitable addition to an
  already-minimal `alpine` base.
- `HEALTHCHECK` satisfies BR3; `start-period=5s` gives the process time to connect to
  Postgres (`internal/shared/database.NewPool`) before the first check counts against it.

## 2. `.dockerignore` (new file)

```
.git
*.md
docs/
specs/
bruno/
.env
.env.example
```

Keeps the build context small and avoids ever sending `.env` (if present locally) into the
build context at all, even though the `Dockerfile` never `COPY`s it — defense in depth, not
a functional requirement.

## 3. `docker-compose.yml`

The `api` service's `environment`/`env_file` block gains whichever of these are already
implemented at the time this feature lands (fail-fast via `${VAR:?...}`, the existing
`JWT_SECRET` convention):

```yaml
environment:
  DATABASE_URL: ${DATABASE_URL:?DATABASE_URL is required}
  JWT_SECRET: ${JWT_SECRET:?JWT_SECRET is required}
  EXTERNAL_NOTIFICATION_SECRET: ${EXTERNAL_NOTIFICATION_SECRET:?EXTERNAL_NOTIFICATION_SECRET is required}
  EXTERNAL_STATUS_UPDATE_SECRET: ${EXTERNAL_STATUS_UPDATE_SECRET:?EXTERNAL_STATUS_UPDATE_SECRET is required}
  RESEND_API_KEY: ${RESEND_API_KEY:?RESEND_API_KEY is required}
```

(`RESEND_API_KEY` per `specs/service-order-status-update/` — whichever mechanism is finally
used to send the quote notification e-mail; name confirmed against that feature's
implementation when this task is done, not assumed here.)

`.env.example` gains a matching empty entry for each, grouped under a `# Fase 2 — external
integrations` comment, next to the existing variables.

## 4. Traceability

| Requirement | Design section |
| --- | --- |
| BR1 (layer caching) | §1 |
| BR2 (non-root) | §1 |
| BR3 (HEALTHCHECK) | §1 |
| BR4 (multi-stage, minimal final image) | §1 |
| BR5 (no behavior change) | §1 (binary/flags unchanged) |
| BR6 (compose stays working, new vars documented) | §3 |
