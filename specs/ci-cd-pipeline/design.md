# Design — CI/CD Pipeline (Fase 2)

Satisfies: `requirements.md` (all sections). Revised 2026-10-21 with it (`requirements.md`
§0); the original design's `helm/kind-action` + `docker run --network host` + `sed "<tag>"`
flow is replaced by the one below.

## 1. Workflow structure

One workflow, `.github/workflows/ci.yml` (name `CI`, unchanged), so the deploy can `needs:`
the existing checks by their real job ids — a second workflow chained with `workflow_run`
cannot express "these four jobs of this commit passed" as directly.

```yaml
on:
  push:                 # every branch: checks + image build
  pull_request:         # checks + image build, nothing else
  workflow_dispatch:    # manual deploy (input: simulate_migration_failure)
permissions:
  contents: read        # default for every job (BR10)
```

| Job | Runs on | Needs | Token permissions | Does |
| --- | --- | --- | --- | --- |
| `build` | every event | — | `contents: read` | unchanged: build, vet, test |
| `coverage` | every event | — | `contents: read` | unchanged: migrations + seed on Postgres, RNF06 |
| `migrations` | every event | — | `contents: read` | unchanged: `scripts/migrate.sh verify` |
| `security` | every event | — | `contents: read` | unchanged: `govulncheck`, `gosec` |
| `image` | every event | — | `contents: read` | builds the image, **no push** (validates the `Dockerfile` on PRs too) |
| `publish` | deploy events | `build`, `coverage`, `migrations`, `security`, `image` | `contents: read`, `packages: write` | builds (cache hit) and pushes `…:<sha>`; outputs the full reference |
| `deploy` | deploy events | all of the above | `contents: read`, `packages: read` | ephemeral environment: Terraform → config → migration → bootstrap → rollout → smoke → evidence → destroy |

"Deploy events" (BR4) is one expression, repeated on `publish` and `deploy`:

```yaml
if: github.event_name == 'workflow_dispatch' || (github.event_name == 'push' && github.ref == 'refs/heads/main')
```

Neither `if` calls a status function, so GitHub ANDs it with the implicit `success()` of every
job in `needs:` — a failed, cancelled or skipped required job skips `publish`, and therefore
`deploy` (BR1, AC3). On `pull_request` both are skipped, and no job a pull request runs has
`packages: write` or reads a secret (BR2): the `image` job builds with the default
read-only token. The rule, as documented in the README: **nothing is published unless the
four checks and the image build passed for that commit; nothing is deployed unless it was
published; nothing is rolled out unless the database and the migration succeeded.**

`deploy` also declares:

```yaml
environment:
  name: ci-ephemeral          # created by the first run; an org admin may add protection rules
concurrency:
  group: deploy-ci-ephemeral  # one deploy at a time (BR10)
  cancel-in-progress: false   # a running deploy is never cancelled by a newer one
timeout-minutes: 45
```

GitHub keeps at most one pending run per concurrency group: a third deploy queued behind a
second replaces it, which is the wanted behavior (the newer commit wins, the running one
finishes and destroys its environment).

## 2. Image (`image` and `publish`)

- **Name**: `ghcr.io/${GITHUB_REPOSITORY_OWNER,,}/automotive-workshop-api` — Bash lowercases
  the owner, so the organization `Os-Fiapudos` becomes `os-fiapudos` (BR3; registries reject
  uppercase).
- **Tag**: the full commit SHA (`${{ github.sha }}`). No `latest`: nothing deploys a mutable
  tag, so publishing one would only invite it.
- **Reference handed to the deploy**: `name:<sha>@<digest>`, built from
  `docker/build-push-action`'s `digest` output. The tag keeps it readable, the digest makes it
  immutable — Kubernetes pulls by the digest when both are present.
