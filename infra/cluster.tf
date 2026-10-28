# Local Kubernetes cluster (kind). Terraform is its only creator and destroyer —
# nothing else runs `kind create cluster` for it (requirements.md BR1).
locals {
  # Absolute, so `terraform -chdir=infra output -raw kubeconfig_path` is usable from the
  # repository root (FP-36's scripts/deploy.sh). The directory is git-ignored.
  kubeconfig_path = abspath("${path.module}/.kube/${var.cluster_name}.kubeconfig")

  # Names fixed by the deploy contract (specs/ci-cd-pipeline/design.md §9), so they are
  # not variables.
  namespace     = "automotive-workshop"
  postgres_name = "postgres"
  postgres_port = 5432

  common_labels = {
    "app.kubernetes.io/part-of"    = "automotive-workshop"
    "app.kubernetes.io/managed-by" = "terraform"
  }
}

resource "kind_cluster" "this" {
  name            = var.cluster_name
  node_image      = var.node_image
  wait_for_ready  = true
  kubeconfig_path = local.kubeconfig_path

  kind_config {
    kind        = "Cluster"
    api_version = "kind.x-k8s.io/v1alpha4"

    # One node: the phase demonstrates pod autoscaling (HPA), not node autoscaling.
    node {
      role = "control-plane"
    }
  }
}
