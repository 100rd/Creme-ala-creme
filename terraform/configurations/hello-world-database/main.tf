# Example: Service-owned database for hello-world application
# Each service team can provision their own database using this pattern

terraform {
  required_version = ">= 1.5.0"

  backend "s3" {
    # Configure backend in backend.hcl or via CLI
    # bucket         = "my-terraform-state"
    # key            = "hello-world/database/terraform.tfstate"
    # region         = "us-east-1"
    # encrypt        = true
    # dynamodb_table = "terraform-locks"
  }

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
  }
}

provider "aws" {
  region = var.aws_region

  default_tags {
    tags = {
      Project     = "hello-world"
      ManagedBy   = "Terraform"
      Service     = "hello-world"
      Environment = var.environment
      Team        = var.team
    }
  }
}

# Data sources to get VPC and subnet information
data "aws_vpc" "main" {
  id = var.vpc_id
}

data "aws_subnets" "database" {
  filter {
    name   = "vpc-id"
    values = [var.vpc_id]
  }

  tags = {
    Tier = "database"
  }
}

# Security group for EKS pods that need database access
data "aws_security_group" "eks_pods" {
  vpc_id = var.vpc_id

  tags = {
    Name = "${var.cluster_name}-eks-pods"
  }
}

# RDS PostgreSQL instance for hello-world
module "hello_world_db" {
  source = "../../modules/rds-postgres"

  name_prefix = "hello-world-${var.environment}"
  environment = var.environment

  # Network
  vpc_id                      = var.vpc_id
  subnet_ids                  = data.aws_subnets.database.ids
  allowed_security_group_ids  = [data.aws_security_group.eks_pods.id]

  # Database
  database_name   = var.database_name
  master_username = var.master_username
  # master_password is generated automatically if not provided

  # Instance sizing
  engine_version    = var.engine_version
  instance_class    = var.instance_class
  allocated_storage = var.allocated_storage
  max_allocated_storage = var.max_allocated_storage

  # High Availability (enable for production)
  multi_az = var.multi_az

  # Backup and Maintenance
  backup_retention_period = var.backup_retention_period
  backup_window           = var.backup_window
  maintenance_window      = var.maintenance_window
  deletion_protection     = var.deletion_protection
  skip_final_snapshot     = var.skip_final_snapshot

  # Monitoring
  monitoring_interval                   = var.monitoring_interval
  performance_insights_enabled          = var.performance_insights_enabled
  performance_insights_retention_period = var.performance_insights_retention_period
  enabled_cloudwatch_logs_exports       = ["postgresql", "upgrade"]

  # Security
  storage_encrypted = true
  kms_key_id        = var.kms_key_id

  # DB Parameters
  db_parameters = var.db_parameters

  tags = var.additional_tags
}

# CloudWatch alarms for database monitoring
resource "aws_cloudwatch_metric_alarm" "database_cpu" {
  alarm_name          = "${var.environment}-hello-world-db-cpu-utilization"
  comparison_operator = "GreaterThanThreshold"
  evaluation_periods  = "2"
  metric_name         = "CPUUtilization"
  namespace           = "AWS/RDS"
  period              = "300"
  statistic           = "Average"
  threshold           = "80"
  alarm_description   = "This metric monitors database CPU utilization"
  alarm_actions       = var.alarm_sns_topic_arn != null ? [var.alarm_sns_topic_arn] : []

  dimensions = {
    DBInstanceIdentifier = module.hello_world_db.db_instance_id
  }

  tags = {
    Environment = var.environment
    Service     = "hello-world"
  }
}

resource "aws_cloudwatch_metric_alarm" "database_connections" {
  alarm_name          = "${var.environment}-hello-world-db-connections"
  comparison_operator = "GreaterThanThreshold"
  evaluation_periods  = "2"
  metric_name         = "DatabaseConnections"
  namespace           = "AWS/RDS"
  period              = "300"
  statistic           = "Average"
  threshold           = var.max_connections_threshold
  alarm_description   = "This metric monitors database connections"
  alarm_actions       = var.alarm_sns_topic_arn != null ? [var.alarm_sns_topic_arn] : []

  dimensions = {
    DBInstanceIdentifier = module.hello_world_db.db_instance_id
  }

  tags = {
    Environment = var.environment
    Service     = "hello-world"
  }
}
