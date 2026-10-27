# Tasks — CI/CD Pipeline (Fase 2)

Revised 2026-10-21 with `requirements.md` §0 (FP-36).

## 1. Spec confirmation

- [x] GitHub Actions availability: repository is public, GitHub-hosted runners are free
      (`requirements.md` §7).
- [x] Spec revised to the FP-36 story and approved: deploy on `main` + manual; credentials
      generated per run; demonstration environment on a developer machine with the same
      script; implemented against the FP-34/FP-35 contract while those features are pending
      (`design.md` §9).

## 2. Prerequisite features

- [x] FP-32 image (`specs/containerization/`) and FP-33 migrations + `/bootstrap-admin`
      (`specs/database-migrations/`) on `develop`.
- [ ] FP-35 `/infra` and FP-34 `/k8s` on `develop`, matching `design.md` §9 (owner: the
      teammate; the `check` stage reports what is missing).

## 3. Workflow (`design.md` §1–§3)

- [x] `workflow_dispatch` trigger with `simulate_migration_failure`; workflow-wide
      `permissions: contents: read`.
- [x] `image` job: build without push on every event, `gha` cache.
- [x] `publish` job: `needs` the four checks + `image`, deploy events only,
      `packages: write`, lowercase name, SHA tag, full reference as output.
- [x] `deploy` job: deploy events only, `environment: ci-ephemeral`, serialized
      concurrency, pinned Terraform/`kubectl`, per-run credentials, one step per stage,
      evidence + upload + destroy on `always()`.

## 4. Scripts (`design.md` §4–§8)

- [x] `scripts/deploy.sh` with the stages `check`, `infra`, `config`, `migrate`,
      `bootstrap`, `rollout`, `smoke`, `evidence`, `destroy`, `up`.
- [x] `scripts/deploy-smoke.sh`: health, `401` without token, login, `/auth/me`,
      `/services`.
- [x] `.gitignore`: `deploy-evidence/` and the state/kubeconfig `terraform apply` leaves in
      `infra/`. `Makefile`: `deploy`, `deploy-destroy`.

## 5. Verification

- [x] `actionlint` clean; `bash -n`/`shellcheck` clean; `go build/vet/test` still pass
      (2026-10-21).
- [x] Local end to end against contract stand-ins (`design.md` §12), 2026-10-21, Terraform
      1.16.6 + `tehcyx/kind` 0.11.0 (Kubernetes 1.35) + `kubectl` 1.36.5: CI-style happy path
      (migration `Job` `Complete`, admin bootstrapped on the fresh database, rollout with
      `name:<sha>@<digest>`, authenticated smoke); `up` again on the same cluster as the
      demonstration environment (migration `no change`, bootstrap left the admin unchanged,
      new rollout, smoke); controlled failure (`99999999` `Failed`, database dirty at that
      version, **no `Deployment`**, evidence collected, destroy); evidence re-scanned for
      every generated value — none found; planted values and `postgres://` passwords
      redacted; a pod stuck on an image pull fails the `Job` wait after ~60 s with the
      reason; `destroy` refuses outside CI without `DEPLOY_CONFIRM_DESTROY=yes`, prints no
      attribute, leaves no `kind` container, and is a no-op without state; `check` names the
      missing FP-34/FP-35 files and refuses uppercase, `latest`, placeholder images and
      over-long `Job` names.
- [x] `scripts/deploy-smoke.sh` against an isolated API (migrations + seed admin): passes;
      fails on a wrong password (containing a quote and a backslash), a missing variable and an
      unreachable URL.
- [x] Local end to end against the real FP-35 `/infra` and FP-34 `/k8s` (not stand-ins),
      2026-10-27, Terraform 1.16.6 + `tehcyx/kind` 0.13.0 (Kubernetes 1.37) + `kubectl`
      1.36.5, image built from `4c731a0` and loaded into the node (no GHCR image yet):
      `check` → `infra` (clean apply, one state) → `config` → `migrate` (`Complete`) →
      `bootstrap` → `rollout` → `smoke` → `evidence` all ok; second `plan` empty; controlled
      failure on a fresh ephemeral environment (`Failed`, rollout blocked, no `Deployment`,
      evidence, destroy with no container left); evidence scanned for every generated value —
      none found.
- [ ] Pull request run: `publish`/`deploy` skipped. *(after the PR is opened)*

## 6. Evidence on GitHub (after FP-34/FP-35 land — AC10)

- [ ] Manual run, success: URL, SHA, image reference, migration `Job` `Complete`, rollout,
      authenticated smoke, duration.
- [ ] Manual run with `simulate_migration_failure`: URL, migration `Job` `Failed`, no
      `Deployment`, environment destroyed.
- [ ] Push to `main` (when `develop` is merged): automatic deploy run.

## 7. Documentation

- [x] README: "CI/CD" section — stages, triggers, failure rule, manual run, controlled
      failure, evidence, demonstration environment.
- [x] `CLAUDE.md` (§2 CI, §3 layout, §7 checks) and `specs/architecture.md` (CI component).