- **Labels**: `org.opencontainers.image.source` (links the package to the repository, which
  grants this repository's `GITHUB_TOKEN` access to it) and `org.opencontainers.image.revision`.
- **Cache**: `type=gha` (`mode=max`) on both jobs, so `publish` rebuilds from the layers the
  `image` job just produced instead of compiling again.
- **Pull credential (BR3)**: the package may be private (GHCR's default). The `deploy` job
  passes its own `GITHUB_TOKEN` (`packages: read`) to `scripts/deploy.sh config`, which
  creates a `kubernetes.io/dockerconfigjson` `Secret` named `ghcr-pull` and adds it to the
  namespace's `default` `ServiceAccount` before any pod using the image exists. If the
  package is made public, the step is harmless. Locally (demonstration environment) the same
  happens when `REGISTRY_USERNAME`/`REGISTRY_PASSWORD` are set (a token with
  `read:packages`); unset, the image must be public.

## 3. The `deploy` job

```yaml
env:
  DEPLOY_ID: ci-${{ github.run_id }}-${{ github.run_attempt }}
  DEPLOY_EPHEMERAL: "true"
  IMAGE: ${{ needs.publish.outputs.image }}
  SIMULATE_MIGRATION_FAILURE: ${{ github.event_name == 'workflow_dispatch' && inputs.simulate_migration_failure }}
  TF_IN_AUTOMATION: "1"
  TF_INPUT: "0"
steps:
  - checkout
  - hashicorp/setup-terraform   # terraform_version 1.16.6, terraform_wrapper: false (§6)
  - azure/setup-kubectl         # v1.36.5 (§11)
  - Generate ephemeral credentials          # §6.1
  - scripts/deploy.sh check                 # contract preflight (§9), before any resource exists
  - scripts/deploy.sh infra                 # Terraform apply + Postgres accepting connections
  - scripts/deploy.sh config                # Secrets, ConfigMap, pull credential (GITHUB_TOKEN)
  - scripts/deploy.sh migrate               # migration Job → Complete (or the run stops here)
  - scripts/deploy.sh bootstrap             # /bootstrap-admin Job → Complete
  - scripts/deploy.sh rollout               # Deployment/Service/HPA → rollout status
  - scripts/deploy.sh smoke                 # authenticated smoke (§8)
  - scripts/deploy.sh evidence   (always)   # §7, before the destroy
  - upload-artifact deploy-evidence-<DEPLOY_ID>   (always)
  - scripts/deploy.sh destroy    (always)   # Terraform destroy (BR11)
```

Each stage is its own step, so the run page shows exactly which one failed, and GitHub skips
the remaining non-`always()` steps after a failure — which is what keeps a failed migration
from reaching `rollout` (BR7) without any extra logic.

## 4. `scripts/deploy.sh`

One Bash script, used unchanged by the pipeline and by a developer for the demonstration
environment (BR12). Every stage is idempotent where it can be and recomputes what it needs
(kubeconfig path, `DATABASE_URL`, `Job` names) from Terraform outputs and `DEPLOY_ID`, so CI
can call the stages from separate steps without passing state between them.

| Stage | Does |
| --- | --- |
| `check` | Tools present (`terraform`, `kubectl`, `curl`, `jq`); `/infra` and the `/k8s` files of the contract (§9) present — otherwise a message naming FP-34/FP-35; `IMAGE` is a lowercase, complete reference that is not `latest` and has no placeholder; every manifest renders (§5) and no `<…>` placeholder survives rendering. |
| `infra` | `terraform init` + `apply` (`-json`, §6.3); kubeconfig from the `kubeconfig_path` output (never the developer's current context); waits for the `postgres` `StatefulSet` rollout and then for `pg_isready -h 127.0.0.1` inside the pod (accepting TCP connections, i.e. past the image's init restart). |
| `config` | Server-side applies the pipeline's own `ConfigMap` `automotive-workshop-deploy` (deploy id, ephemeral or not, commit, image, run URL — annotating Terraform's `Namespace` instead would be undone by the next `terraform apply`), the API `Secret`, the bootstrap `Secret`, the migrations `ConfigMap` (from `migrations/*.sql`, plus the failing file of §8 when requested) and, with registry credentials, the pull `Secret` + `ServiceAccount` patch. Values go through mode-`0600` temporary files, never command-line arguments. |
| `migrate` | Renders `k8s/migration-job.yaml` with the suffix `-<DEPLOY_ID>`, `kubectl create`s it, and waits for a `Complete` or `Failed` condition (timeout `MIGRATION_TIMEOUT`, 300 s). A pod that cannot start stops the wait early — `CreateContainerConfigError`/`InvalidImageName` at once, `ErrImagePull`/`ImagePullBackOff` after a minute of kubelet retries — instead of burning the timeout. Prints the `Job`'s pod logs either way; anything but `Complete` exits non-zero, naming the reason. |
| `bootstrap` | Same for `k8s/bootstrap-admin-job.yaml`, with the API image set to `IMAGE` (FP-33 §9: same tag as the `Deployment`). |
| `rollout` | Renders `deployment.yaml`, `service.yaml`, `hpa.yaml` (+ `configmap.yaml` if present) with `IMAGE`, applies them server-side, waits for `rollout status`, and checks the `Deployment`'s pod template carries exactly `IMAGE`. |
| `smoke` | `kubectl port-forward svc/automotive-workshop-api <SMOKE_LOCAL_PORT>:8080`, then `scripts/deploy-smoke.sh` with the administrator's credentials (§8). |
| `evidence` | §7. Runs after failures too, so a missing or unreachable cluster is reported, not fatal. It fails only when a credential survives sanitizing — and then deletes the evidence instead of leaving it to be uploaded. |
| `destroy` | `terraform destroy` (`-json`); a no-op when nothing was applied. |
| `up` | `check` → `infra` → `config` → `migrate` → `bootstrap` → `rollout` → `smoke` → `evidence`, in one process, for the demonstration environment. No destroy. |

Inputs (environment variables): `IMAGE`; `DEPLOY_ID` (default `local-<UTC timestamp>`);
`DEPLOY_EPHEMERAL`; `POSTGRES_PASSWORD` (exported to Terraform as `TF_VAR_postgres_password`);
`JWT_SECRET`, `EXTERNAL_NOTIFICATION_SECRET`, `EXTERNAL_STATUS_UPDATE_SECRET`; the optional
`RESEND_API_KEY`/`RESEND_FROM_EMAIL` pair; `ADMIN_EMAIL`, `ADMIN_PASSWORD`, optional
`ADMIN_NAME`; optional `REGISTRY_USERNAME`/`REGISTRY_PASSWORD`; `SIMULATE_MIGRATION_FAILURE`;
`DEPLOY_EVIDENCE_DIR` (default `deploy-evidence`, git-ignored); the timeouts
`POSTGRES_READY_TIMEOUT` (180 s), `MIGRATION_TIMEOUT` (300 s), `BOOTSTRAP_TIMEOUT` (180 s),
`ROLLOUT_TIMEOUT` (300 s); `SMOKE_LOCAL_PORT` (18080, away from Compose's 8080). A stage checks
only the variables it uses and names the missing ones, never their values.

`DATABASE_URL` is FP-35's `postgres_database_url_template` output with `<password>` replaced
by the percent-encoded `POSTGRES_PASSWORD`, so any password works in the URL.

## 5. Rendering the manifests

The committed `/k8s` files stay plain YAML (FP-34). The script never edits them; it copies the
ones a stage needs into a temporary directory, writes a `kustomization.yaml` next to them and
renders with `kubectl kustomize` — kustomize is built into `kubectl`, so no new tool:

```yaml
resources: [migration-job.yaml]          # or the rollout set
nameSuffix: -ci-18712345678-1            # Jobs only: a new, unique Job per run
commonAnnotations:
  automotive-workshop/deploy-id: ci-18712345678-1
  automotive-workshop/commit-sha: <sha>
images:
  - name: ghcr.io/os-fiapudos/automotive-workshop-api   # what FP-34 commits (any tag or none)
    newName: ghcr.io/os-fiapudos/automotive-workshop-api
    newTag: <sha>
    digest: sha256:<digest>
```

This yields `image: ghcr.io/os-fiapudos/automotive-workshop-api:<sha>@sha256:<digest>` in
every object that uses the API image (`Deployment`, bootstrap `Job`) and leaves the
`migrate/migrate` image untouched. Kustomize renames only the objects in that set, so the
`Secret`/`ConfigMap` references inside the `Job` keep their names. Annotations, not labels:
kustomize's `commonLabels` would rewrite the `Deployment` selector, which is immutable. A
rendered `Job` name over 63 characters (the label-value limit Kubernetes applies to it) is
refused by `check` before anything is created.

## 6. Credentials and logs (BR9)

### 6.1 Generated per run

```bash
for name in POSTGRES_PASSWORD JWT_SECRET EXTERNAL_NOTIFICATION_SECRET \
            EXTERNAL_STATUS_UPDATE_SECRET ADMIN_PASSWORD; do
  value=$(openssl rand -hex 32)          # 64 hex chars: ≥ 32 bytes for JWT, 12–72 for bcrypt
  echo "::add-mask::$value"              # masked before it is ever written anywhere
  echo "$name=$value" >> "$GITHUB_ENV"
done
echo "ADMIN_EMAIL=admin@${DEPLOY_ID}.ci.invalid" >> "$GITHUB_ENV"
```

The two `EXTERNAL_*` values are independent random values, so they always differ (the API
refuses to start when they are equal). Resend is left unset: an ephemeral run sends no
e-mail. Nothing here exists after the runner is gone, and no repository or environment
secret is read.

### 6.2 Inside the cluster

- Secrets and the `ConfigMap` are applied with `kubectl apply --server-side`: client-side
  apply would copy the whole object, values included, into the
  `last-applied-configuration` annotation, which `kubectl describe` prints.
- The API `Secret` holds `DATABASE_URL`, `JWT_SECRET`, both `EXTERNAL_*` and the optional
  Resend pair; the administrator's credentials go to a separate `Secret` that only the
  bootstrap `Job` reads, so API pods never receive the administrator's password.
- The evidence never describes or dumps a `Secret` — it lists their names and types only.

### 6.3 Terraform

The `kind` provider does not mark `kubeconfig`, `client_key` or `client_certificate` as
sensitive, so a plain `terraform destroy` prints the cluster's private key in its plan. The
script runs `apply` and `destroy` with `-json` and prints only each message's `@message`
(plus a diagnostic's `detail`) through `jq`: resource addresses and progress, never attribute
values or output values. `setup-terraform`'s wrapper is disabled (it captures stdout into step
outputs). The state stays in the runner's `infra/` directory and is never uploaded; the
kubeconfig is only ever referenced by path; `terraform output` is called only with `-raw` into
variables, for the two non-sensitive outputs of §9.

### 6.4 Sanitized evidence

Before the evidence directory is uploaded, the script replaces every credential value it knows
(the six generated values, `REGISTRY_PASSWORD`, `RESEND_API_KEY`) with `[REDACTED]`, masks the
password part of any `postgres://user:password@` URL, and then re-scans the directory: any
remaining occurrence fails the stage and nothing is uploaded. GitHub's log masking still
covers the job log; artifacts are not masked by GitHub, hence this step.

## 7. Evidence (BR11, AC7)

`scripts/deploy.sh evidence` writes to `deploy-evidence/` (uploaded as
`deploy-evidence-<DEPLOY_ID>`; its `summary.md` is also appended to the run's job summary):

| File | Content |
| --- | --- |
| `summary.md` | Environment id, ephemeral or not, commit SHA, deployed image reference, the image id the running pods resolved (`imageID`, the digest actually pulled), run URL, one row per stage (`ok`/`failed`, duration; a missing row means the stage did not run), the smoke output. |
| `stages.tsv` | Raw stage results, appended by each stage on exit. |
| `cluster.txt` | `kubectl get` of nodes, and of `jobs,pods,deploy,rs,svc,hpa,pvc,configmap,statefulset` in the namespace (`-o wide`), plus `Secret` names/types. |
| `migration-job.txt`, `bootstrap-job.txt` | `kubectl describe job` and the pod logs of each `Job` of this `DEPLOY_ID`. |
| `rollout.txt` | `rollout status`, `rollout history`, the pod template's image and the pods' `imageID`s. |
| `api-logs.txt` | Logs of the API pods (the API never logs credentials — `specs/auth/` BR5 — sanitized anyway). |
| `events.txt` | Namespace events, sorted by time. |
| `smoke.txt` | The smoke test output. |

## 8. Smoke test and controlled failure

`scripts/deploy-smoke.sh` (POSIX `sh` + `curl`, like `scripts/smoke.sh`; never prints the
token or the password), against `SMOKE_BASE_URL` with `SMOKE_EMAIL`/`SMOKE_PASSWORD`:

| Step | Expectation | Proves |
| --- | --- | --- |
| `GET /health` (retries up to `SMOKE_WAIT_SECONDS`) | `200` | process and `Service` up |
| `GET /api/v1/auth/me` without a token | `401` | the auth middleware is in front of protected routes |
| `POST /api/v1/auth/login` | `200` + `access_token` | `users` table exists; `/bootstrap-admin` created the user on the fresh database |
| `GET /api/v1/auth/me` with the token | `200`, `email` = `SMOKE_EMAIL` | JWT round trip, database read |
| `GET /api/v1/services` with the token | `200`, an `items` array | another schema table, a protected listing |

It is read-only, so it is equally safe against the demonstration environment.

**Controlled failure (BR13).** With `SIMULATE_MIGRATION_FAILURE=true` — accepted only when
`DEPLOY_EPHEMERAL=true` — `config` adds `99999999_fp36_simulated_failure.up.sql` to the
migrations `ConfigMap`:

```sql
DO $$ BEGIN RAISE EXCEPTION 'FP-36 simulated migration failure (workflow_dispatch input)'; END $$;
```

The real migrations apply, this one fails, the database is left dirty at that version (FP-33
§6 — irrelevant, the environment is about to be destroyed), the `Job` (`backoffLimit: 0`)
becomes `Failed`, `migrate` exits non-zero, `bootstrap`/`rollout`/`smoke` do not run, and
`evidence` + `destroy` do. `cluster.txt` shows no `Deployment`; the run ends failed. The file
exists only inside that run's `ConfigMap` — never in `migrations/`.

## 9. Contract with FP-34 (`/k8s`) and FP-35 (`/infra`)

What this pipeline expects from the two features it consumes. FP-33's own contract
(`specs/database-migrations/design.md` §9) still holds and is restated where it applies;
where `specs/kubernetes-deployment/` or `specs/infrastructure-provisioning/` say otherwise,
this table and FP-33's are the integration agreement (both specs are left to their owner to
update, as FP-33 did).

| # | Item | Expected |
| --- | --- | --- |
| 1 | Terraform root | `infra/`, local backend (state stays where `terraform` runs), runs with no prompt given `TF_VAR_postgres_password`. |
| 2 | Cluster owner | `infra/` creates **and** destroys the `kind` cluster; the pipeline creates no cluster of its own (no `helm/kind-action`). |
| 3 | Created by Terraform | Namespace `automotive-workshop`; `metrics-server`; Postgres `StatefulSet` `postgres` (pod `postgres-0`, image with `pg_isready`) + `Service` `postgres`, port 5432. |
| 4 | Variable | `postgres_password` (string, `sensitive = true`, no default). |
| 5 | Outputs | `kubeconfig_path` (`kind_cluster.<name>.kubeconfig_path`); `postgres_database_url_template` (contains the literal `<password>`; host reachable from inside the cluster). No credential in a non-`sensitive` output. |
| 6 | Manifests applied by the pipeline | `k8s/deployment.yaml`, `k8s/service.yaml`, `k8s/hpa.yaml`, optional `k8s/configmap.yaml` (rollout); `k8s/migration-job.yaml`; `k8s/bootstrap-admin-job.yaml`. Every object in namespace `automotive-workshop`. Not applied by the pipeline: `k8s/namespace.yaml` (Terraform owns it), `k8s/secret.yaml` (template only), any committed migrations `ConfigMap`. |
| 7 | API image | Lowercase name `ghcr.io/os-fiapudos/automotive-workshop-api`, any tag or none — the pipeline sets tag + digest (§5). Pods use the namespace's `default` `ServiceAccount` (or one that lists `imagePullSecrets: [ghcr-pull]`). |
| 8 | `Deployment` / `Service` | Both named `automotive-workshop-api`; `Service` port 8080; config via `envFrom` the `Secret` `automotive-workshop-api-secrets`. |
| 9 | API `Secret` | `automotive-workshop-api-secrets`, keys `DATABASE_URL`, `JWT_SECRET`, `EXTERNAL_NOTIFICATION_SECRET`, `EXTERNAL_STATUS_UPDATE_SECRET`, optional `RESEND_API_KEY`/`RESEND_FROM_EMAIL` — created by the pipeline. |
| 10 | Migration `Job` | Base name `automotive-workshop-api-migrate` (the pipeline appends `-<DEPLOY_ID>`); `migrate/migrate:v4.20.1`; args `["-path=/migrations", "-database", "$(DATABASE_URL)", "up"]`; `DATABASE_URL` from the API `Secret`; `ConfigMap` `automotive-workshop-api-migrations` mounted at `/migrations` (created by the pipeline from `migrations/`); `backoffLimit: 0`; `restartPolicy: Never`. |
| 11 | Bootstrap `Job` | Base name `automotive-workshop-api-bootstrap-admin`; the API image; `command: ["/bootstrap-admin"]`; `DATABASE_URL` from the API `Secret`; `ADMIN_EMAIL`/`ADMIN_PASSWORD`/optional `ADMIN_NAME` from the `Secret` `automotive-workshop-admin-bootstrap` (created by the pipeline); `restartPolicy: Never`. |
| 12 | Pull credential | `kubernetes.io/dockerconfigjson` `Secret` `ghcr-pull`, created by the pipeline when it has registry credentials. |
| 13 | Owned by the pipeline | The two `Secret`s of items 9 and 11, `ghcr-pull`, the `ConfigMap`s `automotive-workshop-api-migrations` and `automotive-workshop-deploy`, and the `Job`s. `/k8s` and `/infra` must not create objects with these names (`k8s/secret.yaml` stays a template the pipeline never applies), and the pipeline never modifies an object Terraform owns. |

The pipeline's `check` stage verifies items 1, 5 (at `infra` time), 6, 7 and the
`Job`-name lengths, and fails with a message naming FP-34/FP-35 when a piece is missing, so
until both land on `develop` a deploy run fails fast at its first step instead of halfway
through.

## 10. Demonstration environment and cloud variant

On a developer machine with Docker, Terraform, `kubectl` and `jq`:

```bash
export IMAGE=ghcr.io/os-fiapudos/automotive-workshop-api:<sha>   # from a successful run
export POSTGRES_PASSWORD=... JWT_SECRET=... EXTERNAL_NOTIFICATION_SECRET=... \
       EXTERNAL_STATUS_UPDATE_SECRET=... ADMIN_EMAIL=... ADMIN_PASSWORD=...
make deploy            # scripts/deploy.sh up — stays up
make deploy-destroy    # scripts/deploy.sh destroy — only when asked
```

The same values must be reused on later runs against the same environment (Postgres keeps
the password it was initialized with). The video uses this environment for API consumption
and the HPA demonstration; the GitHub run is the evidence of the pipeline itself.

If the cluster moves to a cloud provider, FP-35's cluster resource changes, the CI `deploy`
job drops its final `destroy` (a persistent cluster), and the job authenticates to that
cluster; every stage from `config` on is unchanged because it depends only on a kubeconfig.

## 11. Pinned versions

| Tool | Version | Why |
| --- | --- | --- |
| Terraform | `1.16.6` | Latest 1.16 patch on 2026-10-21 (1.17.0 was released that day — not adopted untested). |
| `kubectl` | `v1.36.5` | Within ±1 minor of the `tehcyx/kind` provider's default node (Kubernetes 1.35) and of newer `kindest/node` images (1.37). Its embedded kustomize (v5.8.1) renders tag + digest (§5). |
| Actions | `actions/checkout@v5`, `actions/upload-artifact@v4` (same as the existing jobs); `docker/setup-buildx-action@v4`, `docker/login-action@v4`, `docker/build-push-action@v7`, `hashicorp/setup-terraform@v4`, `azure/setup-kubectl@v5` | Current major versions on 2026-10-21. |

Raising any of them is a reviewed change, like the image pins in `CLAUDE.md` §2.

## 12. Testing strategy

- **Static**: `actionlint` on the workflow; `bash -n` and `shellcheck` on the scripts.
- **Local, end to end, before FP-34/FP-35 land**: the stages run against throwaway
  stand-ins of `/infra` and `/k8s` written to the contract of §9 (outside the repository,
  never committed) on a real `kind` cluster: happy path (`up`), controlled failure (no
  `Deployment` created), evidence re-scan for credential values, `destroy` leaving no `kind`
  container.
- **`scripts/deploy-smoke.sh`** against the Compose stack (seeded admin) and the stand-in
  cluster, including a wrong password (must fail).
- **GitHub, after FP-34/FP-35 land on `develop`** (AC10): a pull request run (no
  `publish`/`deploy`); a manual run (success); a manual run with
  `simulate_migration_failure` (failure, no rollout). Their URLs, the SHA/image and the
  measured duration are recorded in `tasks.md` §6 and on the Jira story.

## 13. Traceability

| Requirement | Design section |
| --- | --- |
| BR1 (checks preserved, real names, failure gate) | §1 |
| BR2 (PR safety) | §1, §2 |
| BR3 (lowercase name, SHA + digest, pull credential) | §2, §5 |
| BR4 (main + manual) | §1 |
| BR5 (Terraform, single owner, Postgres ready) | §3, §4, §9 |
| BR6 (config, secrets, migration Job → Complete) | §4, §6.2, §9 |
| BR7 (rollout only after migration) | §3, §4 |
| BR8 (authenticated smoke) | §8 |
| BR9 (credentials and state out of logs) | §6 |
| BR10 (least privilege, concurrency) | §1 |
| BR11 (identified, evidence, destroy) | §3, §7 |
| BR12 (demonstration environment) | §10 |
| BR13 (controlled failure) | §8 |
| AC1–AC10 | §1–§8, §12 |
