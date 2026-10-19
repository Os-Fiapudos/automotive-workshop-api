# Tasks — Kubernetes Manifests (`/k8s`)

## 1. Spec confirmation

- [ ] Confirm local-vs-cloud cluster decision (`requirements.md` §7) before the `Service`
      type for the video demo is finalized — the base manifests (§2–§6 below) are
      cluster-agnostic and can be written regardless.

## 2. Manifests (`design.md` §1–§6)

- [ ] `k8s/namespace.yaml`.
- [ ] `k8s/secret.yaml` — placeholder values only, never a real secret committed.
- [ ] `k8s/deployment.yaml` — `resources.requests`/`limits`, readiness/liveness probes
      against `/health`, `envFrom` the Secret.
- [ ] `k8s/service.yaml` — `ClusterIP`.
- [ ] `k8s/hpa.yaml` — CPU + memory, `minReplicas`/`maxReplicas` matching the Deployment's
      floor.
- [ ] `k8s/migration-job.yaml` — uses the pinned `migrate/migrate` image directly (no
      custom image to build); mounts migrations via a `ConfigMap`, not baked in
      (`design.md` §6).
- [ ] `k8s/migration-configmap.yaml` — generated from `migrations/`
      (`kubectl create configmap ... --from-file=migrations/ --dry-run=client -o yaml`,
      `design.md` §6) — add a note in this file's header (or the README) that it must be
      regenerated whenever `specs/database-migrations/` adds a new migration file.

## 3. Local verification (kind/minikube)

- [ ] Install `metrics-server` locally (if not already provisioned by
      `specs/infrastructure-provisioning/` by the time this is tested).
- [ ] `kubectl apply -f k8s/` with a real, locally-created `Secret` (not the committed
      template) against a database `specs/infrastructure-provisioning/` already provisioned.
- [ ] `kubectl get pods`/`kubectl get svc`/`kubectl get hpa` — confirm the API is reachable
      and the HPA reports real metrics.
- [ ] `kubectl delete pod <one-api-pod>` — confirm the Deployment replaces it and the
      replacement passes readiness before receiving traffic.
- [ ] Simulate load (e.g. `hey`/`ab`/a simple loop of `curl`) against the Service — confirm
      `kubectl get hpa` shows replica count increase, then decrease once load stops
      (`requirements.md` AC4 — this is also the video demo's own evidence,
      `specs/phase-2-deliverables/`).
- [ ] `kubectl apply -f k8s/migration-job.yaml`; `kubectl wait --for=condition=complete
      job/...` — confirm it completes before the API rollout is considered done.

## 4. Documentation

- [ ] README "Deploy em Kubernetes" section — `kubectl apply -f k8s/` walkthrough,
      including the `Secret` creation step (`specs/phase-2-deliverables/`, referenced).

## 5. Verification

- [ ] Re-check `requirements.md` AC1–AC6 against the actual cluster behavior observed.
