variable "app_name" {
  description = "Application name used for tagging and resource naming"
  type        = string
  default     = "cloudflare-session-operator"
}

variable "aws_region" {
  description = "AWS region"
  type        = string
}

variable "private_subnet_ids" {
  description = "List of private subnet IDs for RDS"
  type        = list(string)
}

variable "db_security_group_ids" {
  description = "Security group IDs to attach to the RDS instance"
  type        = list(string)
  default     = []
}

variable "db_engine_version" {
  description = "PostgreSQL engine version, e.g., 14.10"
  type        = string
  default     = "14.10"
}

variable "db_instance_class" {
  description = "RDS instance class"
  type        = string
  default     = "db.t3.micro"
}

variable "db_allocated_storage" {
  description = "Initial allocated storage (GiB)"
  type        = number
  default     = 20
}

variable "db_max_allocated_storage" {
  description = "Maximum allocated storage for autoscaling (GiB)"
  type        = number
  default     = 100
}

variable "db_master_password" {
  description = "Master password (if not provided, a random password will be generated)"
  type        = string
  default     = null
  sensitive   = true
}

variable "db_multi_az" {
  description = "Whether to create a Multi-AZ RDS deployment"
  type        = bool
  default     = false
}

variable "db_backup_retention" {
  description = "Backup retention period in days"
  type        = number
  default     = 7
}

variable "db_deletion_protection" {
  description = "Enable deletion protection"
  type        = bool
  default     = false
}

variable "db_skip_final_snapshot" {
  description = "Skip final snapshot on destroy"
  type        = bool
  default     = true
}

variable "db_performance_insights_enabled" {
  description = "Enable Performance Insights"
  type        = bool
  default     = false
}

# --------------------------------------------------------------------------
# k8s-operator module variables
# --------------------------------------------------------------------------

variable "k8s_operator_namespace" {
  description = "Kubernetes namespace for the operator"
  type        = string
  default     = "cloudflare-system"
}

variable "k8s_operator_iam_role_arn" {
  description = "IAM role ARN for IRSA annotation on the operator ServiceAccount"
  type        = string
  default     = ""
}

variable "k8s_operator_pod_security" {
  description = "Pod Security Standards level for the operator namespace"
  type        = string
  default     = "restricted"
}

variable "k8s_operator_resource_quota" {
  description = "Resource quota for the operator namespace (null to disable)"
  type = object({
    requests_cpu    = string
    requests_memory = string
    limits_cpu      = string
    limits_memory   = string
    pods            = string
  })
  default = null
}

variable "k8s_operator_limit_range" {
  description = "Limit range for containers in the operator namespace (null to disable)"
  type = object({
    default_cpu            = string
    default_memory         = string
    default_request_cpu    = string
    default_request_memory = string
    max_cpu                = string
    max_memory             = string
  })
  default = null
}
