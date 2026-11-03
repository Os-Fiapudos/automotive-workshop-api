# Design — Infrastructure Provisioning (Terraform)

Satisfies: `requirements.md` (all sections). Revised 2026-10-25 with it (`requirements.md`
§0): the original single `cluster.tf` sketch is replaced by the layout below, written to
FP-36's contract (`specs/ci-cd-pipeline/design.md` §9, items 1–5).

## 1. Layout

One Terraform root, `infra/`, local backend, one state:

| File | Content |
| --- | --- |
| `versions.tf` | `required_version` and the exact provider pins (§8) |
| `providers.tf` | `kind`, and `kubernetes`/`helm` configured from the cluster resource (§2) |
| `cluster.tf` | the `kind_cluster` (§3) |
| `namespace.tf` | the project `Namespace` (§4) |
| `metrics-server.tf` | the `metrics-server` Helm release (§5) |
| `postgres.tf` | credentials `Secret`, `StatefulSet`, headless `Service` (§6) |
| `variables.tf` / `outputs.tf` | inputs and the outputs the deploy consumes (§7) |
| `terraform.tfvars.example` | `postgres_password = "REPLACE_ME"` only |
| `README.md` | runbook (§10) |

No modules: the configuration is small, and a module boundary would add indirection without
reuse. A `locals` block holds the names fixed by the contract (`automotive-workshop`
namespace, `postgres` service/StatefulSet) so they are not variables someone can drift.

## 2. One root without a two-step bootstrap (BR8)

```hcl
provider "kubernetes" {
  host                   = kind_cluster.this.endpoint
  cluster_ca_certificate = kind_cluster.this.cluster_ca_certificate
  client_certificate     = kind_cluster.this.client_certificate
  client_key             = kind_cluster.this.client_key
}

provider "helm" {
  kubernetes = { host = …, cluster_ca_certificate = …, client_certificate = …, client_key = … }
}
```

On a clean `apply` those attributes are unknown at plan time. That is safe for the resources
used here — `kubernetes_namespace_v1`, `kubernetes_secret_v1`, `kubernetes_stateful_set_v1`,
`kubernetes_service_v1`, `helm_release` — which plan a create without contacting the
cluster; Terraform creates `kind_cluster` first because every other resource depends on it
through the provider configuration. `kubernetes_manifest` is deliberately **not** used: it
needs the cluster's API at plan time and would force the two-state split the story warns
about. `destroy` works the same way in reverse: the provider configuration comes from the
state, the Kubernetes resources are removed first, then the cluster.

The one case this does not cover is a cluster deleted **outside** Terraform (`kind delete
cluster`, Docker reset): the next plan cannot refresh the Kubernetes resources. The README
documents the recovery — remove the stale state (`terraform state rm` of the Kubernetes/Helm
resources, or of everything when the cluster is gone too) and apply again — and the
`-target=kind_cluster.this` two-step as a fallback. This is preferred to two states because
FP-36's pipeline and `make deploy` run exactly one `terraform apply` on `infra/`.

## 3. `cluster.tf`

```hcl
resource "kind_cluster" "this" {
  name            = var.cluster_name           # "automotive-workshop"
  node_image      = var.node_image             # kindest/node:v1.37.0@sha256:… (§8)
  wait_for_ready  = true
  kubeconfig_path = local.kubeconfig_path      # abspath("${path.module}/.kube/<name>.kubeconfig")

  kind_config {
    kind        = "Cluster"
    api_version = "kind.x-k8s.io/v1alpha4"
    node { role = "control-plane" }
  }
}
```

- `kubeconfig_path` is an absolute path inside `infra/.kube/` (git-ignored): FP-36's
  script calls `terraform -chdir=infra output -raw kubeconfig_path` from the repository root
  and checks the file exists, so a relative path would break it. The provider writes the
  kubeconfig there and removes the context on destroy.
- A single node, no port mappings: the API is reached through `kubectl port-forward` for the
  demo and through the `Service` inside the cluster for the load test (FP-34).
- The node image is the default of the `kind` library the provider embeds
  (`sigs.k8s.io/kind v0.33.0` → Kubernetes 1.37.0), pinned by digest. **Hosts on cgroup v1**
  cannot run kubelet ≥ 1.35 (it refuses cgroup v1); the README tells them to pass an older
  node image through `node_image` (verified with `kindest/node:v1.34.3`) or move the host to
  cgroup v2.

## 4. `namespace.tf`

