provider "aws" {
  region = var.aws_region

  default_tags {
    tags = {
      Application = var.app_name
      ManagedBy   = "terraform"
    }
  }
}

############################
# S3 bucket for the app
############################
module "s3_bucket" {
  source  = "terraform-aws-modules/s3-bucket/aws"
  version = "~> 4.1"

  bucket = var.app_name

  block_public_acls       = true
  block_public_policy     = true
  ignore_public_acls      = true
  restrict_public_buckets = true

  server_side_encryption_configuration = {
    rule = {
      apply_server_side_encryption_by_default = {
        sse_algorithm = "aws:kms"
      }
    }
  }

  versioning = {
    enabled = true
  }

  lifecycle_rule = [{
    id      = "expire-noncurrent"
    enabled = true
    noncurrent_version_expiration = {
      days = 30
    }
  }]
}

############################
# PostgreSQL RDS instance
############################
resource "random_password" "db_master" {
  length  = 20
  special = false
}

module "db" {
  source  = "terraform-aws-modules/rds/aws"
  version = "~> 6.5"

  identifier = "${var.app_name}-postgres"

  engine               = "postgres"
  engine_version       = var.db_engine_version
  instance_class       = var.db_instance_class
  allocated_storage    = var.db_allocated_storage
  max_allocated_storage = var.db_max_allocated_storage

  db_name  = "sessions"
  username = "app"
  password = coalesce(var.db_master_password, random_password.db_master.result)

  create_db_subnet_group = true
  subnet_ids             = var.private_subnet_ids
  vpc_security_group_ids = var.db_security_group_ids

  publicly_accessible = false
  multi_az            = var.db_multi_az

  storage_encrypted = true

  backup_retention_period = var.db_backup_retention
  deletion_protection     = var.db_deletion_protection
  skip_final_snapshot     = var.db_skip_final_snapshot

  performance_insights_enabled = var.db_performance_insights_enabled

  tags = {
    Name = "${var.app_name}-rds"
  }
}
