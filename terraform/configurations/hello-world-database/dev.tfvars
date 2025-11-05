# Development environment configuration for hello-world database

environment  = "dev"
aws_region   = "us-east-1"
team         = "platform"

# Network (update these with your actual values)
vpc_id       = "vpc-xxxxx"
cluster_name = "my-eks-cluster"

# Database
database_name   = "hellodb"
master_username = "dbadmin"
engine_version  = "16"

# Instance sizing (small for dev)
instance_class        = "db.t3.micro"
allocated_storage     = 20
max_allocated_storage = 50

# High Availability (disabled for dev to save costs)
multi_az = false

# Backup (minimal for dev)
backup_retention_period = 3
backup_window           = "03:00-04:00"
maintenance_window      = "sun:04:00-sun:05:00"

# Protection (relaxed for dev)
deletion_protection = false
skip_final_snapshot = true

# Monitoring
monitoring_interval                   = 60
performance_insights_enabled          = true
performance_insights_retention_period = 7

# Alarms
max_connections_threshold = 50
# alarm_sns_topic_arn     = "arn:aws:sns:us-east-1:123456789:dev-alerts"

additional_tags = {
  CostCenter = "engineering"
  Project    = "hello-world"
}