`kubernetes_namespace_v1` `automotive-workshop`, labelled
`app.kubernetes.io/part-of=automotive-workshop`, `app.kubernetes.io/managed-by=terraform`
and the Pod Security Admission labels `enforce=restricted`, `audit=restricted`,
`warn=restricted` (pinned to `latest`). Every workload in it — Postgres (§6) and all of
FP-34's pods — is written to the `restricted` profile, so the label turns a regression into
a rejected pod instead of a silent one. Terraform is its only owner (`requirements.md`
BR4); `/k8s` has no `Namespace` manifest, and FP-36's script writes its deploy metadata to
its own `ConfigMap`, not to this object.

## 5. `metrics-server.tf`

```hcl
resource "helm_release" "metrics_server" {
  name       = "metrics-server"
  repository = "https://kubernetes-sigs.github.io/metrics-server/"
  chart      = "metrics-server"
  version    = "3.13.1"            # app v0.8.1
  namespace  = "kube-system"
  wait       = true
  timeout    = 300
  values     = [yamlencode({ args = ["--kubelet-insecure-tls"] })]
}
```

`args` is appended to the chart's `defaultArgs` (`--cert-dir`, `--kubelet-preferred-address-
types`, `--kubelet-use-node-status-port`, `--metric-resolution=15s`). `--kubelet-insecure-tls`
is the one `kind`-only value (`requirements.md` §6).

## 6. `postgres.tf`

- `kubernetes_secret_v1` `postgres-credentials` (`username`, `password`) — used only by the
  Postgres pod. The API gets its `DATABASE_URL` from FP-36's `Secret`, built from the output
  of §7.
- `kubernetes_stateful_set_v1` `postgres`:
  - `replicas = 1`, `service_name = "postgres"`, labels
    `app.kubernetes.io/{name=postgres,component=database,part-of=automotive-workshop}`.
  - Image `var.postgres_image` = `postgres:16.15-alpine3.24@sha256:…` (§8); Compose uses the
    same Postgres major.
  - `POSTGRES_USER`/`POSTGRES_DB` from variables, `POSTGRES_PASSWORD` from the `Secret`,
    `PGDATA=/var/lib/postgresql/data/pgdata` (a subdirectory: `initdb` refuses a non-empty
    mount point).
  - `args = ["-c", "max_connections=${var.postgres_max_connections}"]` (default 300). Budget:
    the API's `pgxpool` opens up to `max(4, NumCPU)` connections per pod, and `NumCPU` is the
    host's CPU count inside a container; 8 pods (HPA max) × 32 CPUs + the `Job`s fits 300.
  - Restricted security context: `runAsUser/runAsGroup/fsGroup = 70` (the image's `postgres`
    user — the entrypoint skips its root-only `chown` step when already running as that
    user), `runAsNonRoot`, `RuntimeDefault` seccomp, no privilege escalation, all
    capabilities dropped. `/var/run/postgresql` (the socket directory) is an `emptyDir`.
  - Probes: `startup` (`pg_isready` every 2 s, up to 60 s for the first `initdb`),
    `readiness` and `liveness` — all `pg_isready -h 127.0.0.1` (TCP: during the first start
    the image's temporary server listens on the socket only, so a socket check passes too
    early — same reason FP-36's script checks over TCP).
  - Resources: requests `250m`/`256Mi`, limits `1`/`512Mi`.
  - `volume_claim_template` `data`, `ReadWriteOnce`, `var.postgres_storage_size` (1 Gi), the
    cluster's default `StorageClass` (`standard`, kind's local-path provisioner);
    `persistent_volume_claim_retention_policy` `Retain` on delete and scale — deleting the
    pod or the `StatefulSet` keeps the data; only destroying the cluster removes it.
  - `wait_for_rollout = true`: `apply` returns only when the pod is ready (§6 probes).
- `kubernetes_service_v1` `postgres`: headless (`cluster_ip = "None"`), port 5432 — the
  stable DNS name `postgres.automotive-workshop.svc.cluster.local`.

## 7. `variables.tf` / `outputs.tf`

| Variable | Default | Notes |
| --- | --- | --- |
| `postgres_password` | — | `sensitive`, required, validated ≥ 12 characters |
| `postgres_user` / `postgres_db` | `workshop` / `automotive_workshop` | same as Compose |
| `cluster_name` | `automotive-workshop` | also names the kubeconfig file |
| `node_image` | `kindest/node:v1.37.0@sha256:a1ed…` | override for cgroup v1 hosts (§3) |
| `postgres_image` | `postgres:16.15-alpine3.24@sha256:f461…` | |
| `postgres_storage_size` | `1Gi` | |
| `postgres_max_connections` | `300` | §6 |

| Output | Value |
| --- | --- |
| `cluster_name`, `kube_context` | `kind_cluster` name and context |
| `kubeconfig_path` | absolute path of the kubeconfig (§3) |
| `namespace` | `automotive-workshop` |
| `postgres_host` / `postgres_port` | `postgres.automotive-workshop.svc.cluster.local` / `5432` |
| `postgres_database` / `postgres_user` | from the variables |
| `postgres_database_url_template` | `postgres://<user>:<password>@<host>:5432/<db>?sslmode=disable` — the literal `<password>` is replaced by the deploy (FP-36 §4) |

