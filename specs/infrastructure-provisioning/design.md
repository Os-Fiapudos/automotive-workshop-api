# Design — Infrastructure Provisioning (Terraform)

Satisfies: `requirements.md` (all sections). Assumes `requirements.md` §7's working default
(local `kind`); the cloud variant only changes §1 (`design.md` §6 explains the boundary).

## 1. `infra/cluster.tf` (local default)

```hcl
terraform {
  required_providers {
    kind       = { source = "tehcyx/kind", version = "~> 0.7" }
    kubernetes = { source = "hashicorp/kubernetes", version = "~> 2.31" }
    helm       = { source = "hashicorp/helm", version = "~> 2.14" }
  }
}

resource "kind_cluster" "this" {
  name           = "automotive-workshop"
  wait_for_ready = true

  kind_config {
    kind        = "Cluster"
    api_version = "kind.x-k8s.io/v1alpha4"
    node {
      role = "control-plane"
    }
  }
}

provider "kubernetes" {
  host                   = kind_cluster.this.endpoint
  cluster_ca_certificate = kind_cluster.this.cluster_ca_certificate
  client_certificate     = kind_cluster.this.client_certificate
  client_key             = kind_cluster.this.client_key
}

provider "helm" {
  kubernetes {
    host                   = kind_cluster.this.endpoint
    cluster_ca_certificate = kind_cluster.this.cluster_ca_certificate
    client_certificate     = kind_cluster.this.client_certificate
    client_key             = kind_cluster.this.client_key
  }
}
```

A single control-plane node is sufficient — this project demonstrates pod-level HPA scaling
(`specs/kubernetes-deployment/`), not node autoscaling, so a multi-node cluster adds
complexity without demonstrating anything the PDF's video requirement (p.6: "Escalabilidade
automática... pode simular aumento de carga") actually asks to show.

## 2. `infra/namespace.tf`

```hcl
resource "kubernetes_namespace" "this" {
  metadata {
    name = "automotive-workshop"
  }
}
```

Same namespace `specs/kubernetes-deployment/`'s own `k8s/namespace.yaml` declares
(`requirements.md` §1) — whichever is applied first (Terraform here, or a direct `kubectl
apply -f k8s/namespace.yaml`) creates it; the other is a no-op against the existing
`Namespace` object. Documented as a coordination point in both specs rather than silently
assumed to be owned by only one.

## 3. `infra/metrics-server.tf`

```hcl
resource "helm_release" "metrics_server" {
  name       = "metrics-server"
  repository = "https://kubernetes-sigs.github.io/metrics-server/"
  chart      = "metrics-server"
  version    = "3.12.1"
  namespace  = "kube-system"

  # kind's local, self-signed kubelet certs need this on a local cluster; a managed cloud
  # cluster typically does not, so this is one of the few values this spec flags as
  # local-cluster-specific without being a whole separate file (requirements.md §6).
  set {
    name  = "args[0]"
    value = "--kubelet-insecure-tls"
  }
}
```

Satisfies `requirements.md` BR2 — without this, `specs/kubernetes-deployment/`'s HPA would
show `<unknown>` for CPU/memory indefinitely instead of real metrics.

## 4. `infra/postgres.tf`

```hcl
resource "kubernetes_stateful_set" "postgres" {
  metadata {
    name      = "postgres"
    namespace = kubernetes_namespace.this.metadata[0].name
  }
  spec {
    service_name = "postgres"
    replicas     = 1
    selector { match_labels = { app = "postgres" } }
    template {
      metadata { labels = { app = "postgres" } }
      spec {
        container {
          name  = "postgres"
          image = "postgres:16-alpine"
          port { container_port = 5432 }
          env {
            name  = "POSTGRES_USER"
            value = var.postgres_user
          }
          env {
            name = "POSTGRES_PASSWORD"
            value_from {
              secret_key_ref {
                name = kubernetes_secret.postgres_credentials.metadata[0].name
                key  = "password"
              }
            }
          }
          env {
            name  = "POSTGRES_DB"
            value = var.postgres_db
          }
          volume_mount {
            name       = "data"
            mount_path = "/var/lib/postgresql/data"
          }
        }
      }
    }
    volume_claim_template {
      metadata { name = "data" }
      spec {
        access_modes       = ["ReadWriteOnce"]
        resources { requests = { storage = "1Gi" } }
      }
    }
  }
}

resource "kubernetes_secret" "postgres_credentials" {
  metadata {
    name      = "postgres-credentials"
    namespace = kubernetes_namespace.this.metadata[0].name
  }
  data = {
    password = var.postgres_password
  }
}

resource "kubernetes_service" "postgres" {
  metadata {
    name      = "postgres"
    namespace = kubernetes_namespace.this.metadata[0].name
  }
  spec {
    cluster_ip = "None"  # headless — StatefulSet convention
    selector   = { app = "postgres" }
    port { port = 5432 }
  }
}
```

