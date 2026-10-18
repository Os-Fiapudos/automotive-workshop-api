# Design — CI/CD Pipeline (Fase 2)

Satisfies: `requirements.md` (all sections).

## 1. Workflow structure

New jobs appended to `.github/workflows/ci.yml` (or a new `.github/workflows/deploy.yml`
that `needs:`/`workflow_run`-triggers off the existing `ci.yml` — decided during
implementation based on which keeps the YAML more readable; functionally equivalent):

```yaml
on:
  push:
    branches: [main]
  workflow_dispatch: {}

jobs:
  build-test-security:   # existing jobs, unchanged
    ...

  build-and-push-image:
    needs: [build-test-security]
    runs-on: ubuntu-latest
    permissions:
      contents: read
      packages: write
    steps:
      - uses: actions/checkout@v4
      - uses: docker/login-action@v3
        with:
          registry: ghcr.io
          username: ${{ github.actor }}
          password: ${{ secrets.GITHUB_TOKEN }}
      - uses: docker/build-push-action@v6
        with:
          context: .
          push: true
          tags: |
            ghcr.io/${{ github.repository_owner }}/automotive-workshop-api:${{ github.sha }}
            ghcr.io/${{ github.repository_owner }}/automotive-workshop-api:latest

  deploy:
    needs: [build-and-push-image]
    if: github.ref == 'refs/heads/main' || github.event_name == 'workflow_dispatch'
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: hashicorp/setup-terraform@v3
      - uses: helm/kind-action@v1          # provisions kind + kubectl context for this job
        with:
          cluster_name: automotive-workshop
      - name: Terraform apply (metrics-server, namespace, Postgres)
        working-directory: infra
        env:
          TF_VAR_postgres_password: ${{ secrets.POSTGRES_PASSWORD }}
        run: terraform init && terraform apply -auto-approve
      - name: Run migrations
        run: |
          docker run --rm --network host -v $PWD/migrations:/migrations migrate/migrate:v4.18.1 \
            -path=/migrations -database "$DATABASE_URL" up
        env:
          DATABASE_URL: ${{ steps.tf-output.outputs.database_url }}
      - name: Create API Secret
        run: |
          kubectl create secret generic automotive-workshop-api-secrets \
            -n automotive-workshop \
            --from-literal=DATABASE_URL="$DATABASE_URL" \
            --from-literal=JWT_SECRET="${{ secrets.JWT_SECRET }}" \
            --from-literal=EXTERNAL_NOTIFICATION_SECRET="${{ secrets.EXTERNAL_NOTIFICATION_SECRET }}" \
            --from-literal=EXTERNAL_STATUS_UPDATE_SECRET="${{ secrets.EXTERNAL_STATUS_UPDATE_SECRET }}" \
            --from-literal=RESEND_API_KEY="${{ secrets.RESEND_API_KEY }}"
      - name: Deploy API manifests
        run: |
          sed "s|<tag>|${{ github.sha }}|" k8s/deployment.yaml | kubectl apply -f -
          kubectl apply -f k8s/service.yaml -f k8s/hpa.yaml
          kubectl rollout status deployment/automotive-workshop-api -n automotive-workshop --timeout=120s
      - name: Smoke test
        run: |
          kubectl port-forward -n automotive-workshop svc/automotive-workshop-api 8080:8080 &
          sleep 3
          curl --fail http://localhost:8080/health
      - name: Terraform destroy
        if: always()
        working-directory: infra
        env:
          TF_VAR_postgres_password: ${{ secrets.POSTGRES_PASSWORD }}
        run: terraform destroy -auto-approve
```

(Illustrative — exact action versions/pin shas confirmed and pinned at implementation time,
matching `CLAUDE.md` §7's "pin exact versions" posture already applied to `gosec`/
`govulncheck`.)

## 2. Image tagging and manifest substitution

