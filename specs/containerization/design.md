# Design — Containerization Hardening

Satisfies: `requirements.md` (all sections). Revised 2026-10-19 for FP-32 — see
`requirements.md` §0 for what changed from the first version.

## 1. `Dockerfile` (BR1–BR5, BR10)

```dockerfile
# Build stage: same Go line as go.mod and CI (CLAUDE.md §2).
FROM golang:1.26-alpine AS build
WORKDIR /src

# Dependencies first: this layer is reused until go.mod/go.sum change.
COPY go.mod go.sum ./
RUN go mod download

COPY cmd ./cmd
COPY internal ./internal
ENV CGO_ENABLED=0
RUN go build -o /out/api ./cmd/api \
    && go build -o /out/bootstrap-admin ./cmd/bootstrap-admin

# Runtime stage: pinned Alpine minor, reviewed in specs/containerization/.
FROM alpine:3.24
RUN addgroup -S -g 10001 app \
    && adduser -S -D -H -u 10001 -G app -s /sbin/nologin app
COPY --from=build /out/api /api
COPY --from=build /out/bootstrap-admin /bootstrap-admin
USER 10001:10001
EXPOSE 8080
HEALTHCHECK --interval=10s --timeout=3s --start-period=10s --retries=3 \
    CMD wget -q -T 2 -O /dev/null "http://127.0.0.1:${PORT:-8080}/health" || exit 1
CMD ["/api"]
```

- **Cache (BR2).** `go mod download` sits in its own layer after `COPY go.mod go.sum`; a
  source edit only re-runs the two `COPY` source layers and the build. No BuildKit cache
  mounts: they do not survive across CI runners, while plain layers can be cached by the
  FP-36 pipeline.
- **Source copy.** `COPY cmd` + `COPY internal` instead of `COPY . .` — the same set the
  `.dockerignore` allowlist re-includes (§2), so what the build sees is explicit in both
  places. Nothing outside these paths is needed: there is no `go:embed` and the binaries read
  no file at runtime.
- **`CGO_ENABLED=0`** keeps both binaries statically linked, which is what lets them run on
  musl without a C toolchain — already implicitly true (no `cgo` import), made explicit.
- **`/bootstrap-admin`** is kept exactly as FP-33 introduced it
  (`specs/database-migrations/design.md` §7.2): the Kubernetes bootstrap `Job` and
  `make bootstrap-admin` run it from this image. `CMD` (not `ENTRYPOINT`) stays, so
  `docker compose run api /bootstrap-admin` keeps working.
- **Non-root (BR4).** System user/group `app` with UID/GID `10001`; `USER 10001:10001` is
  numeric so `runAsNonRoot` can verify it. The copied binaries keep owner `root` and mode
  `0755` — executable, not writable, by `app`. The API listens on `8080` (> 1024), so no
  capability is needed.
