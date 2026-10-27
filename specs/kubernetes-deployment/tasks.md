# Tasks — Kubernetes Manifests (`/k8s`)

Jira **FP-34**. Revised 2026-10-25 with `requirements.md` §0.

## 1. Spec confirmation

- [x] Local-vs-cloud closed (local `kind`, `specs/infrastructure-provisioning/` §7); demo
      through `kubectl port-forward` on the `ClusterIP` `Service`.
- [x] FP-36 contract (`specs/ci-cd-pipeline/design.md` §9 items 6–13) and FP-33 contract
      (`specs/database-migrations/design.md` §9) reviewed; names follow them.

## 2. Manifests (`design.md` §1–§7)

- [x] `k8s/configmap.yaml` — `PORT`, `JWT_TTL`.
- [x] `k8s/deployment.yaml` — no `replicas`; image without tag; `envFrom`; resources;
      startup/readiness/liveness on `/health`; `preStop`; restricted security context.
- [x] `k8s/service.yaml` — `ClusterIP` 8080.
- [x] `k8s/hpa.yaml` — 2–8, CPU 70 %, memory 80 %, `behavior`.
- [x] `k8s/migration-job.yaml` — `migrate/migrate:v4.20.1`, `backoffLimit: 0`, migrations
      `ConfigMap` mounted, never committed.
- [x] `k8s/bootstrap-admin-job.yaml` — API image, `/bootstrap-admin`, admin `Secret`.
- [x] `k8s/templates/secrets.example.yaml` — `REPLACE_ME` only, never applied.
- [x] `k8s/load-test/load-generator-job.yaml` — load through the `Service`.
- [x] `scripts/k8s-evidence.sh` — `status`, `hpa-load`, `pod-recovery`, `db-persistence`.
- [x] `Makefile` `k8s-*` targets; `.gitignore` covers `evidence/` and `k8s/**/*.local.yaml`.

## 3. Static verification

- [x] FP-36's kustomize render (namespace, image tag + digest, `nameSuffix`) produces the
      expected names and image; `kubeconform -strict` passes.
- [x] `shellcheck scripts/k8s-evidence.sh`.
- [x] `.github/workflows/iac.yml` `manifests` job runs both on changes to `k8s/**`.
- [x] `scripts/deploy.sh check` from PR #42 passes against these files.

## 4. Operational verification (on FP-35's cluster)

### 4.1 Run of 2026-10-25 (environment that implemented the feature)

A real `kind` node could not boot in that environment (cgroup v1 + nested `runc`, see
`specs/infrastructure-provisioning/tasks.md` §4.1), so the checks ran on two partial
substitutes. What they cannot prove is listed in §4.2.

**Real API server** (etcd 3.6.5, `kube-apiserver` + `kube-controller-manager` v1.34.3 with
`PodSecurity` admission; no scheduler/kubelet, so pods are admitted but never run), with
FP-35's namespace and Postgres applied by Terraform and PR #42's `scripts/deploy.sh`
(commit `ad8a1ac`, `kubectl` v1.36.5):
- `deploy.sh check` → `contract ok` (names, image rendering, `Job` suffixes, admin `Secret`,
  migrations `ConfigMap`).
- `deploy.sh config` → both `Secret`s and the migrations `ConfigMap` server-side applied;
  `migrate` created `automotive-workshop-api-migrate-ci-1-1` and, with no `Complete`,
  stopped with "the rollout is blocked" (the AC7 gate, from the timeout path); `bootstrap`
  created its `Job` with the rendered API image; `rollout` applied
  `ConfigMap`/`Deployment`/`Service`/`HPA`, and the `Deployment` carries exactly
  `…:<sha>@sha256:<digest>` (AC2).
- Every pod (API, both `Job`s, load generator, Postgres) **admitted under `restricted`**,
  with no `FailedCreate`/`forbidden` event (AC9).
- The HPA controller raised the `Deployment` from 1 to `minReplicas` 2. After
  `kubectl scale --replicas=5`, a server-side re-apply (`deploy.sh rollout`) **and** a
  client-side `kubectl apply` both left `spec.replicas` at 5 (AC4).
