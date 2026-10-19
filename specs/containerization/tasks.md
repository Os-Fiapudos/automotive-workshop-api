# Tasks — Containerization Hardening

## 1. Dockerfile (`design.md` §1)

- [ ] Reorder `COPY`s so `go.mod`/`go.sum` + `go mod download` happen before `COPY . .`.
- [ ] Add `CGO_ENABLED=0` to the `go build` step.
- [ ] Add a non-root user/group in the final stage; `USER app`.
- [ ] Add `apk add --no-cache ca-certificates curl` to the final stage.
- [ ] Add `HEALTHCHECK` against `GET /health`.

## 2. `.dockerignore` (`design.md` §2)

- [ ] Create `.dockerignore` excluding `.git`, `*.md`, `docs/`, `specs/`, `bruno/`, `.env`,
      `.env.example`.

## 3. Compose and env (`design.md` §3)

- [ ] `docker-compose.yml`: add fail-fast `${VAR:?...}` entries for every new secret
      introduced by Fase 2 features already implemented at this point.
- [ ] `.env.example`: add matching empty entries, grouped under a comment.

## 4. Verification

- [ ] `docker build .` twice (second run with only a source-file touch) — confirm the
      dependency-download layer is cached on the second build.
- [ ] `docker run --rm <image> id` (or equivalent against the running container) — confirm
      non-root uid.
- [ ] `docker compose up -d` — confirm `docker inspect` shows the healthcheck transitioning
      to `healthy`.
- [ ] `docker compose up -d` end-to-end smoke test: `curl localhost:8080/health` and one
      authenticated request (e.g. `POST /api/v1/auth/login`) succeed exactly as before this
      change.
- [ ] `go build ./...`, `go vet ./...`, `go test ./...` unaffected (no Go source changed).
