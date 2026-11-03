# One root, one state (design.md §2): the kubernetes and helm providers are configured
# from the cluster resource itself. On a clean apply those values are unknown at plan
# time, which is fine for the resources used here — they plan a create without calling
# the cluster API. Do not add kubernetes_manifest: it needs the API at plan time and
# would force a second state.
provider "kind" {}

provider "kubernetes" {
  host                   = kind_cluster.this.endpoint
  cluster_ca_certificate = kind_cluster.this.cluster_ca_certificate
  client_certificate     = kind_cluster.this.client_certificate
  client_key             = kind_cluster.this.client_key
}

provider "helm" {
  kubernetes = {
    host                   = kind_cluster.this.endpoint
    cluster_ca_certificate = kind_cluster.this.cluster_ca_certificate
    client_certificate     = kind_cluster.this.client_certificate
    client_key             = kind_cluster.this.client_key
  }
}
