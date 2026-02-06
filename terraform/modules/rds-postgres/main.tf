# RDS PostgreSQL module for service-owned infrastructure
# Teams can use this module to provision their own databases

terraform {
  required_version = ">= 1.5.0"
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
    random = {
      source  = "hashicorp/random"
      version = "~> 3.5"
    }
  }
}

# Generate random password if not provided
resource "random_password" "master_password" {
  count   = var.master_password == null ? 1 : 0
  length  = 32
  special = true
}

# Store database credentials in AWS Secrets Manager
resource "aws_secretsmanager_secret" "db_credentials" {
  name_prefix = "${var.name_prefix}-db-credentials-"
  description = "Database credentials for ${var.name_prefix}"
  kms_key_id  = var.kms_key_id

  tags = merge(var.tags, {
    Name        = "${var.name_prefix}-db-credentials"
    Environment = var.environment
  })
}

resource "aws_secretsmanager_secret_version" "db_credentials" {
  secret_id = aws_secretsmanager_secret.db_credentials.id
  secret_string = jsonencode({
    username    = var.master_username
    password    = var.master_password != null ? var.master_password : random_password.master_password[0].result
    engine      = "postgres"
    host        = aws_db_instance.main.address
    port        = aws_db_instance.main.port
    dbname      = aws_db_instance.main.db_name
    database_url = "postgres://${var.master_username}:${var.master_password != null ? var.master_password : random_password.master_password[0].result}@${aws_db_instance.main.endpoint}/${aws_db_instance.main.db_name}?sslmode=require"
  })
}

# DB Subnet Group
resource "aws_db_subnet_group" "main" {
  name_prefix = "${var.name_prefix}-"
  subnet_ids  = var.subnet_ids

  tags = merge(var.tags, {
    Name        = "${var.name_prefix}-db-subnet-group"
    Environment = var.environment
  })
}

# Security Group for RDS
resource "aws_security_group" "rds" {
  name_prefix = "${var.name_prefix}-rds-"
  description = "Security group for ${var.name_prefix} RDS instance"
  vpc_id      = var.vpc_id

  tags = merge(var.tags, {
    Name        = "${var.name_prefix}-rds-sg"
    Environment = var.environment
  })
}

# Allow inbound from application security groups
resource "aws_security_group_rule" "rds_ingress" {
  for_each = toset(var.allowed_security_group_ids)

  type                     = "ingress"
  from_port                = 5432
  to_port                  = 5432
  protocol                 = "tcp"
  security_group_id        = aws_security_group.rds.id
  source_security_group_id = each.value
  description              = "PostgreSQL access from ${each.value}"
}

# Restrict egress to VPC CIDR only
resource "aws_security_group_rule" "rds_egress_vpc" {
  type              = "egress"
  from_port         = 0
  to_port           = 0
  protocol          = "-1"
  security_group_id = aws_security_group.rds.id
  cidr_blocks       = [var.vpc_cidr_block]
  description       = "Allow egress to VPC CIDR only"
}

# RDS Parameter Group
resource "aws_db_parameter_group" "main" {
  name_prefix = "${var.name_prefix}-"
  family      = "postgres${var.engine_version}"
  description = "Custom parameter group for ${var.name_prefix}"

  dynamic "parameter" {
    for_each = var.db_parameters
    content {
      name         = parameter.value.name
      value        = parameter.value.value
      apply_method = lookup(parameter.value, "apply_method", "immediate")
    }
  }

  tags = merge(var.tags, {
    Name        = "${var.name_prefix}-db-parameter-group"
    Environment = var.environment
  })
}

# RDS Instance
resource "aws_db_instance" "main" {
  identifier_prefix = "${var.name_prefix}-"

  # Engine
  engine         = "postgres"
  engine_version = var.engine_version

  # Instance
  instance_class = var.instance_class

  # Storage
  allocated_storage     = var.allocated_storage
  max_allocated_storage = var.max_allocated_storage
  storage_type          = var.storage_type
  storage_encrypted     = var.storage_encrypted
  kms_key_id            = var.kms_key_id

  # Database
  db_name  = var.database_name
  username = var.master_username
  password = var.master_password != null ? var.master_password : random_password.master_password[0].result
  port     = 5432

  # IAM Authentication
  iam_database_authentication_enabled = var.iam_authentication_enabled

  # Network
  db_subnet_group_name   = aws_db_subnet_group.main.name
  vpc_security_group_ids = [aws_security_group.rds.id]
  publicly_accessible    = var.publicly_accessible

  # Backup
  backup_retention_period = var.backup_retention_period
  backup_window           = var.backup_window
  maintenance_window      = var.maintenance_window
  skip_final_snapshot     = var.skip_final_snapshot
  final_snapshot_identifier = var.skip_final_snapshot ? null : "${var.name_prefix}-final-snapshot-${formatdate("YYYY-MM-DD-hhmm", timestamp())}"

  # Monitoring
  enabled_cloudwatch_logs_exports = var.enabled_cloudwatch_logs_exports
  monitoring_interval             = var.monitoring_interval
  monitoring_role_arn             = var.monitoring_interval > 0 ? aws_iam_role.rds_monitoring[0].arn : null
  performance_insights_enabled    = var.performance_insights_enabled
  performance_insights_kms_key_id          = var.kms_key_id
  performance_insights_retention_period = var.performance_insights_retention_period

  # Parameters
  parameter_group_name = aws_db_parameter_group.main.name

  # Protection
  deletion_protection = var.deletion_protection

  # Multi-AZ
  multi_az = var.multi_az

  # Auto minor version upgrade
  auto_minor_version_upgrade = var.auto_minor_version_upgrade

  tags = merge(var.tags, {
    Name        = "${var.name_prefix}-rds"
    Environment = var.environment
  })
}

# IAM Role for Enhanced Monitoring
resource "aws_iam_role" "rds_monitoring" {
  count              = var.monitoring_interval > 0 ? 1 : 0
  name_prefix        = "${var.name_prefix}-rds-monitoring-"
  assume_role_policy = data.aws_iam_policy_document.rds_monitoring_assume[0].json

  tags = merge(var.tags, {
    Name        = "${var.name_prefix}-rds-monitoring-role"
    Environment = var.environment
  })
}

data "aws_iam_policy_document" "rds_monitoring_assume" {
  count = var.monitoring_interval > 0 ? 1 : 0

  statement {
    actions = ["sts:AssumeRole"]

    principals {
      type        = "Service"
      identifiers = ["monitoring.rds.amazonaws.com"]
    }
  }
}

resource "aws_iam_role_policy_attachment" "rds_monitoring" {
  count      = var.monitoring_interval > 0 ? 1 : 0
  role       = aws_iam_role.rds_monitoring[0].name
  policy_arn = "arn:aws:iam::aws:policy/service-role/AmazonRDSEnhancedMonitoringRole"
}
