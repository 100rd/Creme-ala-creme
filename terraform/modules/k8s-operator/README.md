# k8s-operator Terraform Module

Provisions Kubernetes resources for a controller-runtime based operator.

## Resources Created

- **Namespace** with Pod Security Standards labels (enforce, audit, warn)
- **ServiceAccount** with optional IRSA annotation for AWS IAM integration
- **ClusterRole** with least-privilege RBAC for the operator's CRD, pods, deployments, and events
- **ClusterRoleBinding** binding the ClusterRole to the operator's ServiceAccount
- **Role** (namespace-scoped) for leader election lease management
- **RoleBinding** for leader election
- **ResourceQuota** (optional) to limit resource consumption in the operator namespace
- **LimitRange** (optional) to set default and maximum resource limits for containers

## Usage

```hcl
module "k8s_operator" {
  source = "../../modules/k8s-operator"

  operator_name      = "cloudflare-session-operator"
  namespace          = "cloudflare-system"
  iam_role_arn       = "arn:aws:iam::123456789012:role/cloudflare-operator-role"
  pod_security_level = "restricted"

  resource_quota = {
    requests_cpu    = "1"
    requests_memory = "512Mi"
    limits_cpu      = "2"
    limits_memory   = "1Gi"
    pods            = "10"
  }

  limit_range = {
    default_cpu            = "200m"
    default_memory         = "256Mi"
    default_request_cpu    = "100m"
    default_request_memory = "128Mi"
    max_cpu                = "1"
    max_memory             = "512Mi"
  }
}
```

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|----------|
| `operator_name` | Name of the Kubernetes operator | `string` | n/a | yes |
| `namespace` | Namespace to create | `string` | n/a | yes |
| `iam_role_arn` | IAM role ARN for IRSA | `string` | `""` | no |
| `pod_security_level` | PSS level (restricted/baseline/privileged) | `string` | `"restricted"` | no |
| `namespace_labels` | Additional namespace labels | `map(string)` | `{}` | no |
| `resource_quota` | Resource quota settings | `object` | `null` | no |
| `limit_range` | Limit range settings | `object` | `null` | no |

## Outputs

| Name | Description |
|------|-------------|
| `namespace` | Created namespace name |
| `service_account_name` | Operator ServiceAccount name |
| `cluster_role_name` | Operator ClusterRole name |
| `cluster_role_binding_name` | Operator ClusterRoleBinding name |
