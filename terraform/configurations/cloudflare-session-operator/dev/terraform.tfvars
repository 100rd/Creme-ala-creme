aws_region = "us-east-1"

private_subnet_ids = [
  "subnet-aaaaaaaa",
  "subnet-bbbbbbbb"
]

db_instance_class               = "db.t4g.micro"
db_allocated_storage            = 20
db_max_allocated_storage        = 100
db_engine_version               = "14.10"
db_multi_az                     = false
db_backup_retention             = 1
db_deletion_protection          = false
db_skip_final_snapshot          = true
db_performance_insights_enabled = false

# k8s-operator module variables
k8s_operator_namespace    = "cloudflare-system"
k8s_operator_iam_role_arn = ""
k8s_operator_pod_security = "restricted"

k8s_operator_resource_quota = {
  requests_cpu    = "500m"
  requests_memory = "256Mi"
  limits_cpu      = "1"
  limits_memory   = "512Mi"
  pods            = "5"
}

k8s_operator_limit_range = {
  default_cpu            = "200m"
  default_memory         = "128Mi"
  default_request_cpu    = "50m"
  default_request_memory = "64Mi"
  max_cpu                = "500m"
  max_memory             = "256Mi"
}
