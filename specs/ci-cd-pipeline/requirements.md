# Requirements — CI/CD Pipeline (Fase 2)

Status: **Proposed** (depends on GitHub Actions minutes/license availability for this
repository/org being confirmed — `GAPS-FASE-2.md` §7 item 10 — see §7)
Touches: `.github/workflows/ci.yml` (extended) or a new `.github/workflows/deploy.yml`.

## 1. Context

Source: `SOAT - Fase 2 - Tech challenge.pdf` p.4, "Pipeline de CI/CD configurada..., que
execute: Build da aplicação; Execução dos testes automatizados; Build da imagem Docker;
Deploy no cluster Kubernetes; Deploy do banco de dados; Aplicação dos manifestos YAML no
cluster." `.github/workflows/ci.yml` already does the first two (build, test, plus
coverage and security scanning — `CLAUDE.md` §7); this feature adds the four deploy-related
steps.

**The local-cluster problem** (`GAPS-FASE-2.md` §7 item 10): if
`specs/infrastructure-provisioning/` provisions a local `kind` cluster (the team's default,
pending FIAP cloud confirmation), a GitHub-hosted runner cannot reach a cluster running on
someone's laptop. Resolution adopted by this spec: **the pipeline provisions its own
short-lived `kind` cluster inside the runner for every pipeline execution**, reusing the
exact Terraform configuration from `specs/infrastructure-provisioning/` — the CI run itself
*is* a real, reproducible deploy (build → provision → deploy → smoke-test → migrate), not a
simulation, it just targets a cluster that lives only for the duration of the job (GitHub-
hosted runners are themselves ephemeral, so this matches the runner's own lifecycle rather
than fighting it). If `requirements.md` §7's cloud decision lands differently later, this
same pipeline's `terraform apply` step targets the real, persistent cloud cluster instead —
only the cluster-provisioning step's target changes, not the pipeline's shape.

## 2. User story

> As whoever needs to prove this project's deploy process is automated (the Fase 2
> grader, watching the required video demonstration),
> I want a single pipeline run to build the app, run its tests, build and push its image,
> provision/target a Kubernetes cluster, deploy the database and the API to it, and prove the
> API is reachable afterward,
> so that "Execução do CI/CD" in the video (p.6) is a real pipeline run, not a manual
> walkthrough dressed up as one.

## 3. Business rules

1. The pipeline runs the existing build/test/coverage/security jobs unchanged — this
   feature is additive to `.github/workflows/ci.yml`, not a replacement.
2. A Docker image build step produces the image `specs/containerization/`'s `Dockerfile`
   defines, tagged with the commit SHA (and `latest` on the default branch), and pushes it
   to GHCR (`ghcr.io/<org>/automotive-workshop-api`) using the workflow's built-in
   `GITHUB_TOKEN` (sufficient for same-repository GHCR push, no extra PAT needed).
3. A deploy job (depends on the build/test/image-push jobs succeeding) runs
   `terraform apply` (`specs/infrastructure-provisioning/`) to produce a cluster + Postgres,
   `migrate ... up` (`specs/database-migrations/`) against it, and
   `kubectl apply -f k8s/` (`specs/kubernetes-deployment/`) with the just-pushed image tag
   substituted into `deployment.yaml` — end to end, a single pipeline run exercises every
   step the PDF lists.
4. The deploy job ends with a smoke test — at minimum `GET /health` against the deployed
   Service (via `kubectl port-forward` or the cluster-internal address from within the
   runner) returning `200` — before the job is considered successful.
5. The deploy job tears down its ephemeral cluster (`terraform destroy`) on completion,
   success or failure, so a GitHub Actions runner never leaks a `kind` cluster/Docker
   resources into the next job.
6. Every secret the deploy job needs (`JWT_SECRET`, `EXTERNAL_NOTIFICATION_SECRET`,
   `EXTERNAL_STATUS_UPDATE_SECRET`, `RESEND_API_KEY`, `postgres_password`) comes from GitHub
   Actions repository/environment secrets — never hardcoded in the workflow file
   (`CLAUDE.md` §13's "secrets come from the environment" rule, applied to CI configuration
   too).
7. The deploy job only runs on pushes to `main` (or on a manual
   `workflow_dispatch`, for a deliberate on-demand demo run) — not on every PR, to avoid
   spinning up a cluster for every draft PR's CI run.

## 4. Scope

In scope: the new deploy job(s) in `.github/workflows/`, the GHCR image push step, the
substitution of the built image tag into the deploy manifests, the required GitHub Actions
secrets list.

Out of scope: the existing build/test/coverage/security jobs (`CLAUDE.md` §7, unchanged);
the Terraform configuration itself (`specs/infrastructure-provisioning/`); the Kubernetes
manifests themselves (`specs/kubernetes-deployment/`); the migration tooling itself
(`specs/database-migrations/`).

## 5. Acceptance criteria

1. A push to `main` triggers a workflow run that builds the app, runs the existing test/
   coverage/security jobs, builds and pushes a Docker image to GHCR tagged with the commit
   SHA.
2. The same run provisions a cluster, deploys Postgres, runs migrations, applies the `/k8s`
   manifests with the just-built image, and a smoke test against `/health` passes.
3. The ephemeral cluster is destroyed at the end of the run regardless of the job's outcome
   (`if: always()` on the teardown step).
4. No secret value appears in the workflow file or in job logs (GitHub Actions' own secret
   masking, relied upon, not reimplemented).
5. The whole pipeline (build → test → image → deploy → smoke test → teardown) completes
   within a reasonable CI time budget (documented, not a hard number the PDF specifies) —
   measured once implemented and recorded in `specs/phase-2-deliverables/`'s video script
   notes.
6. A manual `workflow_dispatch` run is possible for recording the video's CI/CD
   demonstration on demand, not only by pushing a throwaway commit.

## 6. Out of scope

- Blue/green or canary deployment strategies — a single `kubectl apply` rollout is
  sufficient for what the PDF asks to demonstrate.
- Deploying to a *persistent* cloud cluster from CI — only relevant if `requirements.md` §7
  resolves to cloud; this spec's pipeline shape accommodates it (swap the Terraform target,
  §1) but does not build it preemptively.
- Automatic rollback on a failed smoke test — the job simply fails and is destroyed (§3
  item 5); a human re-runs after fixing the issue, same as this project's existing CI
  failure handling.

## 7. Open question — confirm before implementation

**GitHub Actions availability/license for this repository's org** (`GAPS-FASE-2.md` §7 item
10) — the existing `ci.yml` already runs on GitHub Actions today (build/test/coverage/
security), so basic availability is proven; what needs confirming is whether the *additional*
minutes this deploy job consumes (image build + ephemeral cluster provisioning, likely
several minutes per run) fit within whatever plan/quota applies. If GitHub Actions turns out
to be unavailable or quota-constrained for this additional job, the fallback is to still
write and test this pipeline, but demonstrate the deploy job as a manual
`workflow_dispatch`/local run for the video rather than relying on automatic `push`-triggered
runs — a scheduling change, not a redesign.