Satisfies BR3. `postgres:16-alpine` matches `docker-compose.yml`'s existing local Postgres
version (`CLAUDE.md` §2 — Postgres 16), so the schema `docs/schema.sql`/`migrations/`
(`specs/database-migrations/`) target the same server version in both environments.

## 5. `infra/variables.tf` / `infra/outputs.tf`

```hcl
variable "postgres_user" {
  type    = string
  default = "workshop"
}
variable "postgres_db" {
  type    = string
  default = "automotive_workshop"
}
variable "postgres_password" {
  type      = string
  sensitive = true
}

output "postgres_host" {
  value = "postgres.${kubernetes_namespace.this.metadata[0].name}.svc.cluster.local"
}
output "postgres_port" {
  value = 5432
}
output "postgres_database_url_template" {
  value     = "postgres://${var.postgres_user}:<password>@postgres.${kubernetes_namespace.this.metadata[0].name}.svc.cluster.local:5432/${var.postgres_db}?sslmode=disable"
  sensitive = false
}
```

Satisfies BR5/BR6 — `postgres_password` is never written to plain `terraform output`;
`postgres_database_url_template` gives the exact string to substitute the real password
into when populating `specs/kubernetes-deployment/`'s `Secret`
(`sslmode=disable` matches this project's existing local-Postgres convention, `CLAUDE.md`
§13's "without an explicit, documented decision" caveat — documented here as that decision,
for this local/in-cluster Postgres specifically; a cloud-managed Postgres reached over the
public internet would need `sslmode=require` instead, a one-line change if/when §7's
decision lands on cloud).

`infra/terraform.tfvars.example`:
```hcl
postgres_password = "REPLACE_ME"
```

## 6. Local-vs-cloud boundary (`requirements.md` §6)

Only `infra/cluster.tf` (§1) and the provider credential variables change for a cloud
target; `namespace.tf` (§2), `metrics-server.tf` (§3, minus the `--kubelet-insecure-tls`
flag, which a managed cluster's valid kubelet certs don't need), `postgres.tf` (§4), and
`variables.tf`/`outputs.tf` (§5) are written against the generic `kubernetes`/`helm`
providers and apply unchanged to whichever cluster `cluster.tf` produces.

## 7. Documentation

`infra/README.md` (or linked from the root README, `specs/phase-2-deliverables/`): lists
every resource this configuration creates (cluster, namespace, metrics-server, Postgres
StatefulSet/PVC/Service/Secret), the exact `terraform init && terraform apply
-var-file=terraform.tfvars` / `terraform destroy -var-file=terraform.tfvars` commands
(`requirements.md` AC4), and a note that `specs/kubernetes-deployment/`'s own `Secret` must
be populated from `terraform output` (§5) before `kubectl apply -f k8s/`.

## 8. Testing strategy

Operational, not `go test`-based: `terraform validate`/`terraform plan` clean on every
change; `terraform apply` → verify cluster/metrics-server/Postgres reachable
(`requirements.md` AC1) → `terraform destroy` → verify no orphaned `kind` Docker
containers/networks/volumes remain (`docker ps -a`/`docker network ls`/`docker volume ls`,
AC3).

## 9. Traceability

| Requirement | Design section |
| --- | --- |
| BR1 (cluster) | §1 |
| BR2 (metrics-server) | §3 |
| BR3 (Postgres StatefulSet + PVC + headless Service) | §4 |
| BR4 (destroy removes everything) | §1, §4, §8 |
| BR5 (no hardcoded credentials) | §5 |
| BR6 (outputs feed the K8s Secret) | §5 |
| BR7 (documented resources/commands) | §7 |
| AC1–AC5 | §1, §3, §4, §5, §7, §8 |
