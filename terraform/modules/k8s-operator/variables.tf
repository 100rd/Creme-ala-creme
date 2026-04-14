variable "operator_name" {
  description = "Name of the Kubernetes operator (used for all resource names)"
  type        = string
}

variable "namespace" {
  description = "Kubernetes namespace to create for the operator"
  type        = string
}

variable "iam_role_arn" {
  description = "IAM role ARN for IRSA annotation on the ServiceAccount (empty string to disable)"
  type        = string
  default     = ""
}

variable "pod_security_level" {
  description = "Pod Security Standards level for the namespace (restricted, baseline, or privileged)"
  type        = string
  default     = "restricted"

  validation {
    condition     = contains(["restricted", "baseline", "privileged"], var.pod_security_level)
    error_message = "pod_security_level must be one of: restricted, baseline, privileged"
  }
}

variable "namespace_labels" {
  description = "Additional labels to apply to the operator namespace"
  type        = map(string)
  default     = {}
}

variable "resource_quota" {
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

variable "limit_range" {
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
