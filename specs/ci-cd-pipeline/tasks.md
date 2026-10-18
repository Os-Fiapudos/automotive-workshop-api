# Tasks — CI/CD Pipeline (Fase 2)

## 1. Spec confirmation

- [ ] Confirm GitHub Actions minutes/license availability (`requirements.md` §7) covers the
      additional deploy job's expected runtime.

## 2. Prerequisite features

- [ ] Confirm `specs/containerization/`, `specs/database-migrations/`,
      `specs/kubernetes-deployment/`, and `specs/infrastructure-provisioning/` are
      implemented (or implement this feature's workflow against their designs in parallel,
      wiring the actual paths/commands once each lands).

## 3. GitHub Actions secrets

- [ ] Add repository (or environment) secrets: `POSTGRES_PASSWORD`, `JWT_SECRET`,
      `EXTERNAL_NOTIFICATION_SECRET`, `EXTERNAL_STATUS_UPDATE_SECRET`, `RESEND_API_KEY`.

## 4. Workflow (`design.md` §1–§5)

- [ ] `build-and-push-image` job: checkout, GHCR login via `GITHUB_TOKEN`, build+push
      tagged with `${{ github.sha }}` and `latest`.
- [ ] `deploy` job: `kind` cluster provisioning, `terraform apply` (metrics-server,
      namespace, Postgres), migration run, API `Secret` creation, `k8s/deployment.yaml`
      image-tag substitution + `kubectl apply`, rollout wait, smoke test against `/health`.
- [ ] `terraform destroy` step with `if: always()`.
- [ ] Trigger: `push` to `main` + `workflow_dispatch`.

## 5. Verification

- [ ] Trigger a manual `workflow_dispatch` run end to end — confirm every step succeeds and
      the cluster is destroyed afterward (check the Actions run log, not just exit code).
- [ ] Intentionally fail the smoke-test step once (e.g. point it at a wrong port) — confirm
      `terraform destroy` still runs (`if: always()` proof).
- [ ] Confirm no secret value appears in any step's log output.
- [ ] Re-check `requirements.md` AC1–AC6.

## 6. Documentation

- [ ] README "CI/CD" section describing the pipeline stages and how to trigger a manual run
      for a video recording (`specs/phase-2-deliverables/`, referenced).
