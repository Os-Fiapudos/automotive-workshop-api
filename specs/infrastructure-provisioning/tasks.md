# Tasks — Infrastructure Provisioning (Terraform)

Jira **FP-35**. Revised 2026-10-25 with `requirements.md` §0.

## 1. Spec confirmation

- [x] Local `kind` vs. cloud closed: local `kind` (`requirements.md` §7).
- [x] Integration contract with FP-36 (`specs/ci-cd-pipeline/design.md` §9 items 1–5) and
      FP-33 (`specs/database-migrations/design.md` §9) reviewed; names and outputs follow it.

## 2. Terraform configuration (`design.md` §1–§8)

- [x] `infra/versions.tf` — exact provider pins.
- [x] `infra/providers.tf` — `kubernetes`/`helm` from the `kind_cluster` attributes; no
      `kubernetes_manifest` anywhere (`design.md` §2).
- [x] `infra/cluster.tf` — single node, pinned image, absolute `kubeconfig_path`.
- [x] `infra/namespace.tf` — `automotive-workshop`, Pod Security `restricted`.
- [x] `infra/metrics-server.tf` — chart `3.13.1`, `--kubelet-insecure-tls`.
- [x] `infra/postgres.tf` — `Secret`, `StatefulSet` (TCP probes, resources, non-root,
      `max_connections`, PVC `Retain`), headless `Service`.
- [x] `infra/variables.tf` / `infra/outputs.tf` — `postgres_password` sensitive/required;
      `kubeconfig_path` and `postgres_database_url_template` per the contract.
- [x] `infra/terraform.tfvars.example`; `.gitignore` covers `infra/.terraform/`,
      `*.tfstate*`, `terraform.tfvars`, `infra/.kube/`, `*.tfplan`.
- [x] `Makefile` `infra-*` targets.
- [x] `.terraform.lock.hcl` committed, generated against the public registry for the team's
      platforms (`design.md` §8). Done 2026-10-26 with Terraform 1.16.6 (`hashicorp/terraform`
      image, the CI pin): `terraform providers lock` for linux_amd64, darwin_arm64,
      darwin_amd64 and windows_amd64; `init -lockfile=readonly` + `validate` pass with it.

## 3. Static verification

- [x] `terraform fmt -check -recursive infra`, `terraform init -backend=false`,
      `terraform validate`.
- [x] `.github/workflows/iac.yml` runs the three on changes to `infra/**`.

## 4. Operational verification (`design.md` §9)

### 4.1 Run of 2026-10-25 (environment that implemented the feature)

Environment: Linux 6.18 VM, **cgroup v1**, Docker 29.9.2, Terraform 1.16.6. The Terraform
registry, GitHub releases and `kubernetes-sigs.github.io` were blocked, so providers came
from a filesystem mirror (`hashicorp/*` from releases.hashicorp.com; `tehcyx/kind` 0.13.0
built from its module source) and the chart from the upstream source at `v0.9.0`.

- **Real `kind` could not boot here**, so AC1, AC7 and AC8 still need a run on a developer
  machine (§4.2). kubelet ≥ 1.35 refuses cgroup v1 (default node 1.37.0), and with
  `node_image = kindest/node:v1.34.3` the nested `runc` inside the node failed
  (`can't get final child's PID from pipe`), a limit of this sandbox, not of the
  configuration. The failed create left no cluster behind (`kind get clusters` empty).
- **BR8 (single root)** — on a clean state, `terraform apply` planned all six resources
  (`kind_cluster`, `helm_release`, namespace, secret, service, StatefulSet) with the
  provider configuration still unknown, and started creating the cluster first.
- **AC2/AC4/AC6 against a real API server** — etcd 3.6.5 + `kube-apiserver` and
  `kube-controller-manager` v1.34.3 (no scheduler/kubelet), `infra/namespace.tf` +
  `postgres.tf` applied with the real `kubernetes` provider 3.4.0: namespace with
  `restricted` labels, `postgres-credentials`, headless `Service`, `StatefulSet` created;
  **`postgres-0` admitted under `restricted`** after a fix (the provider sends
  `runAsNonRoot=false` for the container block unless it is set — caught here, fixed in
  `postgres.tf`); second `plan` → no changes. No object of the deploy contract was created by
  Terraform (AC4).
- **Postgres runtime settings** — the pinned image ran as UID 70 with all capabilities
  dropped, `PGDATA` in a subdirectory, the socket directory on a writable tmpfs:
  `pg_isready -h 127.0.0.1` ready after 3 s, `show max_connections` → 300, and the FP-33
  migrations applied on it (`0001_init`, `0002`).
- **AC3/AC5** — outputs reviewed: no credential, `postgres_database_url_template` has
  `<password>`; `git ls-files infra` holds no state, tfvars or kubeconfig.

### 4.2 First run on a cgroup v2 machine with Docker (2026-10-27)

Record the environment (host, node image, versions) and results here.

**Run of 2026-10-27 (UTC)** — Linux x86_64, cgroup v2, Docker 29.9.2, 8 CPUs / 16 GiB;
Terraform 1.16.6, kubectl v1.36.5, `kindest/node:v1.37.0` (pinned digest), image built from
commit `4c731a0` and loaded into the node (no GHCR image published yet). Driven by
`scripts/deploy.sh` (DEPLOY_ID `local-evidencia`) and `scripts/k8s-evidence.sh`; raw output in
the git-ignored `evidence/` and `deploy-evidence/` of that machine, summarized in the root
README ("Evidências da execução").

- [x] AC1 — clean `apply`: cluster, `metrics-server` (`kubectl top nodes`), namespace,
      Postgres reachable inside the cluster. 6 resources in ~90 s, one root, one state;
      `kubectl top nodes` answered; the API migrated and served from it.
- [x] AC2 — `postgres-0` ready under the `restricted` profile on a real node;
      `deploy.sh infra` confirmed `pg_isready` over TCP before continuing.
- [x] AC3 — `terraform output`: `kubeconfig_path`, `postgres_database_url_template` with
      `<password>`, no credential.
- [x] AC4 — no object created by both Terraform and `/k8s`/the deploy flow.
- [x] AC5 — no secret committed (`git ls-files infra` reviewed).
- [x] AC6 — second `terraform plan -detailed-exitcode` on the full root: exit 0, "No changes".
- [x] AC7 — `scripts/k8s-evidence.sh db-persistence`: new pod UID, same PVC
      `data-postgres-0`, marker read back; authenticated smoke passed again afterwards.
- [x] AC8 — `destroy` on the disposable environment: 6 destroyed, no `automotive-workshop`
      container, empty state (twice: demo environment and the failed-migration one).

## 5. Documentation

- [x] `infra/README.md` (`design.md` §10).
- [x] Root `README.md` section linking to it; `CLAUDE.md` and `specs/architecture.md`
      updated.

## 6. Hand-off

- [x] FP-34: `/k8s` deploys against this environment with the `Secret` built from
      `postgres_database_url_template` (`deploy.sh config`).
- [x] FP-36: `scripts/deploy.sh check` (PR #42) passes against `infra/` + `k8s/`.
- [x] Re-check `requirements.md` AC1–AC9.
