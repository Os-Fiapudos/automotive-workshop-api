# Requirements — Infrastructure Provisioning (Terraform, `/infra`)

Status: **Proposed** (depends on `GAPS-FASE-2.md` §7 item 1 — local vs. cloud cluster, not
yet confirmed by FIAP — see §7)
New top-level folder: `/infra`.

## 1. Context

Source: `SOAT - Fase 2 - Tech challenge.pdf` p.4, "Infraestrutura como Código (IaC): Criar
scripts em Terraform para provisionamento do cluster Kubernetes (local ou cloud); Banco de
Dados; Documentar quais recursos estão sendo criados e como aplicar." `GAPS-FASE-2.md` §3.2
confirms no `/infra` folder exists today and that `terraform` was not available in the
environment used for that analysis.

**Scope boundary with `specs/kubernetes-deployment/`**: per that spec's §1, the PDF
separates the Kubernetes *application* manifests (`/k8s`: Deployment/Service/ConfigMap/
Secret/HPA — no database) from what Terraform provisions (cluster **and** database). This
spec therefore owns: the Kubernetes cluster itself, the `metrics-server` the HPA depends on,
the project's `Namespace` (coordinated with `specs/kubernetes-deployment/` §1, created once),
and Postgres — provisioned here as a Kubernetes `StatefulSet` (not `/k8s`, not a cloud
managed database — team decision, `GAPS-FASE-2.md` §7 item 9: "Postgres como StatefulSet").

Team decision on local vs. cloud (2026-10-12, `GAPS-FASE-2.md` §7 item 1): **default to a
local cluster (kind)**, pending confirmation of whether FIAP provides cloud credits/access;
this spec is written so switching to a cloud provider later is a Terraform *provider*
change, not a re-architecture (§6).

## 2. User story

> As whoever needs a Kubernetes cluster and a Postgres database to deploy this API into
> (a developer running the Fase 2 demo locally, or the CI/CD pipeline provisioning an
> ephemeral/shared environment),
> I want `terraform apply` to create both from a documented, versioned configuration,
> so that the environment is reproducible and not a set of manual `kind create cluster`/
> `kubectl` commands nobody wrote down.

## 3. Business rules

1. `terraform apply` provisions a Kubernetes cluster (local: `kind`, via the `tehcyx/kind`
   Terraform provider; cloud: a managed cluster via that cloud's Terraform provider — a
   `cluster.tf` swap, §6) sized and configured appropriately for this project's local demo
   needs (a single node is sufficient; HPA scaling is demonstrated by pod count, not node
   count).
2. `terraform apply` installs `metrics-server` into the cluster (via Terraform's
   `helm`/`kubernetes` provider) — a prerequisite for `specs/kubernetes-deployment/`'s HPA to
   report real metrics, not optional.
3. `terraform apply` provisions Postgres as a Kubernetes `StatefulSet` with a
   `PersistentVolumeClaim` (so data survives a pod restart, not a cluster teardown — cluster
   teardown is expected to be destructive, same posture `docker compose down -v` already has
   locally) and a headless `Service` other workloads (the API's own `Deployment`) address it
   through.
4. Every resource Terraform creates is named/labeled so `terraform destroy` removes exactly
   what `terraform apply` created — no manual cleanup step, no resource Terraform doesn't
   track.
5. No credential (Postgres password, cloud provider credentials) is hardcoded in a `.tf`
   file — Terraform variables (`variable "postgres_password" { sensitive = true }`), sourced
   from a gitignored `terraform.tfvars` or environment variables
   (`TF_VAR_postgres_password`), the same "secrets come from the environment, never
   committed" rule `CLAUDE.md` §13 already states project-wide.
6. `terraform apply`'s output includes (via `output` blocks) whatever
   `specs/kubernetes-deployment/`'s `Secret` needs to be populated with for `DATABASE_URL`
   (host, port, database name — not the password itself in plain `terraform output`, unless
   marked `sensitive`) — so populating that `Secret` is a copy-paste from `terraform output`,
   not a second source of truth for connection details.
7. Documentation lists exactly which resources are created and the exact commands to apply/
   destroy them (PDF's explicit "Documentar quais recursos estão sendo criados e como
   aplicar").

## 4. Scope

In scope: `/infra/*.tf` (cluster, metrics-server, namespace, Postgres `StatefulSet`/PVC/
Service, variables, outputs), `/infra/README.md` (or a `specs/phase-2-deliverables/`-linked
section of the project README — decided in that spec, not duplicated), `/infra/terraform.tfvars.example`.

Out of scope: the API's own Kubernetes manifests (`specs/kubernetes-deployment/`), CI/CD's
own `terraform apply`/`destroy` invocation (`specs/ci-cd-pipeline/`), the migration tooling
itself (`specs/database-migrations/` — Terraform only creates the Postgres *instance*, the
migration `Job` that creates its schema is `/k8s`'s job, per that spec's §6).

## 5. Acceptance criteria

1. `terraform apply` from a clean state creates a working `kind` cluster (or the chosen
   cloud cluster, §7), `metrics-server` reporting real pod metrics within a few minutes,
   a `Namespace`, and a Postgres `StatefulSet` reachable from inside the cluster.
2. `terraform output` prints the values `specs/kubernetes-deployment/`'s `Secret` needs for
   `DATABASE_URL`, with the password marked `sensitive` (not printed by a bare
   `terraform output`, only `terraform output -raw postgres_password` or the `tfvars`
   file itself).
3. `terraform destroy` removes every resource `terraform apply` created, leaving no
   orphaned `kind` cluster, Docker network, or volume.
4. Documentation lists every resource Terraform creates and the exact `terraform
   init`/`plan`/`apply`/`destroy` commands, runnable by someone who has never run this
   project's Terraform before.
5. No real secret value is committed — `terraform.tfvars.example` has placeholders only,
   `terraform.tfvars` itself is `.gitignore`d.

## 6. Local-vs-cloud: how the swap stays contained

`cluster.tf` is the one file that changes between local and cloud: locally, a `kind_cluster`
resource (`tehcyx/kind` provider); on a cloud provider, that provider's own managed-cluster
resource (e.g. `google_container_cluster`, `aws_eks_cluster` — not decided, since the
provider itself depends on §7). Every other `.tf` file (`metrics-server.tf`, `postgres.tf`,
`namespace.tf`) targets the resulting cluster through Terraform's generic `kubernetes`/
`helm` providers, configured from whichever cluster resource's own outputs (kubeconfig,
endpoint, CA cert) — so only `cluster.tf` and the provider block at the top of the
configuration are cloud-specific; the Postgres/metrics-server/namespace resources are
written once and apply to either target.

## 7. Open question — confirm before implementation

Same open item as `specs/kubernetes-deployment/requirements.md` §7, tracked once here:
**local (`kind`) or a specific cloud provider?** This spec's working default is local
`kind`, chosen per `GAPS-FASE-2.md` §7 item 1's recommendation (cheap, reproducible in the
video, no account/credits dependency) — confirm before `tasks.md` is executed; if cloud is
chosen instead, only `cluster.tf` (§6) and the provider credentials variables change.
