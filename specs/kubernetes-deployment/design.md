# Design — Kubernetes Manifests (`/k8s`)

Satisfies: `requirements.md` (all sections). Revised 2026-10-25 with it (`requirements.md`
§0). The manifests themselves are the source of the exact YAML; this document records what
each one does and why.

## 1. Layout

```
k8s/
  configmap.yaml              ConfigMap automotive-workshop-api-config      (rollout set)
  deployment.yaml             Deployment automotive-workshop-api            (rollout set)
  service.yaml                Service automotive-workshop-api               (rollout set)
  hpa.yaml                    HorizontalPodAutoscaler automotive-workshop-api (rollout set)
  migration-job.yaml          Job automotive-workshop-api-migrate          (+ -<deploy id>)
  bootstrap-admin-job.yaml    Job automotive-workshop-api-bootstrap-admin  (+ -<deploy id>)
  templates/secrets.example.yaml   key reference for the two Secrets — never applied
  load-test/load-generator-job.yaml  in-cluster load against the Service — evidence only
  README.md                   runbook
```

The top level holds exactly the files FP-36's contract names (`specs/ci-cd-pipeline/
design.md` §9 item 6). `kubectl apply -f k8s/` is not recursive, so the `Secret` template
and the load generator are never picked up by an accidental wholesale apply
(`requirements.md` AC8). Every object carries `namespace: automotive-workshop` and the
labels `app.kubernetes.io/name`, `app.kubernetes.io/component` and
`app.kubernetes.io/part-of: automotive-workshop`. Annotations, not labels, carry deploy
metadata (FP-36 §5: kustomize `commonLabels` would rewrite the immutable selector).

## 2. Configuration and secrets

- `configmap.yaml` — `PORT: "8080"`, `JWT_TTL: "1h"`: the only non-sensitive variables
  `internal/shared/config` reads. Applied with the rollout set (FP-36 includes
  `configmap.yaml` when present).
- `automotive-workshop-api-secrets` — `DATABASE_URL`, `JWT_SECRET`,
  `EXTERNAL_NOTIFICATION_SECRET`, `EXTERNAL_STATUS_UPDATE_SECRET`, optional
  `RESEND_API_KEY`/`RESEND_FROM_EMAIL`. `DATABASE_URL` is FP-35's
  `postgres_database_url_template` with `<password>` replaced by the percent-encoded
  password.
- `automotive-workshop-admin-bootstrap` — `ADMIN_EMAIL`, `ADMIN_PASSWORD`, optional
  `ADMIN_NAME`. Separate from the API `Secret` so API pods never receive the
  administrator's password.
- Both `Secret`s are created by the deploy (FP-36 `config` stage, or the manual commands of
  the README) with `kubectl create secret … --dry-run=client -o yaml | kubectl apply
  --server-side -f -` — server-side, so the values do not end up in a
  `last-applied-configuration` annotation. `templates/secrets.example.yaml` documents the
  keys with `REPLACE_ME`; its header says it is never applied.
- The migrations `ConfigMap` `automotive-workshop-api-migrations` is **generated at deploy
  time** from `migrations/*.sql` (`kubectl create configmap --from-file=migrations/`), never
  committed: the files in the commit being deployed are the single source, so no second copy
  can drift (`requirements.md` §0 item 5).

## 3. `deployment.yaml`

- **No `spec.replicas`.** The HPA (§5) owns the count. On the first apply the `Deployment`
  starts with the API default of one pod and the HPA raises it to `minReplicas` (2) within a
  sync period; every later apply (client- or server-side) leaves the HPA's value alone
  (`requirements.md` AC4).
- `strategy: RollingUpdate`, `maxSurge: 1`, `maxUnavailable: 0`: a new version adds a pod
  before removing one, so capacity never drops during a rollout. `revisionHistoryLimit: 5`.
- Image `ghcr.io/os-fiapudos/automotive-workshop-api` (no tag), `imagePullPolicy:
  IfNotPresent`. The deploy renders `:<sha>@sha256:<digest>` through kustomize `images:`;
  a raw apply would ask for `:latest`, which the pipeline never publishes, and fail to pull
  instead of running an unknown image.
- `envFrom`: the `ConfigMap` then the API `Secret`.
- Resources: requests `100m`/`128Mi`, limits `500m`/`256Mi` (the HPA's baseline; Go ≥ 1.25
  sizes `GOMAXPROCS` from the CPU limit). Eight pods request 0.8 CPU / 1 GiB — within a
  single `kind` node on a developer machine, with Postgres (0.25 CPU) and the system pods.
- Probes on `/health` port `http` (8080): `startupProbe` (2 s × 15), `readinessProbe` (5 s,
  timeout 2 s, 3 failures), `livenessProbe` (10 s, timeout 2 s, 3 failures). `/health` is
  static (it does not query the database), so a database outage does not restart API pods;
  database reachability is proven at startup (`database.NewPool` pings) and by the smoke
  test.
- Termination: `lifecycle.preStop` runs `/bin/sleep 5` (BusyBox, present in the Alpine
  runtime image) and `terminationGracePeriodSeconds: 30`. The API process does not handle
  `SIGTERM` (`cmd/api/main.go` calls `ListenAndServe` directly), so without the delay a pod
  removed by a rollout or a scale-down could be stopped while the `Service` still routes to
  it. The delay lets the endpoint removal propagate first. Graceful shutdown in the process
  itself is recorded as a follow-up, not done here (`requirements.md` §6).
- Security (`requirements.md` BR6): pod `runAsNonRoot`, `runAsUser`/`runAsGroup` 10001 (the
  image's numeric user), `seccompProfile: RuntimeDefault`; container
  `allowPrivilegeEscalation: false`, `readOnlyRootFilesystem: true`, `capabilities.drop:
  [ALL]`; `automountServiceAccountToken: false` (the API never calls the Kubernetes API).
  The pod uses the namespace's `default` `ServiceAccount`, which FP-36 gives the
  `ghcr-pull` image pull secret when the package is private.

## 4. `service.yaml`

`ClusterIP`, port 8080 → target port `http`, selector `app.kubernetes.io/name:
automotive-workshop-api` + `app.kubernetes.io/component: api`. kube-proxy spreads
connections across ready pods; the load generator (§7) targets this name, so the load is
distributed the same way real in-cluster traffic would be.

## 5. `hpa.yaml`

`autoscaling/v2`, target the `Deployment`, `minReplicas: 2`, `maxReplicas: 8`, metrics CPU
`averageUtilization: 70` and memory `averageUtilization: 80` (of the requests). With two
metrics the HPA scales up when either is above target and down only when both allow it.

`behavior`:
- `scaleUp`: no stabilization window; up to +100 % or +4 pods per 15 s (whichever is
  larger) — reacts within one or two metric resolutions (`metrics-server` scrapes every 15 s).
- `scaleDown`: 120 s stabilization window (the default is 300 s; 120 s keeps the video
  short while still ignoring brief dips), at most −50 % per 60 s.

Memory rarely drives scaling for this API (an idle pod uses a fraction of its 128 Mi
request), and the Go runtime returns memory to the OS slowly, so a memory-driven scale-up
can keep replicas higher for longer than CPU alone would; both are kept because the PDF names
both. The HPA changes pod count only: it neither adds nodes nor makes Postgres highly
available (FP-35 limits).

## 6. Deploy-time `Job`s

| | `migration-job.yaml` | `bootstrap-admin-job.yaml` |
| --- | --- | --- |
| Base name | `automotive-workshop-api-migrate` | `automotive-workshop-api-bootstrap-admin` |
| Image | `migrate/migrate:v4.20.1` (FP-33 pin) | the API image (same reference as the `Deployment`) |
| Command | args `["-path=/migrations", "-database", "$(DATABASE_URL)", "up"]` | `["/bootstrap-admin"]` |
| Environment | `DATABASE_URL` from `automotive-workshop-api-secrets` | `DATABASE_URL` from the API `Secret`; `envFrom` `automotive-workshop-admin-bootstrap` |
| Volumes | `ConfigMap` `automotive-workshop-api-migrations` at `/migrations` (read-only) | — |
| `backoffLimit` | `0` — a failed migration leaves the database dirty; a retry cannot succeed (FP-33 §6) | `1` — idempotent, a transient connection error may be retried once |
| Limits | `activeDeadlineSeconds: 600`, `ttlSecondsAfterFinished: 86400` | `activeDeadlineSeconds: 300`, `ttlSecondsAfterFinished: 86400` |
| User | 65534 (`nobody`) — the tool only reads files and talks to Postgres | 10001 |

Both: `restartPolicy: Never`, restricted security context, small requests/limits,
`automountServiceAccountToken: false`. A `Job`'s pod template is immutable, so each run
creates a new object: FP-36 appends `-<DEPLOY_ID>` (kustomize `nameSuffix`); the manual
runbook appends a timestamp. `ttlSecondsAfterFinished` keeps finished `Job`s for a day (the
evidence stage reads their logs minutes later) and then garbage-collects them so runs do not
accumulate. The bootstrap `Job` runs only after the migration `Job` is `Complete`; the
rollout set is applied only after both (`requirements.md` BR8).

## 7. Load generator and evidence

`load-test/load-generator-job.yaml`: `busybox:1.37.0@sha256:…`, `parallelism` =
`completions` = 4 workers, each running `wget` in a loop against
`http://automotive-workshop-api.automotive-workshop.svc.cluster.local:8080/health` for
`DURATION_SECONDS` (default 240). Requests go through the `Service`, so kube-proxy spreads
them across every ready pod — a port-forward pins one pod and proves nothing about
distribution (`requirements.md` AC5). Restricted security context, user 65534.

`scripts/k8s-evidence.sh` (Bash, `set -euo pipefail`; `KUBECONFIG` from `terraform -chdir=
infra output -raw kubeconfig_path` unless already set; writes to `evidence/<check>-<UTC
timestamp>/`, git-ignored):

| Command | Does | Fails when |
| --- | --- | --- |
| `status` | nodes, `kubectl top`, objects in the namespace, HPA description | the namespace is missing |
| `hpa-load` | snapshot → creates the load `Job` (unique name; workers/duration from `LOAD_WORKERS`/`LOAD_DURATION`) → samples HPA/replicas/`top` every 15 s during the load and a cool-down (`COOLDOWN_SECONDS`, 300) → `summary.md` (min/max/final replicas) | replicas never rose above the starting count |
| `pod-recovery` | deletes one ready API pod, polls ready endpoints of the `Service` until the `Deployment` is back to its desired ready count | no replacement within `RECOVERY_TIMEOUT`, or the `Service` had zero ready endpoints at any sample |
| `db-persistence` | (FP-35 AC7) creates a marker in a **separate** database `fp35_persistence_check` (never the application database: `0001_init` refuses a non-empty schema), deletes `postgres-0`, waits for it, reads the marker back, drops the check database | the marker is missing after the restart |

It reads no `Secret`; `psql` inside `postgres-0` uses the pod's local socket. Output files
contain object names, counts and metrics only.

## 8. Manual walkthrough (no pipeline)

`k8s/README.md` documents two paths with the same order (BR8): FP-36's `scripts/deploy.sh`
stages (`make deploy`) when available, and plain commands — create both `Secret`s and the
migrations `ConfigMap`, create the migration `Job` with a timestamp suffix and `kubectl wait
--for=condition=complete`, the same for the bootstrap `Job`, then apply `configmap.yaml`,
`deployment.yaml` (image substituted with `sed` on the stream, never in the file),
`service.yaml`, `hpa.yaml` server-side and wait for `rollout status`.

## 9. Validation

`.github/workflows/iac.yml` (`manifests` job, on changes to `k8s/**`): renders the rollout
set and each `Job` exactly as FP-36 does (kustomize with namespace, a sample
`:<sha>@sha256:<digest>` image and a `nameSuffix` for the `Job`s), validates every rendered
object and the two non-contract files with `kubeconform -strict` against the cluster's
Kubernetes version, and checks the repository rules: no `replicas` in `deployment.yaml`, no
`Namespace` or populated `Secret` in `k8s/`, the API image name matching the contract.
Operationally (`tasks.md` §4): server-side dry-run and a real deploy on FP-35's cluster
(which also enforces `restricted`), then the evidence commands of §7.

## 10. Traceability

| Requirement | Design section |
| --- | --- |
| BR1 (Deployment, image reference, resources) | §3 |
| BR2 (Service) | §4 |
| BR3 (ConfigMap/Secret) | §2 |
| BR4 (HPA, owner of replicas) | §3, §5 |
| BR5 (probes, termination) | §3 |
| BR6 (hardening) | §3, §6, §7 |
| BR7 (Jobs) | §6 |
| BR8 (order, never wholesale) | §1, §6, §8 |
| BR9 (contract names) | §1, §2, §6 |
| BR10 (evidence tooling) | §7 |
| AC1–AC9 | §2–§9 |