`k8s/deployment.yaml` (`specs/kubernetes-deployment/`) ships with a `<tag>` placeholder in
its `image:` field specifically so this pipeline's `sed` step (or `kustomize edit set
image`, if the project later adopts `kustomize` — not required for this MVP) can substitute
the just-built commit SHA without hand-editing the manifest on every deploy. This keeps the
committed manifest buildable/readable on its own (`<tag>` is an obviously-a-placeholder
value) while the pipeline is the one place that knows the real tag.

## 3. Secrets required (GitHub Actions repository/environment secrets)

| Secret | Used for |
| --- | --- |
| `POSTGRES_PASSWORD` | Terraform's `postgres_password` variable |
| `JWT_SECRET` | API `Secret` |
| `EXTERNAL_NOTIFICATION_SECRET` | API `Secret` (`specs/service-order-quote-decision/`) |
| `EXTERNAL_STATUS_UPDATE_SECRET` | API `Secret` (`specs/service-order-status-update/`) |
| `RESEND_API_KEY` | API `Secret` (quote-sent e-mail notification) |

`secrets.GITHUB_TOKEN` (built-in, no setup needed) authenticates the GHCR push — satisfies
`requirements.md` BR2 without a new PAT to manage/rotate.

## 4. Teardown guarantee

The `Terraform destroy` step's `if: always()` (§1) satisfies `requirements.md` BR5/AC3 — it
runs even if an earlier step (migration, deploy, smoke test) failed, so a failed pipeline
run never leaks a `kind` cluster or its Docker resources on the runner (moot for a
GitHub-hosted runner, which is itself destroyed after the job either way, but kept explicit
so the same workflow is safe to run on a self-hosted runner too, if that's ever needed).

## 5. Cloud-cluster variant (if `requirements.md` §7 resolves to cloud later)

Only the `deploy` job's cluster-provisioning step changes: `helm/kind-action` is replaced by
whatever authenticates `kubectl`/`terraform` against the real, persistent cloud cluster
(e.g. a cloud provider's own GitHub Action, or `terraform apply` targeting that provider's
cluster resource directly — `specs/infrastructure-provisioning/design.md` §6's `cluster.tf`
swap), and the final `terraform destroy` step is removed (a persistent cluster is not torn
down after every deploy) — everything from "Run migrations" onward is unchanged, since it
already only depends on having a working `kubectl` context, not on *how* that context was
obtained.

## 5a. Seed data (not run by this pipeline)

The ephemeral cluster's Postgres receives only the schema (via migrations, §1) — `docs/
seed.sql` is **not** run by this pipeline. The smoke test (§1, AC4) only needs `/health`,
which requires no data. The video's API-consumption demo
(`specs/phase-2-deliverables/design.md` §4, beat 3) creates its own customer/vehicle/catalog
data live through the API rather than depending on seed fixtures, since this pipeline's
cluster is destroyed and recreated on every run — seeding it would just be extra setup time
for data the demo doesn't actually need pre-populated. If a future need arises for seeded
demo data in CI specifically, that is an explicit addition to this design, not assumed here.

## 6. Testing strategy

Operational: a manual `workflow_dispatch` run, watched end to end, is the acceptance test
for this feature — there is no unit-testable Go code here. `requirements.md` AC1–AC6 are
each verified directly against a real workflow run's logs/outcome, and this run **is** the
video's "Execução do CI/CD" + "Deploy da aplicação" footage
(`specs/phase-2-deliverables/`).

## 7. Traceability

| Requirement | Design section |
| --- | --- |
| BR1 (existing jobs unchanged) | §1 |
| BR2 (image build + GHCR push) | §1, §3 |
| BR3 (deploy job: terraform, migrate, kubectl apply) | §1, §2 |
| BR4 (smoke test) | §1 |
| BR5 (teardown always) | §1, §4 |
| BR6 (secrets from GitHub Actions secrets) | §3 |
| BR7 (deploy only on main/manual) | §1 |
| AC1–AC6 | §1, §6 |