- **Health check (BR5).** BusyBox `wget` (in Alpine's base) exits non-zero on a non-2xx
  response, a refused connection or after `-T 2` seconds; `HEALTHCHECK --timeout=3s` bounds
  it as well. The shell form expands `PORT` at check time, so a container started with
  another `PORT` is still probed correctly. `start-period=10s` covers the database
  connection at startup (`database.NewPool`). Kubernetes ignores `HEALTHCHECK` and uses its
  own probes against the same `/health` (`specs/kubernetes-deployment/`).
- **Base (BR3).** `alpine:3.24` (`3.24.2` on 2026-10-19) includes `ca-certificates-bundle`,
  needed by the Resend HTTPS call; no `apk add` at all.

## 2. `.dockerignore` (new file, BR6)

```
# Allowlist: the image is built only from the module files and the Go source
# (Dockerfile). Everything else - .env, backups/, Terraform state, kubeconfigs,
# .git, coverage/security outputs - never enters the build context, including
# files added to the repository later.
*
!go.mod
!go.sum
!cmd
!internal
# Tests are not compiled into the image; leaving them out also keeps a test-only
# change from invalidating the build layer.
**/*_test.go
**/testdata
```

A path added later that the build really needs must be re-included here **and** copied in
the `Dockerfile` — both on purpose.

## 3. `docker-compose.yml` (BR3, BR7–BR9)

Changes on top of FP-33's file:

- A header comment documenting the startup order and the host ports (BR7).
- `adminer` → `adminer:6.1.1`, default host port `8081`; `swagger-ui` →
  `swaggerapi/swagger-ui:v5.33.1`, default host port `8082` — matching `.env.example`,
  `README.md` and `CLAUDE.md`.
- `api`: unchanged environment and dependencies. It inherits the image's `HEALTHCHECK`, so
  `docker compose ps` reports `healthy`. No `PORT` is set: inside the container the API
  stays on `8080`, and `API_PORT` only changes the host side of the mapping.
- `db`, `migrate`, `seed`: unchanged. No `docker-entrypoint-initdb.d` mount comes back
  (BR9).

Startup order (what waits for what):

| Service | Waits for | Ends as |
| --- | --- | --- |
| `db` | — | running, `healthy` (`pg_isready`) |
| `migrate` | `db` healthy | exits 0 (applies `migrations/`); a failure keeps `api` down |
| `seed` | `migrate` exit 0 | exits 0 (`docs/seed.sql`, idempotent); nothing waits on it |
| `api` | `db` healthy + `migrate` exit 0 | running, `healthy` (`/health`) |
| `adminer` | `db` healthy | running |
| `swagger-ui` | — | running |

Secrets (BR8) keep the existing pattern: `${JWT_SECRET:?…}` and
`${EXTERNAL_NOTIFICATION_SECRET:?…}` stop `docker compose` with the message naming the
variable; `RESEND_API_KEY`/`RESEND_FROM_EMAIL` default to empty (`${VAR:-}`), and
`config.Load` enforces the pair.

## 4. `.env.example` (BR12)

Already lists every variable Compose references (`POSTGRES_*`, `*_PORT`, `JWT_*`,
`EXTERNAL_NOTIFICATION_SECRET`, `RESEND_*`, plus the commented `ADMIN_*` for
`make bootstrap-admin`). This feature adds a short header saying which ones are required and
labels the ports as host ports. `PORT` is deliberately absent (see §3).

## 5. Smoke test — `scripts/smoke.sh` and `make smoke` (BR11)

POSIX `sh` + `curl`, no `jq` (the response fields it needs are extracted with `sed`).

| Step | Request | Expects |
| --- | --- | --- |
| 1 | `GET /health`, retried up to `SMOKE_WAIT_SECONDS` (default 60) | 200 |
| 2 | `POST /api/v1/auth/login` | 200 and a `token` |
| 3 | only if `SMOKE_EXPECT_ORDER_ID` is set: `GET /api/v1/service-orders/{that id}` | 200 — data survived the restart |
| 4 | `POST /api/v1/service-orders` (customer, vehicle, one requested service) | 201 and an `id` |
| 5 | `GET /api/v1/service-orders/{id}` | 200 |

- Defaults target the local stack: `SMOKE_BASE_URL=http://localhost:${API_PORT:-8080}`,
  the seed's admin (`admin@workshop.local` / `admin123`, already public in `README.md`), and
  the seed's customer/vehicle/service `a0…01`/`b0…01`/`d0…01`. Each can be overridden
  (`SMOKE_EMAIL`, `SMOKE_PASSWORD`, `SMOKE_CUSTOMER_ID`, `SMOKE_VEHICLE_ID`,
  `SMOKE_SERVICE_ID`).
- Output: one line per step; the last line prints the created order's id, to pass as
  `SMOKE_EXPECT_ORDER_ID` after a restart. The token and the password are never printed
  (`specs/auth/requirements.md` BR5); on failure the step, the HTTP status and the response
  body are printed and the script exits 1.
- Each run creates one `RECEIVED` service order — fine on the local database; there is no
  rule against several orders for one vehicle.
- `make smoke` runs it with the `API_PORT` from `.env`; `SMOKE_*` variables pass through.

## 6. Documentation

- `README.md`: Docker section — startup order and readiness, image notes (non-root, health
  check, pinned versions), the smoke test and the restart check, Swagger/collections.
- `CLAUDE.md`: §2 local infra (image hardening, pinned helper versions to keep reviewed),
  §3 (`scripts/smoke.sh`, `.dockerignore`), §5 (`make smoke`).
- `specs/architecture.md`: the local-environment row (services, non-root image, health
  check).

## 7. Verification (evidence for the PR)

Run in an isolated Compose project with its own ports and volume
(`COMPOSE_PROJECT_NAME=fp32-verify`), removed at the end, so no existing local stack is
touched:

1. AC1 — `docker build` twice, the second after touching a `.go` file: the
   `go mod download` step reports `CACHED`.
2. AC2/AC9 — `docker inspect -f '{{.Config.User}}'`; `docker run --rm --entrypoint id`;
   `docker compose run --rm api /bootstrap-admin` with `ADMIN_*` set.
3. AC4 — a throwaway build that copies the whole context and lists it, with a dummy `.env`
   and `backups/x.dump` present: neither appears.
4. AC5/AC3/AC6 — `up -d` on an empty volume, `ps` (exit codes, `healthy`), Swagger UI and
   Adminer return 200, smoke test.
5. AC7 — `restart`, smoke with `SMOKE_EXPECT_ORDER_ID`; `down` + `up -d`, again.
6. AC8 — `docker compose config` without `JWT_SECRET`/`EXTERNAL_NOTIFICATION_SECRET`; `api`
   with `RESEND_API_KEY` and no sender; `api` with neither.
7. AC10 — `go build`/`vet`/`test` in `golang:1.26-alpine`.
8. AC11 — every `${VAR}` in `docker-compose.yml` is found in `.env.example`.

## 8. Traceability

| Requirement | Design section |
| --- | --- |
| BR1 (multi-stage, Go line) | §1 |
| BR2 (dependency cache) | §1 |
| BR3 (reviewed versions) | §1, §3 |
| BR4 (numeric non-root) | §1 |
| BR5 (HEALTHCHECK) | §1 |
| BR6 (build context) | §2 |
| BR7 (Compose, order, ports, Swagger/collections) | §3, §6 |
| BR8 (secrets) | §3, §4 |
| BR9 (one initialization path) | §3 |
| BR10 (no behavior change) | §1 |
| BR11 (smoke + restart) | §5, §7 |
| BR12 (`.env.example`) | §4 |