`sslmode=disable` is the documented decision for this in-cluster Postgres, reached only over
the cluster network (`CLAUDE.md` §13 asks for it to be explicit): the database has no TLS
certificate, and traffic never leaves the node. A database outside the cluster would need
`sslmode=require` or stricter. No output is `sensitive` because none contains a credential.

## 8. Pinned versions (verified 2026-10-25)

| Item | Pin | Source of the check |
| --- | --- | --- |
| Terraform | `>= 1.16.0, < 2.0.0` (CI uses 1.16.6, as FP-36) | releases.hashicorp.com |
| `tehcyx/kind` | `0.13.0` (embeds `sigs.k8s.io/kind v0.33.0`) | module source |
| `hashicorp/kubernetes` | `3.4.0` | releases.hashicorp.com |
| `hashicorp/helm` | `3.3.0` (v3 syntax: `kubernetes = {}`, `set = [...]`) | releases.hashicorp.com |
| Node image | `kindest/node:v1.37.0@sha256:a1ed56cfb0e7b93589bdf97c8cd566405a265939e3620fc4f5de89adff580ae5` | kind v0.33.0 default |
| metrics-server chart | `3.13.1` (app `v0.8.1`) | chart in the upstream repository at `v0.9.0` |
| Postgres | `postgres:16.15-alpine3.24@sha256:f4610795587492cd4fb887c2dc3de301b813278bb6f616992061141b5f15ac36` | Docker Hub |

The dependency lock file (`.terraform.lock.hcl`) is committed once generated against the
public registry (`terraform providers lock -platform=linux_amd64 -platform=darwin_arm64
-platform=windows_amd64 …`), so every machine verifies the same provider checksums.

## 9. Verification (`requirements.md` AC1–AC9)

Static, on every change (`.github/workflows/iac.yml`): `terraform fmt -check -recursive`,
`terraform init -backend=false`, `terraform validate`.

Operational, recorded in `tasks.md` §4: `apply` from a clean state → `kubectl top nodes`,
`pg_isready`/`psql` from a pod in the namespace (AC1, AC2) → `terraform output` (AC3) →
second `plan` empty (AC6) → `scripts/k8s-evidence.sh db-persistence` (AC7: writes a marker
in a separate database — never in the application database, whose baseline migration
refuses a non-empty schema — deletes `postgres-0`, reads the marker back) → `destroy` and
`docker ps -a`/`kind get clusters` empty (AC8).

## 10. `infra/README.md`

Resource inventory (table of every object and who owns it), prerequisites (Docker, Terraform
≥ 1.16, `kubectl`), the cgroup v1 note, the commands (`init`, `plan`, `apply`, `output`,
`destroy`, with `TF_VAR_postgres_password`), connecting (`export KUBECONFIG=$(terraform
-chdir=infra output -raw kubeconfig_path)`, `kubectl port-forward svc/postgres`), state
protection (local, git-ignored, contains secrets — never commit, upload or share), the
teardown rule, the recovery of §2, the persistence check, the hand-off to `/k8s` and to
FP-36's `scripts/deploy.sh`, and the limits (one node, one Postgres replica: not high
availability; HPA adds pods, not nodes).

## 11. Traceability

| Requirement | Design section |
| --- | --- |
| BR1 (cluster, single owner) | §3 |
| BR2 (metrics-server) | §5 |
| BR3 (Postgres StatefulSet, PVC, Service, readiness, capacity) | §6 |
| BR4 (one owner per resource) | §1, §4, §6 |
| BR5 (credentials) | §6, §7 |
| BR6 (outputs) | §7 |
| BR7 (state) | §1, §10 |
| BR8 (one root, bootstrap ordering) | §2 |
| BR9 (pins) | §8 |
| BR10 (documentation) | §10 |
| AC1–AC9 | §3–§10 |
