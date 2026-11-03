variable "postgres_password" {
  description = "Password of the Postgres superuser. Supply it via TF_VAR_postgres_password or a git-ignored terraform.tfvars — never commit it."
  type        = string
  sensitive   = true

  validation {
    condition     = length(var.postgres_password) >= 12
    error_message = "postgres_password must be at least 12 characters long (e.g. `openssl rand -hex 32`)."
  }
}

variable "postgres_user" {
  description = "Postgres superuser name (same default as Docker Compose)."
  type        = string
  default     = "workshop"
}

variable "postgres_db" {
  description = "Application database name (same default as Docker Compose)."
  type        = string
  default     = "automotive_workshop"
}

variable "cluster_name" {
  description = "Name of the kind cluster; also names its kubeconfig file in infra/.kube/."
  type        = string
  default     = "automotive-workshop"
}

variable "node_image" {
  description = "kind node image. The default is the one kind v0.33.0 (embedded by tehcyx/kind 0.13.0) was released with. Hosts on cgroup v1 need a node image below Kubernetes 1.35 (see infra/README.md)."
  type        = string
  default     = "kindest/node:v1.37.0@sha256:a1ed56cfb0e7b93589bdf97c8cd566405a265939e3620fc4f5de89adff580ae5"
}

variable "postgres_image" {
  description = "Postgres image, pinned by digest. Keep the major version in sync with docker-compose.yml."
  type        = string
  default     = "postgres:16.15-alpine3.24@sha256:f4610795587492cd4fb887c2dc3de301b813278bb6f616992061141b5f15ac36"
}

variable "postgres_storage_size" {
  description = "Size of the Postgres PersistentVolumeClaim."
  type        = string
  default     = "1Gi"
}

variable "postgres_max_connections" {
  description = "Postgres max_connections. Budget: HPA max (8) x the API pool per pod (max(4, host CPUs)) + migration/bootstrap Jobs."
  type        = number
  default     = 300
}
