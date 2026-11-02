# Consumed by the deploy (specs/ci-cd-pipeline/design.md §9 item 5). None contains a
# credential, so none is sensitive.
output "cluster_name" {
  description = "Name of the kind cluster."
  value       = kind_cluster.this.name
}

output "kube_context" {
  description = "Context name inside the kubeconfig."
  value       = kind_cluster.this.context_name
}

output "kubeconfig_path" {
  description = "Absolute path of this cluster's kubeconfig (export KUBECONFIG=...)."
  value       = kind_cluster.this.kubeconfig_path
}

output "namespace" {
  description = "Namespace of the application and the database."
  value       = kubernetes_namespace_v1.this.metadata[0].name
}

output "postgres_host" {
  description = "In-cluster DNS name of Postgres."
  value       = "${kubernetes_service_v1.postgres.metadata[0].name}.${kubernetes_namespace_v1.this.metadata[0].name}.svc.cluster.local"
}

output "postgres_port" {
  description = "Postgres port."
  value       = local.postgres_port
}

output "postgres_database" {
  description = "Application database name."
  value       = var.postgres_db
}

output "postgres_user" {
  description = "Postgres user name."
  value       = var.postgres_user
}

output "postgres_database_url_template" {
  description = "DATABASE_URL for the API Secret: replace the literal <password> with the percent-encoded password. sslmode=disable is deliberate for this in-cluster database (design.md §7)."
  value       = "postgres://${var.postgres_user}:<password>@${kubernetes_service_v1.postgres.metadata[0].name}.${kubernetes_namespace_v1.this.metadata[0].name}.svc.cluster.local:${local.postgres_port}/${var.postgres_db}?sslmode=disable"

  # Only meaningful once Postgres is ready.
  depends_on = [kubernetes_stateful_set_v1.postgres]
}
