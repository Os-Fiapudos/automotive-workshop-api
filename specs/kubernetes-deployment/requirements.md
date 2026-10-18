# Requirements — Kubernetes Manifests (`/k8s`)

Status: **Proposed** (depends on `GAPS-FASE-2.md` §7 item 1, local-vs-cloud cluster, still
pending FIAP confirmation — see §7; this spec is written cluster-agnostic so it holds either
way)
New top-level folder: `/k8s`.

## 1. Context

Source: `SOAT - Fase 2 - Tech challenge.pdf` p.4, "Orquestração com Kubernetes (K8s): Criar
manifestos YAML para deploy em Kubernetes, contemplando: Deployments; Services; ConfigMaps e
Secrets; HPA." `GAPS-FASE-2.md` §3.2 confirms no `/k8s` folder exists today.

**Scope boundary with `specs/infrastructure-provisioning/`**: the PDF asks Terraform to
provision "o cluster Kubernetes (local ou cloud)" **and** "Banco de Dados" (p.4,
"Infraestrutura como Código"), separately from the "/k8s" manifests, which it lists as
`Deployments`/`Services`/`ConfigMaps e Secrets`/`HPA` only — no mention of a database
workload. This spec therefore covers **only the API application's own manifests**; the
Postgres `StatefulSet`/`PersistentVolumeClaim`/headless `Service` are provisioned by
Terraform (`specs/infrastructure-provisioning/`), not applied from `/k8s`. The database
migration `Job` (§6) is the one exception that *is* part of `/k8s`, since it is an
application-deploy concern (apply schema before the new API version serves traffic), not a
database-provisioning one.

## 2. User story

> As whoever deploys this API to Kubernetes (a developer locally against kind/minikube, or
> the CI/CD pipeline, `specs/ci-cd-pipeline/`),
> I want a complete set of Kubernetes manifests for the API,
> so that `kubectl apply -f k8s/` brings up a working, horizontally-scalable deployment of
> the already-containerized image (`specs/containerization/`) against the already-provisioned
> cluster and database (`specs/infrastructure-provisioning/`).

## 3. Business rules

1. The API runs as a `Deployment` (not a bare `Pod` or a `StatefulSet` — it is stateless;
   all state lives in Postgres), with `resources.requests`/`resources.limits` set on CPU and
   memory — required for the HPA (BR4) to have anything to scale against.
2. The API is reachable inside the cluster via a `ClusterIP` `Service` on port 8080 (the
   application's own listening port, unchanged).
3. Every environment variable the API already requires (`DATABASE_URL`, `JWT_SECRET`) and
   every one Fase 2 adds (`EXTERNAL_NOTIFICATION_SECRET`,
   `EXTERNAL_STATUS_UPDATE_SECRET`, `RESEND_API_KEY`) comes from a Kubernetes `Secret` — none
   is hardcoded in the `Deployment` manifest (`CLAUDE.md` §13). Any non-sensitive
   configuration (if any is ever needed beyond what's already env-driven) comes from a
   `ConfigMap`.
4. A `HorizontalPodAutoscaler` scales the `Deployment` by CPU utilization (the PDF's "HPA,
   escalando conforme consumo de CPU/memória" — CPU is the primary signal implemented;
   memory-based scaling is added too since both are named, §6 design), between a minimum and
   maximum replica count.
5. The `Deployment`'s pod template declares liveness and readiness probes against `/health`
   — the same endpoint `specs/containerization/`'s `HEALTHCHECK` already uses, reused here
   rather than inventing a second health signal.
6. `DATABASE_URL` in the `Secret` points at the Postgres instance
   `specs/infrastructure-provisioning/` provisions (in-cluster `StatefulSet` or managed,
   depending on that spec's own local-vs-cloud outcome) — this spec does not provision or
   assume a specific database location, only that the value is injected via `Secret`.
7. Schema migrations (`specs/database-migrations/`) run via a Kubernetes `Job`, applied
   before the `Deployment`'s new pods are expected to serve traffic against a changed
   schema — not inside the API's own container at startup (keeps the API's own `Deployment`
   simple and lets a migration failure block the rollout visibly, rather than crash-looping
   API pods).

## 4. Scope

In scope: `/k8s/deployment.yaml`, `/k8s/service.yaml`, `/k8s/configmap.yaml` (if needed),
`/k8s/secret.yaml` (a template/example — the real `Secret` is never committed with real
values, §6), `/k8s/hpa.yaml`, `/k8s/migration-job.yaml`, `/k8s/namespace.yaml` (optional, if
the project deploys into its own namespace rather than `default`).

Out of scope: the cluster itself, the metrics-server HPA depends on, and Postgres —
`specs/infrastructure-provisioning/`. The CI/CD steps that run `kubectl apply -f k8s/` —
`specs/ci-cd-pipeline/`. The container image these manifests reference —
`specs/containerization/`.

## 5. Acceptance criteria

1. `kubectl apply -f k8s/` against a cluster that already has the `Secret`'s real values and
   reaches a provisioned Postgres succeeds and results in the API reachable via its
   `Service`.
2. `kubectl get hpa` shows the `HorizontalPodAutoscaler` actively tracking the `Deployment`
   with real CPU/memory metrics (requires `metrics-server` installed in the cluster —
   `specs/infrastructure-provisioning/` §_ installs it, referenced not duplicated).
3. Killing one pod (`kubectl delete pod ...`) results in the `Deployment` replacing it
   automatically, with the replacement passing its readiness probe before receiving traffic.
4. A simulated load increase (the video's "escalabilidade automática" demonstration,
   `specs/phase-2-deliverables/`) results in `kubectl get hpa` showing replica count
   increase, and decrease again once load subsides.
5. The migration `Job` runs to completion before the `Deployment`'s rollout is considered
   done, in a manual `kubectl apply -f k8s/` walkthrough and in the CI/CD pipeline
   (`specs/ci-cd-pipeline/`, referenced).
6. No real secret value is committed to the repository — `/k8s/secret.yaml` is a template
   (placeholder values or a documented `kubectl create secret` command), consistent with
   `.env.example`'s existing convention (`CLAUDE.md` §13).

## 6. Out of scope

- `Ingress` / external DNS / TLS termination — not requested by the PDF's explicit list
  (`Deployments`, `Services`, `ConfigMaps e Secrets`, `HPA`); `kubectl port-forward` or a
  `NodePort`/cloud `LoadBalancer` `Service` variant is sufficient for the video demo
  (decided in `specs/phase-2-deliverables/`, not here).
- `NetworkPolicy`, `PodDisruptionBudget`, `PodSecurityPolicy`/`PodSecurityStandards` — good
  practice, not requested by the PDF; a future hardening step.
- Helm — the PDF asks for "manifestos YAML," plain `kubectl apply -f k8s/` manifests satisfy
  that without introducing a templating tool this project hasn't aligned on
  (`CLAUDE.md` §12's bar, applied by analogy even though Helm isn't a Go dependency).

## 7. Open question — confirm before implementation

Depends on `GAPS-FASE-2.md` §7 item 1 (local kind/minikube vs. cloud) being resolved — this
spec is written to work either way (every manifest is standard Kubernetes, no cloud-specific
API), but the `Service` type for the video demo (`NodePort` locally vs. `LoadBalancer` on a
cloud provider) and whether `metrics-server` needs a manual install (usually yes on
kind/minikube, often preinstalled on a managed cloud cluster) depend on that decision — see
`specs/infrastructure-provisioning/requirements.md` §7 for the same open item, tracked once
there rather than twice.
