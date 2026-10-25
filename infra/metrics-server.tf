# metrics-server: the source of the CPU/memory metrics the API's HPA scales on
# (requirements.md BR2).
resource "helm_release" "metrics_server" {
  name       = "metrics-server"
  repository = "https://kubernetes-sigs.github.io/metrics-server/"
  chart      = "metrics-server"
  version    = "3.13.1" # app v0.8.1
  namespace  = "kube-system"
  wait       = true
  timeout    = 300

  # Appended to the chart's defaultArgs. kind's kubelet serves a self-signed certificate,
  # so this flag is the one local-cluster-only setting (requirements.md §6).
  values = [yamlencode({
    args = ["--kubelet-insecure-tls"]
  })]
}
