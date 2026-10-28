# Requirements — Kubernetes Manifests (`/k8s`)

Status: **Approved for implementation** (2026-10-25 — revised to the FP-34 story, see §0)
Jira: **FP-34** — "[Fase 2] Aplicação Kubernetes resiliente e escalável por métricas"
(planning id F2-US09, manual requirement R10). Depends on **FP-32**
(`specs/containerization/`, the image), **FP-33** (`specs/database-migrations/`, the
migration chain and `/bootstrap-admin`) and **FP-35** (`specs/infrastructure-provisioning/`,
the cluster, `metrics-server` and Postgres). Consumed by **FP-36**
(`specs/ci-cd-pipeline/`, `scripts/deploy.sh`).
New top-level folder: `/k8s`; new `scripts/k8s-evidence.sh`. Also touches `.gitignore`,
`Makefile`, `.github/workflows/iac.yml` (shared with FP-35) and documentation.

## 0. Revision note (2026-10-25)

The first version of this spec (PR #30) predates the FP-34 Jira story and the contracts
FP-33 (`specs/database-migrations/design.md` §9) and FP-36 (`specs/ci-cd-pipeline/design.md`
§9, PR #42) later wrote against `/k8s`. The story's checklist and those contracts change the
original in these places (each is called out where it applies):

1. **No wholesale `kubectl apply -f k8s/`.** The original user story and AC1 applied the
   whole folder at once. The story forbids it: the `Job`s must run in order, under a name
   unique to the run, and a template `Secret` must never be applied over real credentials.
   The deploy applies named files in a fixed order (BR8); the `Secret` template lives in a
   subfolder that a non-recursive `apply -f k8s/` does not read.
2. **No `Namespace` manifest.** Terraform (FP-35) is the namespace's only owner; the
   original's "whichever is applied first" no-op assumption is gone.
3. **The HPA owns the replica count** (BR4). The original set `replicas: 2` on the
   `Deployment` to "agree" with the HPA, but every re-apply would still reset a scaled-out
   `Deployment` back to 2. The field is omitted.
4. **Immutable image reference** (BR1). The committed manifests name the image without a
   tag (`ghcr.io/os-fiapudos/automotive-workshop-api`, lowercase); the deploy sets
   `:<sha>@sha256:<digest>`. The original's `ghcr.io/<org>/…:<tag>` placeholder is gone.
5. **Migration `Job` per FP-33** (BR7): `migrate/migrate:v4.20.1` (not `v4.18.1`),
   `backoffLimit: 0`, a unique name per run, and the migrations `ConfigMap` **generated at
   deploy time** from `migrations/` instead of a committed, regenerated-by-hand copy that
   could drift from the files.
6. **Administrator bootstrap `Job`** (BR7, new): FP-33's `/bootstrap-admin`, run after the
   migration, so a freshly provisioned database has a user to log in with.
7. **Evidence of resilience and scaling** (AC5–AC8, new): load that reaches the `Service`
   inside the cluster (not a single port-forwarded pod), replica counts before/during/after,
   and pod replacement — produced by `scripts/k8s-evidence.sh`.
8. **Hardening in the pod spec** (BR6, new): the image already runs as a numeric non-root
   user (FP-32); the manifests make it enforceable (`restricted` Pod Security, read-only
   root filesystem, no service-account token).

## 1. Context

Source: `SOAT - Fase 2 - Tech challenge.pdf` p.4, "Orquestração com Kubernetes (K8s): Criar
manifestos YAML para deploy em Kubernetes, contemplando: Deployments; Services; ConfigMaps e
Secrets (para variáveis sensíveis, como tokens de serviços externos); Horizontal Pod
Autoscaler (HPA), escalando conforme consumo de CPU/memória." p.6: "Manifestos Kubernetes
(em /k8s)"; the video must show "Escalabilidade automática (pode simular aumento de carga
ou múltiplas ordens de serviço)".

**Scope boundary with `specs/infrastructure-provisioning/`**: this spec covers the API
application's own manifests and its deploy-time `Job`s. The cluster, `metrics-server`, the
`Namespace` and Postgres are FP-35's (Terraform). The pipeline that renders and applies
these files is FP-36's.

## 2. User story

> As the operations team, I want to run the API with replicas and autoscaling, to absorb
> demand variations and recover from pod failures. *(FP-34)*

## 3. Business rules

1. **Deployment.** The API runs as a `Deployment` `automotive-workshop-api` (stateless; all
   state is in Postgres, outside the pod), referencing the image
   `ghcr.io/os-fiapudos/automotive-workshop-api` with no tag in Git — the deploy sets the
   immutable `:<sha>@sha256:<digest>` reference. CPU and memory `requests`/`limits` are set
   (the HPA computes utilization against the requests).
2. **Service.** A `ClusterIP` `Service` `automotive-workshop-api` on port 8080 distributes
   traffic across ready pods inside the cluster.
3. **Configuration.** Non-sensitive configuration (`PORT`, `JWT_TTL`) comes from a
   `ConfigMap`; every credential (`DATABASE_URL`, `JWT_SECRET`, both `EXTERNAL_*` secrets,
   the optional `RESEND_API_KEY`/`RESEND_FROM_EMAIL` pair) from the `Secret`
   `automotive-workshop-api-secrets`, which is created by the deploy and never committed
   with values. The repository holds only a template with `REPLACE_ME` placeholders.
4. **Autoscaling.** An `autoscaling/v2` `HorizontalPodAutoscaler` scales the `Deployment`
   between 2 and 8 replicas on CPU (70 %) and memory (80 %) utilization, with documented
   scale-up/scale-down behavior. The HPA is the only owner of the replica count: re-applying
   the manifests never resets it.
5. **Health.** Startup, readiness and liveness probes use `/health` (the endpoint
   FP-32's `HEALTHCHECK` already uses). A pod receives traffic only once ready, and a
   terminating pod stops receiving new traffic before the process is stopped.
6. **Pod hardening.** Pods meet the `restricted` Pod Security profile (FP-35 labels the
   namespace to enforce it): non-root numeric user, no privilege escalation, all
   capabilities dropped, `RuntimeDefault` seccomp, read-only root filesystem, no
   service-account token mounted.
7. **Deploy-time `Job`s.** A migration `Job` (FP-33: `migrate/migrate:v4.20.1`, migrations
   from a `ConfigMap` generated from `migrations/` at deploy time, `backoffLimit: 0`) and an
   administrator bootstrap `Job` (FP-33 BR10: the API image, `/bootstrap-admin`, credentials
   from the `Secret` `automotive-workshop-admin-bootstrap`). Each run creates them under a
   new, unique name.
8. **Order.** Infrastructure ready → `Secret`s and migrations `ConfigMap` → migration `Job`
   `Complete` → bootstrap `Job` `Complete` → `ConfigMap`/`Deployment`/`Service`/`HPA` →
   rollout. A failed or timed-out migration stops the deploy before any application object
   is applied. The folder is never applied wholesale.
9. **Contract names.** Object names, `Secret`/`ConfigMap` keys, base `Job` names and the
   image name are the ones in FP-36's contract (`specs/ci-cd-pipeline/design.md` §9 items
   6–13); `/k8s` never creates an object that contract assigns to the pipeline.
10. **Evidence tooling.** A script reproduces the scaling and resilience evidence: load from
    inside the cluster through the `Service`, replica/HPA/metrics timeline before, during
    and after the load, deletion and replacement of an API pod — written to a git-ignored
    folder, with no credential.

## 4. Scope

In scope: `k8s/configmap.yaml`, `k8s/deployment.yaml`, `k8s/service.yaml`, `k8s/hpa.yaml`,
`k8s/migration-job.yaml`, `k8s/bootstrap-admin-job.yaml`,
`k8s/templates/secrets.example.yaml`, `k8s/load-test/load-generator-job.yaml`,
`k8s/README.md`; `scripts/k8s-evidence.sh`; `Makefile` `k8s-*` targets; the manifest job of
`.github/workflows/iac.yml`.

Out of scope: the cluster, `metrics-server`, the `Namespace` and Postgres (FP-35); the
pipeline that renders/applies the files, creates the `Secret`s and the migrations
`ConfigMap` (FP-36); the image (FP-32); the migration files and tooling (FP-33).

## 5. Acceptance criteria

1. With FP-35's environment up and the `Secret`s created, the documented sequence (BR8)
   brings the API up: migration `Job` `Complete`, bootstrap `Job` `Complete`, rollout
   complete, `GET /health` → 200 through the `Service`.
2. The `Deployment` runs the exact reference it was given (`name:<sha>@sha256:<digest>`);
   nothing committed contains a placeholder or a mutable tag reaching the cluster.
3. `kubectl get hpa` shows real CPU and memory values (not `<unknown>`), `minReplicas` 2,
   `maxReplicas` 8.
4. Re-applying the rollout manifests while the HPA has scaled out does not change the
   replica count.
5. Load generated **inside the cluster against the `Service`** raises the replica count,
   and the count returns towards the minimum after the load stops; the timeline (replicas,
   HPA metrics, `kubectl top`) before, during and after is recorded.
6. Deleting an API pod results in a replacement that passes readiness while the `Service`
   keeps serving from the remaining pod(s); data is unaffected (it lives in Postgres).
7. A failing migration `Job` leaves no `Deployment` created by that run.
8. No real secret value is committed; the `Secret` template is not read by
   `kubectl apply -f k8s/`.
9. Every manifest passes strict schema validation and renders through the same kustomize
   step FP-36 uses; every pod passes the `restricted` Pod Security profile.

## 6. Out of scope

- `Ingress`, external DNS, TLS termination — not in the PDF's list; the demo reaches the
  API with `kubectl port-forward` (FP-36 §4 `smoke` does the same).
- `NetworkPolicy`, `PodDisruptionBudget` — good practice, not requested; with one node and
  one Postgres replica they would not change what the phase can demonstrate.
- Helm or kustomize overlays for the manifests — the PDF asks for YAML manifests; the only
  templating is FP-36's deploy-time kustomize render (image, name suffix, annotations).
- Graceful shutdown in the API process (handling `SIGTERM`) — application code; the pod
  spec compensates with a `preStop` delay (`design.md` §3) and the gap is recorded there.
- Node autoscaling and Postgres high availability — FP-35 limits.

## 7. Open question — closed

The local-vs-cloud decision is closed in `specs/infrastructure-provisioning/requirements.md`
§7 (local `kind`). The demo uses `kubectl port-forward` against the `ClusterIP` `Service`;
no `NodePort`/`LoadBalancer` variant is needed.
