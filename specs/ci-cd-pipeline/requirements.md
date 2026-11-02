# Requirements — CI/CD Pipeline (Fase 2)

Status: **Approved for implementation** (2026-10-21 — revised to the FP-36 story, see §0)
Jira: **FP-36** — CI/CD pipeline with validated delivery and Kubernetes deploy (planning id
F2-US10, manual requirement R11). Depends on **FP-29** (Clean Architecture refactor — the
pipeline must keep its tests green), **FP-33** (`specs/database-migrations/`, migration chain,
`/bootstrap-admin`), **FP-34** (`specs/kubernetes-deployment/`, `/k8s`) and **FP-35**
(`specs/infrastructure-provisioning/`, `/infra`). Builds on **FP-32**
(`specs/containerization/`, the image).
Touches: `.github/workflows/ci.yml`, new `scripts/deploy.sh` and `scripts/deploy-smoke.sh`,
`Makefile`, `.gitignore`, and documentation (`README.md`, `CLAUDE.md`,
`specs/architecture.md`).

## 0. Revision note (2026-10-21)

The first version of this spec (PR #30) predates the FP-36 Jira story, FP-33's migration
contract (`specs/database-migrations/design.md` §9) and FP-32's image. The story's acceptance
checklist contradicts it in several places, so this revision changes the original as follows
(each is called out where it applies):

1. **One owner for the cluster.** The original created the cluster with `helm/kind-action`
   *and* ran `terraform apply` against a configuration that also creates a `kind` cluster
   with the same name — two owners, and the second creation fails. Terraform (FP-35) is now
   the only thing that creates and destroys the cluster (BR5).
2. **Migrations run inside the cluster.** The original ran `migrate` with
   `docker run --network host` from the runner, which cannot reach a Postgres that only has a
   cluster-internal address, and read a `steps.tf-output` step that did not exist. The
   pipeline now runs FP-34's migration `Job` and waits for it to finish (BR6), as FP-33's
   contract already says.
3. **Gate before rollout.** The original applied the `Deployment` regardless of the
   migration outcome. A failed database or migration now stops the run before any
   application manifest is applied (BR7), and a run that proves it is part of the evidence
   (BR13).
4. **Smoke test that proves schema and authorization.** `GET /health` does not touch the
   database or the auth middleware. The smoke test now also logs in with the administrator
   created by `/bootstrap-admin` on the freshly created database and calls protected,
   database-backed routes (BR8).
5. **Immutable, complete image reference.** No `<org>`/`<tag>` placeholders reach the
   cluster; the image is published under a lowercase name and deployed by commit SHA tag plus
   digest (BR3). `latest` is no longer what anything deploys.
6. **Real job names, least privilege, PR safety, concurrency** (new — BR1, BR2, BR10).
7. **Ephemeral vs. demonstration environment** (new — BR11, BR12). The pipeline's cluster
   lives inside a GitHub-hosted runner and is destroyed with it; the environment the video
   shows is created on a developer machine with the same deploy script.
8. **Credentials generated per run** (changes the original BR6). The ephemeral environment
   needs no GitHub Actions secrets: every credential is generated at the start of the run,
   masked, and dies with the cluster (BR9). This also removes the dependency on an
   organization admin to register secrets.
9. **The open question is closed** (§7): the repository is public, so GitHub-hosted runner
   minutes are not a constraint.

## 1. Context

Source: `SOAT - Fase 2 - Tech challenge.pdf` p.4, "Pipeline de CI/CD configurada..., que
execute: Build da aplicação; Execução dos testes automatizados; Build da imagem Docker;
Deploy no cluster Kubernetes; Deploy do banco de dados; Aplicação dos manifestos YAML no
cluster." p.5–6 asks the video to show the CI/CD execution and the deploy.

State when this feature starts: `.github/workflows/ci.yml` runs four jobs on every push and
pull request — `build` (build/vet/test), `coverage` (RNF06 against a real Postgres built by
the migrations + seed), `migrations` (`scripts/migrate.sh verify`) and `security`
(`govulncheck` + `gosec`). Nothing builds or publishes the image, and nothing deploys.

**Where the pipeline deploys.** The team's cluster decision is a local `kind` cluster
(`specs/infrastructure-provisioning/` §7). A GitHub-hosted runner cannot reach a cluster on a
developer's machine, so each deploy run creates its own short-lived cluster *inside the
runner*, with the same Terraform configuration a developer uses locally: the run is a real
deploy — provision, database, migrations, rollout, authenticated smoke test — against a
cluster that lives as long as the job. It is an **ephemeral validation environment**, not a
place anyone can consume the API from after the run.

## 2. User story

> As the development team, I want every version to be delivered by a validated pipeline, so
> that manual intervention is reduced and the deploy is reproducible. *(FP-36)*

> As whoever records the Fase 2 video, I want a pipeline run that shows build, tests, image,
> infrastructure, database, migrations, rollout and an authenticated smoke test — and a run
> that shows a failed migration stopping the rollout — plus a demonstration environment
> created by the same steps, so that "Execução do CI/CD" and "Deploy da aplicação" are real.

## 3. Business rules

1. **Existing checks preserved and required.** `build` (build, vet, test), `coverage`
   (Postgres-backed, RNF06), `migrations` and `security` keep running unchanged on every push
   and pull request. Publishing the image and deploying depend on all four succeeding, named
   by their real job ids in `needs:`. Any failure — or a skipped/cancelled required job —
   prevents publication and deploy.
2. **Pull requests never deploy and never hold privileged credentials.** A pull request runs
   the checks and an image build without push. No job reachable from a pull request has
   `packages: write`, uses an Actions secret, or creates infrastructure.
3. **Image.** Built from the FP-32 `Dockerfile`, pushed to GHCR under a **lowercase** name
   (`ghcr.io/os-fiapudos/automotive-workshop-api` — the organization is `Os-Fiapudos`),
   tagged with the full commit SHA. What is deployed is the complete, immutable reference
   `name:<sha>@sha256:<digest>`; no manifest that reaches the cluster contains a placeholder
   (`<org>`, `<tag>`) or a mutable tag. When the package is private, the cluster gets a pull
   credential (an image pull `Secret`) before any pod using the image is created.
4. **When it deploys.** Automatically on a push to `main`; manually through
   `workflow_dispatch` (any branch, subject to whatever protection the GitHub environment
   `ci-ephemeral` is configured with). Never on `pull_request`, never on pushes to other
   branches.
5. **Infrastructure and database by Terraform, single owner.** The pipeline runs
   `terraform apply` on FP-35's `/infra` configuration, which is the only thing that creates
   (and later destroys) the cluster and the Postgres instance. It then waits until Postgres
   accepts connections — not merely until its pod exists.
6. **Configuration, secrets and migrations in the cluster.** After the database is ready the
   pipeline creates/updates the API `Secret`, the bootstrap `Secret`, the migrations
   `ConfigMap` (generated from this commit's `migrations/`) and, when needed, the pull
   credential; then runs FP-34's migration `Job` under a name unique to the run and waits for
   it to reach `Complete`. `Failed`, or no outcome within the timeout, fails the run.
7. **Rollout only after a successful migration.** The administrator bootstrap `Job` (FP-33
   BR10) and the application manifests (`Deployment`, `Service`, `HPA`, and the optional
   `ConfigMap`) are applied only after the migration `Job` is `Complete`. The pipeline then
   waits for the `Deployment` rollout and checks that it runs the published reference. A
   failure in infrastructure, database readiness or migration means no application manifest
   is applied.
8. **Authenticated smoke test.** Against the deployed `Service`, the smoke test checks:
   `GET /health` → `200`; a protected route without a token → `401`; `POST /api/v1/auth/login`
   with the administrator created by `/bootstrap-admin` on that fresh database → `200` and a
   token; `GET /api/v1/auth/me` with the token → `200` for that administrator; and a protected,
   database-backed listing (`GET /api/v1/services`) → `200`. The run fails on any mismatch.
9. **No credential or state in logs or artifacts.** The ephemeral environment's credentials
   (Postgres password, `JWT_SECRET`, both `EXTERNAL_*` secrets, administrator password) are
   generated at the start of the run, masked in the log, and never stored anywhere that
   outlives the run; no repository secret is needed. The registry credential is the job's own
   `GITHUB_TOKEN` (read-only on packages). Terraform's state, plan diffs, kubeconfig and
   sensitive outputs are never printed or uploaded; evidence files are sanitized and checked
   before upload.
10. **Least privilege and controlled concurrency.** The workflow's default token permission
    is `contents: read`; only the publishing job gets `packages: write`, and the deploy job
    `packages: read`. Deploys are serialized (one at a time, a running deploy is never
    cancelled by a newer one).
11. **Ephemeral environment identified, evidenced, destroyed.** Every deploy run has an
    environment id (`ci-<run id>-<attempt>`) recorded on the cluster objects it creates and in
    the evidence. Evidence is collected **before** the environment is destroyed, and the
    destroy runs whatever the outcome. Nothing promises that this environment is reachable
    after the run.
12. **Demonstration environment.** The same deploy script runs on a developer machine
    against the cluster FP-35's Terraform creates there; that environment stays up for the
    video and API consumption, and is destroyed only on request. It deploys an image the
    pipeline already published and validated.
13. **Controlled failure.** A manual run can inject a migration that always fails (ephemeral
    environment only — the script refuses it elsewhere). That run must end failed, with the
    migration `Job` `Failed`, no `Deployment` created, and evidence collected and the
    environment destroyed as in a successful run.

## 4. Scope

In scope: the `image`, `publish` and `deploy` jobs in `.github/workflows/ci.yml` and the
manual trigger; `scripts/deploy.sh` (staged deploy used by CI and locally) and
`scripts/deploy-smoke.sh`; the contract with FP-34/FP-35 (`design.md` §9); evidence
collection; `Makefile` targets for the demonstration environment; documentation.

Out of scope: the existing `build`/`coverage`/`migrations`/`security` jobs (BR1, unchanged);
the Terraform configuration (`/infra`, FP-35); the Kubernetes manifests (`/k8s`, FP-34); the
migration tooling (FP-33); the image itself (FP-32); registering GitHub secrets (none are
needed, BR9); configuring protection rules on the `ci-ephemeral` environment (an
organization admin's choice, BR4).

## 5. Acceptance criteria

1. A pull request runs `build`, `coverage`, `migrations`, `security` and `image` (build
   without push); `publish` and `deploy` are skipped and no job in it has `packages: write`.
2. A push to `main` (or a manual run) publishes `ghcr.io/os-fiapudos/automotive-workshop-api:<sha>`
   only after the four required jobs succeed, and the deploy uses
   `…:<sha>@sha256:<digest>`.
3. A failing required job leaves `publish` and `deploy` skipped.
4. The deploy run creates the cluster and Postgres with Terraform only, waits for Postgres,
   creates the configuration, and the migration `Job` reaches `Complete` before any
   `Deployment` exists.
5. The administrator bootstrap succeeds on the freshly created database, the rollout
   completes with the published reference, and the authenticated smoke test (BR8) passes.
6. A manual run with the simulated migration failure ends failed, with the migration `Job`
   `Failed`, no `Deployment` in the namespace, evidence uploaded, and the environment
   destroyed.
7. Evidence of every deploy run (success or failure) is uploaded as an artifact and shown in
   the run summary: environment id, commit SHA, image reference and running image id, stage
   results, migration and bootstrap `Job` status and logs, rollout status, smoke output.
8. No credential value, kubeconfig, Terraform state or plan attribute appears in the job
   log or in the evidence artifact.
9. The same script brings up the demonstration environment on a developer machine and leaves
   it running; README documents it.
10. Recorded for FP-36: the URL of a successful run and of the controlled-failure run, with
    the deployed SHA/image, migration `Job` `Complete`, rollout and authenticated smoke.

## 6. Out of scope

- Blue/green or canary strategies, automatic rollback — a failed run fails, its environment
  is destroyed, and a human re-runs after the fix.
- Deploying from CI to a persistent cluster. If the cluster decision moves to a cloud
  provider, only FP-35's cluster and the absence of the final destroy change (`design.md`
  §10); every stage from "configuration" on is unchanged.
- Running `docs/seed.sql` in any deployed environment (FP-33 BR11): the administrator comes
  from `/bootstrap-admin`, and the smoke test needs no other data.

## 7. Open question — closed

The original spec waited on GitHub Actions minutes/licensing for the extra deploy job. The
repository is public (checked 2026-10-21), and GitHub-hosted runners are free for public
repositories, so the deploy job runs on `push` to `main` as designed. The run time is
recorded with the evidence (AC10).
