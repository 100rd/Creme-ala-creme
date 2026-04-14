terraform {
  required_version = "~> 1.6"

  # Backend config is provided per-environment at init time:
  #   terraform init -backend-config=dev/backend.hcl
  #   terraform init -backend-config=prod/backend.hcl
  backend "s3" {}

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.46"
    }
    random = {
      source  = "hashicorp/random"
      version = "~> 3.6"
    }
    kubernetes = {
      source  = "hashicorp/kubernetes"
      version = "~> 2.27"
    }
  }
}
