# Terraform and provider pins (specs/infrastructure-provisioning/design.md §8).
# Versions are raised on purpose, as a reviewed change — never with "~>" drift.
terraform {
  required_version = ">= 1.16.0, < 2.0.0"

  required_providers {
    kind = {
      source  = "tehcyx/kind"
      version = "0.13.0"
    }
    kubernetes = {
      source  = "hashicorp/kubernetes"
      version = "3.4.0"
    }
    helm = {
      source  = "hashicorp/helm"
      version = "3.3.0"
    }
  }
}
