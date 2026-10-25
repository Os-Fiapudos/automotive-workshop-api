# Requirements — Infrastructure Provisioning (Terraform, `/infra`)

Status: **Approved for implementation** (2026-10-25 — revised to the FP-35 story, see §0)
Jira: **FP-35** — "[Fase 2] Infraestrutura e banco provisionados com Terraform" (planning id
F2-US07, manual requirement R09). Consumed by **FP-34** (`specs/kubernetes-deployment/`,
`/k8s`) and **FP-36** (`specs/ci-cd-pipeline/`, `scripts/deploy.sh`).
New top-level folder: `/infra`. Also touches `.gitignore`, `Makefile`, the new
`.github/workflows/iac.yml` (static validation, shared with FP-34) and documentation
(`README.md`, `CLAUDE.md`, `specs/architecture.md`).

## 0. Revision note (2026-10-25)

The first version of this spec (PR #30) predates the FP-35 Jira story and the integration
contracts that FP-33 (`specs/database-migrations/design.md` §9) and FP-36
(`specs/ci-cd-pipeline/design.md` §9, PR #42) later wrote against `/infra`. The story's
acceptance checklist and those contracts change the original in these places (each is
called out where it applies):

1. **Local `kind` confirmed.** The open question of §7 is closed: the team's target for this
   phase is a local `kind` cluster (no cloud account). The cloud swap of the original §6 is
   kept only as a documented boundary, not as something to build.
2. **One owner per resource** (BR4). Terraform owns the cluster, the `Namespace`,
   `metrics-server` and Postgres; nothing else creates them. The original design let
   `k8s/namespace.yaml` and Terraform both create the `Namespace` and assumed the second
   creation would be a no-op — `/k8s` no longer has a `Namespace` manifest.
3. **Single root, single state, with the bootstrap problem handled explicitly** (BR8). The
   story asks to split cluster bootstrap and Kubernetes resources into separate steps/states
   *when necessary*. FP-36's deploy script runs one `terraform apply` on one root (`infra/`),
   so the split is avoided by construction: only resources that need no cluster API at plan
   time are used (no `kubernetes_manifest`), and the recovery path for a cluster removed
   outside Terraform is documented instead of a second state.
4. **Outputs consumed by the pipeline** (BR6): `kubeconfig_path` and
   `postgres_database_url_template` (with the literal `<password>`), named exactly as
   FP-36's contract expects. The original only had host/port/template.
5. **Postgres readiness and capacity** (BR3): readiness probe with `pg_isready` over TCP
   (so "rollout complete" means "accepting connections"), resource requests/limits, and
   `max_connections` sized for the API's HPA ceiling.
6. **Pinned, verified versions** (BR9) replace the illustrative pins of the original
   (`tehcyx/kind ~> 0.7`, `kubernetes ~> 2.31`, `helm ~> 2.14`, chart `3.12.1`,
   `postgres:16-alpine`).
7. **Evidence of the lifecycle** (AC6–AC8, new): repeated apply produces an empty plan, a
   recreated Postgres pod keeps its data, and destroy is verified only on a disposable
   environment.

## 1. Context

Source: `SOAT - Fase 2 - Tech challenge.pdf` p.4, "Infraestrutura como Código (IaC): Criar
scripts em Terraform para provisionamento do cluster Kubernetes (local ou cloud); Banco de
Dados; Documentar quais recursos estão sendo criados e como aplicar." p.6: "Scripts
Terraform (em /infra)".

**Scope boundary with `specs/kubernetes-deployment/`**: the PDF separates the Kubernetes
*application* manifests (`/k8s`: Deployment/Service/ConfigMap/Secret/HPA) from what
Terraform provisions (cluster **and** database). This spec owns the cluster, the
`metrics-server` the HPA depends on, the project's `Namespace` and Postgres — provisioned as
a Kubernetes `StatefulSet` inside the cluster (team decision, `GAPS-FASE-2.md` §7 item 9).
The application, its configuration and the migration/bootstrap `Job`s are `/k8s`'s
(FP-34), applied by the deploy flow (FP-36), never by Terraform.

## 2. User story

> As the operations team, I want to provision the cluster and the database from code, so
> that the environment can be recreated and the resources it needs are known. *(FP-35)*

## 3. Business rules

1. **Cluster.** `terraform apply` creates a `kind` cluster (Kubernetes in Docker) with a
   single control-plane node and a pinned node image. One node is enough: the phase
   demonstrates pod autoscaling (HPA), not node autoscaling. Terraform is the cluster's only
   creator and destroyer — no `kind create cluster`, no `helm/kind-action` (FP-36 §0 item 1).
2. **Metrics.** `terraform apply` installs `metrics-server` (Helm chart, pinned) in
   `kube-system`, so the HPA reads real CPU/memory metrics. On `kind` the kubelet serves a
   self-signed certificate, so `--kubelet-insecure-tls` is set — a local-cluster-only flag.
3. **Database.** `terraform apply` provisions Postgres 16 (same major as Docker Compose) as a
   `StatefulSet` `postgres` (one replica, pod `postgres-0`) with a `PersistentVolumeClaim`
   (data survives the pod, not the cluster) and an internal headless `Service` `postgres`
   on port 5432. The pod reports ready only when Postgres accepts TCP connections, declares
   resource requests/limits, runs as a non-root user, and allows enough connections for
   the API's HPA ceiling plus the migration/bootstrap `Job`s.
4. **One owner per resource.** Terraform: cluster, `Namespace` `automotive-workshop`,
   `metrics-server`, Postgres (`StatefulSet`, `Service`, credentials `Secret`
   `postgres-credentials`). `/k8s` + the deploy flow: the API, its `Secret`s/`ConfigMap`s
   and the `Job`s. Terraform never creates an object the deploy flow owns (FP-36 §9 item
   13), and an object created by another owner is adopted with `terraform import`, never
   assumed to be a no-op.
5. **Credentials.** No credential in any `.tf` file or committed file: `postgres_password`
   is a `sensitive` variable with no default, supplied through `TF_VAR_postgres_password` or
   a git-ignored `terraform.tfvars`. Outputs never contain the password.
6. **Outputs for the deploy.** Outputs include `kubeconfig_path` (the kubeconfig of this
   cluster, written by Terraform, so nothing depends on the developer's current context)
   and `postgres_database_url_template` (a complete `DATABASE_URL` with the literal
   `<password>` and an in-cluster host), plus host, port, database, user and namespace.
7. **State.** Local backend: the state stays where `terraform` runs, is git-ignored, and is
   treated as sensitive (it contains the Postgres password and the cluster's client key even
   though the variable is `sensitive`). Nothing uploads or prints it.
8. **Bootstrap ordering in one root.** The `kubernetes`/`helm` providers are configured from
   the cluster resource's own attributes. Every Kubernetes/Helm resource used must be
   plannable while the cluster does not exist yet (no resource that reads the cluster API at
   plan time), so a clean `terraform apply` works in one step; the procedure for a cluster
   deleted outside Terraform is documented.
9. **Pinned versions.** Terraform, every provider, the node image, the Helm chart and the
   Postgres image are pinned to exact, verified versions; raising one is a reviewed change.
10. **Documentation.** `infra/README.md` lists every resource created, prerequisites, the
    exact `init`/`plan`/`apply`/`output`/`destroy` commands, how to connect, how the state
    is protected, the teardown rule (never as a deploy step of a persistent environment)
    and the limits of the environment.

## 4. Scope

In scope: `/infra/*.tf`, `/infra/terraform.tfvars.example`, `/infra/README.md`; the
`.gitignore` entries for state/tfvars/kubeconfig; `Makefile` `infra-*` targets; the
Terraform job of `.github/workflows/iac.yml` (`fmt`/`validate`); the database-persistence
check of `scripts/k8s-evidence.sh` (shared with FP-34).

Out of scope: the API manifests and `Job`s (FP-34); invoking `terraform apply`/`destroy`
from the pipeline (FP-36, which already calls this root); the migration tooling (FP-33);
high availability of Postgres (one replica, one node — documented as a limit); a remote
Terraform backend (no shared environment exists to justify one).

## 5. Acceptance criteria

1. From a clean state, `terraform apply` creates the `kind` cluster, `metrics-server`
   (`kubectl top nodes` reports values within a few minutes), the `Namespace` and a Postgres
   reachable from inside the cluster at `postgres.automotive-workshop.svc.cluster.local:5432`.
2. The `StatefulSet` reaches ready only when `pg_isready` succeeds over TCP; the API's
   connection budget (HPA max × pool size + `Job`s) fits `max_connections`.
3. `terraform output` prints `kubeconfig_path` and `postgres_database_url_template`
   (containing `<password>`) and no credential; `postgres_password` is `sensitive` with no
   default.
4. No resource Terraform creates is also created by `/k8s` or by the deploy flow.
5. No real secret is committed: `terraform.tfvars.example` has a placeholder only;
   `terraform.tfvars`, `*.tfstate*`, `.terraform/` and the kubeconfig are git-ignored.
6. A second `terraform plan` right after `apply` reports no changes.
7. Deleting `postgres-0` brings up a new pod that serves the data written before the
   deletion.
8. `terraform destroy` on a disposable environment removes the cluster with no `kind`
   container left behind; the README forbids it as a deploy step of a persistent one.
9. `infra/README.md` documents resources, prerequisites, commands, connection, state
   protection, teardown and limits (BR10).

## 6. Local-vs-cloud boundary

Only `cluster.tf` and the provider blocks depend on `kind`. `namespace.tf`,
`metrics-server.tf` (minus `--kubelet-insecure-tls`) and `postgres.tf` use the generic
`kubernetes`/`helm` providers and apply unchanged to another cluster. A cloud target would
replace `cluster.tf` with that provider's managed cluster and, most likely, Postgres with a
managed database — a different spec revision, not built here.

## 7. Open question — closed

**Local `kind` or a cloud provider?** Closed 2026-10-25: local `kind` (§0 item 1), the
team's working default since 2026-10-12 and the target FP-36's pipeline was written for.
