################################################################################
# k8s-operator Terraform Module
#
# Provisions Kubernetes resources for a controller-runtime operator:
#   - Namespace with Pod Security Standards labels
#   - ServiceAccount (with optional IRSA annotation)
#   - ClusterRole + ClusterRoleBinding (operator RBAC)
#   - Role + RoleBinding for leader election
#   - ResourceQuota and LimitRange
################################################################################

terraform {
  required_version = ">= 1.5.0"

  required_providers {
    kubernetes = {
      source  = "hashicorp/kubernetes"
      version = "~> 2.25"
    }
  }
}

# --------------------------------------------------------------------------
# Namespace
# --------------------------------------------------------------------------
resource "kubernetes_namespace" "operator" {
  metadata {
    name = var.namespace

    labels = merge(
      {
        "app.kubernetes.io/name"       = var.operator_name
        "app.kubernetes.io/managed-by" = "terraform"
        # Pod Security Standards (Kubernetes 1.23+)
        "pod-security.kubernetes.io/enforce"         = var.pod_security_level
        "pod-security.kubernetes.io/enforce-version" = "latest"
        "pod-security.kubernetes.io/audit"           = var.pod_security_level
        "pod-security.kubernetes.io/audit-version"   = "latest"
        "pod-security.kubernetes.io/warn"            = var.pod_security_level
        "pod-security.kubernetes.io/warn-version"    = "latest"
      },
      var.namespace_labels,
    )
  }
}

# --------------------------------------------------------------------------
# ServiceAccount
# --------------------------------------------------------------------------
resource "kubernetes_service_account" "operator" {
  metadata {
    name      = var.operator_name
    namespace = kubernetes_namespace.operator.metadata[0].name

    labels = {
      "app.kubernetes.io/name"       = var.operator_name
      "app.kubernetes.io/managed-by" = "terraform"
    }

    annotations = var.iam_role_arn != "" ? {
      "eks.amazonaws.com/role-arn" = var.iam_role_arn
    } : {}
  }

  automount_service_account_token = false
}

# --------------------------------------------------------------------------
# ClusterRole -- operator RBAC
# --------------------------------------------------------------------------
resource "kubernetes_cluster_role" "operator" {
  metadata {
    name = var.operator_name

    labels = {
      "app.kubernetes.io/name"       = var.operator_name
      "app.kubernetes.io/managed-by" = "terraform"
    }
  }

  # SessionBinding CRD access
  rule {
    api_groups = ["cloudflare.example.com"]
    resources  = ["sessionbindings"]
    verbs      = ["get", "list", "watch", "create", "update", "patch", "delete"]
  }

  rule {
    api_groups = ["cloudflare.example.com"]
    resources  = ["sessionbindings/status"]
    verbs      = ["get", "update", "patch"]
  }

  rule {
    api_groups = ["cloudflare.example.com"]
    resources  = ["sessionbindings/finalizers"]
    verbs      = ["update"]
  }

  # Pod management
  rule {
    api_groups = [""]
    resources  = ["pods"]
    verbs      = ["get", "list", "watch", "create", "update", "patch", "delete"]
  }

  # Read deployments (for pod template cloning)
  rule {
    api_groups = ["apps"]
    resources  = ["deployments"]
    verbs      = ["get", "list", "watch"]
  }

  # Event creation
  rule {
    api_groups = [""]
    resources  = ["events"]
    verbs      = ["create", "patch"]
  }
}

# --------------------------------------------------------------------------
# ClusterRoleBinding
# --------------------------------------------------------------------------
resource "kubernetes_cluster_role_binding" "operator" {
  metadata {
    name = var.operator_name

    labels = {
      "app.kubernetes.io/name"       = var.operator_name
      "app.kubernetes.io/managed-by" = "terraform"
    }
  }

  role_ref {
    api_group = "rbac.authorization.k8s.io"
    kind      = "ClusterRole"
    name      = kubernetes_cluster_role.operator.metadata[0].name
  }

  subject {
    kind      = "ServiceAccount"
    name      = kubernetes_service_account.operator.metadata[0].name
    namespace = kubernetes_namespace.operator.metadata[0].name
  }
}

# --------------------------------------------------------------------------
# Role -- leader election (namespace-scoped)
# --------------------------------------------------------------------------
resource "kubernetes_role" "leader_election" {
  metadata {
    name      = "${var.operator_name}-leader-election"
    namespace = kubernetes_namespace.operator.metadata[0].name

    labels = {
      "app.kubernetes.io/name"       = var.operator_name
      "app.kubernetes.io/managed-by" = "terraform"
    }
  }

  rule {
    api_groups = ["coordination.k8s.io"]
    resources  = ["leases"]
    verbs      = ["get", "list", "watch", "create", "update", "patch", "delete"]
  }

  rule {
    api_groups = [""]
    resources  = ["events"]
    verbs      = ["create", "patch"]
  }
}

resource "kubernetes_role_binding" "leader_election" {
  metadata {
    name      = "${var.operator_name}-leader-election"
    namespace = kubernetes_namespace.operator.metadata[0].name

    labels = {
      "app.kubernetes.io/name"       = var.operator_name
      "app.kubernetes.io/managed-by" = "terraform"
    }
  }

  role_ref {
    api_group = "rbac.authorization.k8s.io"
    kind      = "Role"
    name      = kubernetes_role.leader_election.metadata[0].name
  }

  subject {
    kind      = "ServiceAccount"
    name      = kubernetes_service_account.operator.metadata[0].name
    namespace = kubernetes_namespace.operator.metadata[0].name
  }
}

# --------------------------------------------------------------------------
# ResourceQuota
# --------------------------------------------------------------------------
resource "kubernetes_resource_quota" "operator" {
  count = var.resource_quota != null ? 1 : 0

  metadata {
    name      = var.operator_name
    namespace = kubernetes_namespace.operator.metadata[0].name

    labels = {
      "app.kubernetes.io/name"       = var.operator_name
      "app.kubernetes.io/managed-by" = "terraform"
    }
  }

  spec {
    hard = {
      "requests.cpu"    = var.resource_quota.requests_cpu
      "requests.memory" = var.resource_quota.requests_memory
      "limits.cpu"      = var.resource_quota.limits_cpu
      "limits.memory"   = var.resource_quota.limits_memory
      "pods"            = var.resource_quota.pods
    }
  }
}

# --------------------------------------------------------------------------
# LimitRange
# --------------------------------------------------------------------------
resource "kubernetes_limit_range" "operator" {
  count = var.limit_range != null ? 1 : 0

  metadata {
    name      = var.operator_name
    namespace = kubernetes_namespace.operator.metadata[0].name

    labels = {
      "app.kubernetes.io/name"       = var.operator_name
      "app.kubernetes.io/managed-by" = "terraform"
    }
  }

  spec {
    limit {
      type = "Container"

      default = {
        cpu    = var.limit_range.default_cpu
        memory = var.limit_range.default_memory
      }

      default_request = {
        cpu    = var.limit_range.default_request_cpu
        memory = var.limit_range.default_request_memory
      }

      max = {
        cpu    = var.limit_range.max_cpu
        memory = var.limit_range.max_memory
      }
    }
  }
}
