# Design — Kubernetes Manifests (`/k8s`)

Satisfies: `requirements.md` (all sections).

## 1. `k8s/namespace.yaml`

```yaml
apiVersion: v1
kind: Namespace
metadata:
  name: automotive-workshop
```

Every other manifest targets this namespace (`metadata.namespace:
automotive-workshop`) — isolates this project's resources from anything else that might
run in the same cluster (relevant for a shared local kind/minikube cluster), and matches
Terraform's own namespace creation, if `specs/infrastructure-provisioning/` creates it there
instead (coordinated, not duplicated — whichever spec is implemented first creates it, the
other reuses the `kubernetes_namespace`/`Namespace` resource that already exists).

## 2. `k8s/secret.yaml` (template)

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: automotive-workshop-api-secrets
  namespace: automotive-workshop
type: Opaque
stringData:
  DATABASE_URL: "REPLACE_ME"
  JWT_SECRET: "REPLACE_ME"
  EXTERNAL_NOTIFICATION_SECRET: "REPLACE_ME"
  EXTERNAL_STATUS_UPDATE_SECRET: "REPLACE_ME"
  RESEND_API_KEY: "REPLACE_ME"
```

Committed with `REPLACE_ME` placeholders only (`requirements.md` AC6) — the README
(`specs/phase-2-deliverables/`) documents applying the real one via
`kubectl create secret generic ... --from-literal=...` or `kubectl apply -f` a
locally-edited, gitignored copy, the same "never commit a real secret" posture
`.env`/`.env.example` already establish.

## 3. `k8s/deployment.yaml`

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: automotive-workshop-api
  namespace: automotive-workshop
spec:
  replicas: 2
  selector:
    matchLabels:
      app: automotive-workshop-api
  template:
    metadata:
      labels:
        app: automotive-workshop-api
    spec:
      containers:
        - name: api
          image: ghcr.io/<org>/automotive-workshop-api:<tag>  # set by CI/CD, specs/ci-cd-pipeline/
          ports:
            - containerPort: 8080
          envFrom:
            - secretRef:
                name: automotive-workshop-api-secrets
          resources:
            requests:
              cpu: 100m
              memory: 128Mi
            limits:
              cpu: 500m
              memory: 256Mi
          readinessProbe:
            httpGet:
              path: /health
              port: 8080
            initialDelaySeconds: 3
            periodSeconds: 5
          livenessProbe:
            httpGet:
              path: /health
              port: 8080
            initialDelaySeconds: 10
            periodSeconds: 10
```

`resources.requests` (BR1) is what the HPA (§5) computes utilization *percentage* against —
without it, CPU-based HPA scaling has no baseline and silently does nothing. `replicas: 2`
is the floor the HPA's `minReplicas` (§5) also uses, so the `Deployment`'s own static
`replicas` and the HPA's floor agree (avoids `kubectl apply` fighting the HPA on every
apply).

## 4. `k8s/service.yaml`

```yaml
apiVersion: v1
kind: Service
metadata:
  name: automotive-workshop-api
  namespace: automotive-workshop
spec:
  selector:
    app: automotive-workshop-api
  ports:
    - port: 8080
      targetPort: 8080
  type: ClusterIP
```

`ClusterIP` satisfies BR2 (in-cluster reachability); the video demo
(`specs/phase-2-deliverables/`) reaches it via `kubectl port-forward svc/automotive-workshop-api
8080:8080` locally, or a cloud `LoadBalancer`/`NodePort` variant applied as a second,
demo-only manifest if the cluster decision (`requirements.md` §7) lands on cloud — kept out
of this base `service.yaml` so the committed manifest stays cluster-agnostic.

## 5. `k8s/hpa.yaml`

```yaml
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: automotive-workshop-api
  namespace: automotive-workshop
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: automotive-workshop-api
  minReplicas: 2
  maxReplicas: 8
  metrics:
    - type: Resource
      resource:
        name: cpu
        target:
          type: Utilization
          averageUtilization: 70
    - type: Resource
      resource:
        name: memory
        target:
          type: Utilization
          averageUtilization: 80
```

Both CPU and memory metrics (`requirements.md` BR4 — the PDF names both) —
`autoscaling/v2`'s multi-metric behavior scales up if *either* threshold is exceeded and
only scales down once *both* are back under their targets, the standard, safe default.
Requires `metrics-server` running in the cluster — installed by
`specs/infrastructure-provisioning/`, referenced here, not duplicated.

## 6. `k8s/migration-job.yaml`

**No custom image is built for this** — it uses the exact same pinned `migrate/migrate`
image `specs/database-migrations/design.md` §1 already pins for local/CI use, with
`migrations/*.sql` (`specs/database-migrations/design.md` §2) injected via a `ConfigMap`
instead of baked into a one-off image. This avoids introducing a second image this project
would need to build, tag, and push (`specs/ci-cd-pipeline/` has no step that builds one, and
adding one only for this Job would be new, undocumented scope):

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: automotive-workshop-api-migrations
  namespace: automotive-workshop
# generated from the migrations/ directory, not hand-written:
#   kubectl create configmap automotive-workshop-api-migrations \
#     --from-file=migrations/ -n automotive-workshop --dry-run=client -o yaml > k8s/migration-configmap.yaml
# regenerated whenever migrations/ gains a new file — both specs/database-migrations/ and
# this spec's own tasks.md call this step out explicitly so it isn't forgotten on a future
# schema change.
---
apiVersion: batch/v1
kind: Job
metadata:
  name: automotive-workshop-api-migrate
  namespace: automotive-workshop
spec:
  backoffLimit: 2
  template:
    spec:
      restartPolicy: Never
      containers:
        - name: migrate
          image: migrate/migrate:v4.18.1  # same pinned tag as specs/database-migrations/design.md §1
          args: ["-path=/migrations", "-database", "$(DATABASE_URL)", "up"]
          volumeMounts:
            - name: migrations
              mountPath: /migrations
          envFrom:
            - secretRef:
                name: automotive-workshop-api-secrets
      volumes:
        - name: migrations
          configMap:
            name: automotive-workshop-api-migrations
```

Applied (or re-applied, as a new `Job` object with a version-suffixed name, since a `Job`'s
`spec.template` is immutable once created — `kubectl delete job ... && kubectl apply -f
k8s/migration-job.yaml`, or a CI-generated unique name per deploy) **before** the
`Deployment`'s rollout, in both the manual walkthrough and `specs/ci-cd-pipeline/`'s own
step ordering (`requirements.md` BR7/AC5) — referenced there, not duplicated. A `ConfigMap`
has a 1MiB total size limit, far above what this project's schema migrations need; if that
ever stops being true, baking a dedicated image becomes a real option, not assumed here.

## 7. Traceability

| Requirement | Design section |
| --- | --- |
| BR1 (Deployment, resources set) | §3 |
| BR2 (ClusterIP Service) | §4 |
| BR3 (Secret/ConfigMap, no hardcoded values) | §2, §3 |
| BR4 (HPA, CPU + memory) | §5 |
| BR5 (probes against /health) | §3 |
| BR6 (DATABASE_URL from Secret, DB location-agnostic) | §2 |
| BR7 (migration Job before rollout) | §6 |
| AC1–AC6 | §1–§6 |
