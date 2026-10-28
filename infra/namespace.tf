# The project namespace. Terraform is its only owner: /k8s has no Namespace manifest and
# the deploy flow never modifies it (requirements.md BR4).
resource "kubernetes_namespace_v1" "this" {
  metadata {
    name = local.namespace

    labels = merge(local.common_labels, {
      # Every workload in the namespace is written to the restricted profile, so a
      # regression is rejected at admission instead of running unnoticed (design.md §4).
      "pod-security.kubernetes.io/enforce"         = "restricted"
      "pod-security.kubernetes.io/enforce-version" = "latest"
      "pod-security.kubernetes.io/audit"           = "restricted"
      "pod-security.kubernetes.io/warn"            = "restricted"
    })
  }
}