- `kubectl apply -f k8s/ --dry-run=client` reads only the six top-level objects, never the
  `Secret` template or the load generator (AC8).
- The manual commands of `k8s/README.md` (Secrets, migrations `ConfigMap`, both `Job`s,
  rollout) ran as written; the API `Secret` has no `last-applied-configuration` annotation.
- `scripts/k8s-evidence.sh status` ran; `hpa-load` created its `Job` with the requested
  workers/duration and wrote the timeline (no metrics without a kubelet).

**Real containers with the manifests' runtime settings** (Docker, same users, read-only
root filesystem, all capabilities dropped, no new privileges):
- `migrate/migrate:v4.20.1` as UID 65534 with `/migrations` read-only → `0001_init`, `0002`
  applied.
- `/bootstrap-admin` as UID 10001 → "administrative user created"; second run → "already
  exists - left unchanged".
- The API as UID 10001 with a read-only root filesystem, 0.5 CPU / 256 MiB:
  `/health` 200, `/api/v1/auth/me` without token 401, login 200, `/api/v1/services` with
  the token 200; `/bin/sleep` (the `preStop` command) present in the image.
- Load sizing: the load generator's default (4 BusyBox `wget` workers) drove a single API
  container to ~370m CPU — 3.7× the 100m request — so the 70 % target is crossed and the
  HPA would settle around 6 replicas (370m / 70m), below the maximum of 8.

### 4.2 First run on a real cluster (2026-10-27)

Record the environment (host, node image, versions) and results here.

**Run of 2026-10-27 (UTC)** — Linux x86_64, cgroup v2, Docker 29.9.2, 8 CPUs / 16 GiB;
Terraform 1.16.6, kubectl v1.36.5, `kindest/node:v1.37.0` (pinned digest), image built from
commit `4c731a0` and loaded into the node (no GHCR image published yet). Driven by
`scripts/deploy.sh` (DEPLOY_ID `local-evidencia`) and `scripts/k8s-evidence.sh`; raw output in
the git-ignored `evidence/` and `deploy-evidence/` of that machine, summarized in the root
README ("Evidências da execução").

- [x] AC1 — documented order: migration `Job` `Complete` (2 migrations), bootstrap `Job`
      `Complete`, rollout complete, `/health` 200 through the `Service` plus the
      authenticated smoke (401 without token, login, `/auth/me`, `/services`).
- [x] AC2 — the `Deployment` runs exactly the rendered `name:<sha>@sha256:<digest>`.
- [x] AC3 — `kubectl get hpa`: `cpu: 1%/70%, memory: 1%/80%`, min 2, max 8.
- [x] AC4 — re-apply while scaled out keeps the replica count.
- [x] AC5 — `scripts/k8s-evidence.sh hpa-load`: 4 workers × 240 s → 2→3→2; 10 workers × 180 s
      → 2→4→5 (CPU 135 %) → 4→2 after the 120 s stabilization; `AbleToScale`/`ScalingActive`
      True; timelines recorded.
- [x] AC6 — `scripts/k8s-evidence.sh pod-recovery`: replacement ready in 6 s, fewest ready
      `Service` endpoints observed: 1.
- [x] AC7 — failing migration `Job` on a fresh ephemeral environment
      (`SIMULATE_MIGRATION_FAILURE=true`, DEPLOY_ID `local-falha-migracao`): `Job` `Failed`,
      `deploy.sh migrate` stopped with "the rollout is blocked", no `Deployment`/HPA/API
      `Service` in the namespace. The same check on GitHub Actions is FP-36's pending run.
- [x] AC8 — no secret committed; `kubectl apply -f k8s/` does not read the template.
- [x] AC9 — every pod admitted under `restricted` Pod Security.

## 5. Documentation

- [x] `k8s/README.md` (`design.md` §8): order, both paths, access, HPA/load, recovery,
      limits.
- [x] Root `README.md` "Deploy em Kubernetes" section; `CLAUDE.md`,
      `specs/architecture.md` updated.

## 6. Follow-ups recorded (not in this story)

- [ ] Graceful shutdown in `cmd/api` (`SIGTERM` → `server.Shutdown`), which would let the
      `preStop` delay be shortened (`design.md` §3).
