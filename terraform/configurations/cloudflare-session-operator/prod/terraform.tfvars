aws_region = "us-east-1"

private_subnet_ids = [
  "subnet-aaaaaaaa",
  "subnet-bbbbbbbb"
]

db_instance_class               = "db.m6g.large"
db_allocated_storage            = 100
db_max_allocated_storage        = 1000
db_engine_version               = "14.10"
db_multi_az                     = true
db_backup_retention             = 14
db_deletion_protection          = true
db_skip_final_snapshot          = false
db_performance_insights_enabled = true

# k8s-operator module variables
k8s_operator_namespace    = "cloudflare-system"
k8s_operator_iam_role_arn = "arn:aws:iam::123456789012:role/cloudflare-operator-prod"
k8s_operator_pod_security = "restricted"

k8s_operator_resource_quota = {
  requests_cpu    = "2"
  requests_memory = "1Gi"
  limits_cpu      = "4"
  limits_memory   = "2Gi"
  pods            = "20"
}

k8s_operator_limit_range = {
  default_cpu            = "500m"
  default_memory         = "256Mi"
  default_request_cpu    = "100m"
  default_request_memory = "128Mi"
  max_cpu                = "2"
  max_memory             = "1Gi"
}
