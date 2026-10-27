# Postgres 16 inside the cluster: StatefulSet + PersistentVolumeClaim + headless Service
# (requirements.md BR3, design.md §6). The API reaches it at
# postgres.automotive-workshop.svc.cluster.local:5432.
locals {
  postgres_labels = merge(local.common_labels, {
    "app.kubernetes.io/name"      = local.postgres_name
    "app.kubernetes.io/component" = "database"
  })
  postgres_selector = {
    "app.kubernetes.io/name"      = local.postgres_name
    "app.kubernetes.io/component" = "database"
  }

  # TCP, not the socket: during the first start the image's temporary initdb server only
  # listens on the socket, so a socket check would pass too early.
  pg_isready = ["pg_isready", "-q", "-h", "127.0.0.1", "-p", tostring(local.postgres_port), "-U", var.postgres_user, "-d", var.postgres_db]
}

# Read only by the Postgres pod. The API gets DATABASE_URL from the deploy's own Secret,
# built from the postgres_database_url_template output.
resource "kubernetes_secret_v1" "postgres_credentials" {
  metadata {
    name      = "postgres-credentials"
    namespace = kubernetes_namespace_v1.this.metadata[0].name
    labels    = local.postgres_labels
  }

  data = {
    username = var.postgres_user
    password = var.postgres_password
  }
}

resource "kubernetes_service_v1" "postgres" {
  metadata {
    name      = local.postgres_name
    namespace = kubernetes_namespace_v1.this.metadata[0].name
    labels    = local.postgres_labels
  }

  spec {
    cluster_ip = "None" # headless: stable DNS name for the StatefulSet pod
    selector   = local.postgres_selector

    port {
      name        = "postgres"
      port        = local.postgres_port
      target_port = "postgres"
    }
  }
}

resource "kubernetes_stateful_set_v1" "postgres" {
  metadata {
    name      = local.postgres_name
    namespace = kubernetes_namespace_v1.this.metadata[0].name
    labels    = local.postgres_labels
  }

  # apply returns only when postgres-0 passes its readiness probe.
  wait_for_rollout = true

  spec {
    service_name = kubernetes_service_v1.postgres.metadata[0].name
    replicas     = 1

    selector {
      match_labels = local.postgres_selector
    }

    # Deleting the pod or the StatefulSet keeps the data; only destroying the cluster
    # removes it.
    persistent_volume_claim_retention_policy {
      when_deleted = "Retain"
      when_scaled  = "Retain"
    }

    template {
      metadata {
        labels = local.postgres_labels
      }

      spec {
        automount_service_account_token = false

        # restricted Pod Security profile. 70 is the postgres user of the Alpine image;
        # its entrypoint skips the root-only chown step when it already runs as that user.
        security_context {
          run_as_non_root = true
          run_as_user     = 70
          run_as_group    = 70
          fs_group        = 70

          seccomp_profile {
            type = "RuntimeDefault"
          }
        }

        container {
          name  = "postgres"
          image = var.postgres_image
          args  = ["-c", "max_connections=${var.postgres_max_connections}"]

          port {
            name           = "postgres"
            container_port = local.postgres_port
          }

          env {
            name  = "POSTGRES_USER"
            value = var.postgres_user
          }
          env {
            name  = "POSTGRES_DB"
            value = var.postgres_db
          }
          env {
            name = "POSTGRES_PASSWORD"
            value_from {
              secret_key_ref {
                name = kubernetes_secret_v1.postgres_credentials.metadata[0].name
                key  = "password"
              }
            }
          }
          env {
            # A subdirectory: initdb refuses a mount point that is not empty.
            name  = "PGDATA"
            value = "/var/lib/postgresql/data/pgdata"
          }

          resources {
            requests = {
              cpu    = "250m"
              memory = "256Mi"
            }
            limits = {
              cpu    = "1"
              memory = "512Mi"
            }
          }

          startup_probe {
            exec {
              command = local.pg_isready
            }
            period_seconds    = 2
            timeout_seconds   = 2
            failure_threshold = 30
          }
          readiness_probe {
            exec {
              command = local.pg_isready
            }
            period_seconds    = 5
            timeout_seconds   = 3
            failure_threshold = 3
          }
          liveness_probe {
            exec {
              command = local.pg_isready
            }
            period_seconds    = 10
            timeout_seconds   = 5
            failure_threshold = 6
          }

          # run_as_non_root is repeated here on purpose: the provider sends false for an
          # unset bool in this block, which the restricted profile rejects.
          security_context {
            run_as_non_root            = true
            allow_privilege_escalation = false
            capabilities {
              drop = ["ALL"]
            }
          }

          volume_mount {
            name       = "data"
            mount_path = "/var/lib/postgresql/data"
          }
          volume_mount {
            name       = "run"
            mount_path = "/var/run/postgresql"
          }
        }

        volume {
          name = "run"
          empty_dir {}
        }
      }
    }

    volume_claim_template {
      metadata {
        name   = "data"
        labels = local.postgres_labels
      }

      spec {
        access_modes = ["ReadWriteOnce"]

        resources {
          requests = {
            storage = var.postgres_storage_size
          }
        }
      }
    }
  }
}
