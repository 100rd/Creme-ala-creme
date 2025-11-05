# Production environment configuration for hello-world database

environment  = "prod"
aws_region   = "us-east-1"
team         = "platform"

# Network (update these with your actual values)
vpc_id       = "vpc-xxxxx"
cluster_name = "my-eks-cluster"

# Database
database_name   = "hellodb"
master_username = "dbadmin"
engine_version  = "16"

# Instance sizing (production-ready)
instance_class        = "db.r6g.large"
allocated_storage     = 100
max_allocated_storage = 500

# High Availability (ENABLED for production)
multi_az = true

# Backup (robust for production)
backup_retention_period = 30
backup_window           = "03:00-04:00"
maintenance_window      = "sun:04:00-sun:05:00"

# Protection (ENABLED for production)
deletion_protection = true
skip_final_snapshot = false

# Monitoring (enhanced for production)
monitoring_interval                   = 30  # More frequent monitoring
performance_insights_enabled          = true
performance_insights_retention_period = 31  # Month retention

# Alarms
max_connections_threshold = 200
# alarm_sns_topic_arn     = "arn:aws:sns:us-east-1:123456789:prod-critical-alerts"

additional_tags = {
  CostCenter    = "engineering"
  Project       = "hello-world"
  Compliance    = "required"
  BackupPolicy  = "30days"
}
