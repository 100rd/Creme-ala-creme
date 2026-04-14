output "namespace" {
  description = "Name of the created namespace"
  value       = kubernetes_namespace.operator.metadata[0].name
}

output "service_account_name" {
  description = "Name of the operator ServiceAccount"
  value       = kubernetes_service_account.operator.metadata[0].name
}

output "cluster_role_name" {
  description = "Name of the operator ClusterRole"
  value       = kubernetes_cluster_role.operator.metadata[0].name
}

output "cluster_role_binding_name" {
  description = "Name of the operator ClusterRoleBinding"
  value       = kubernetes_cluster_role_binding.operator.metadata[0].name
}
