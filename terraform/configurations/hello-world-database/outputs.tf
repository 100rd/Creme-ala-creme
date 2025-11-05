output "db_endpoint" {
  description = "Database connection endpoint"
  value       = module.hello_world_db.db_instance_endpoint
}

output "db_address" {
  description = "Database address"
  value       = module.hello_world_db.db_instance_address
}

output "db_port" {
  description = "Database port"
  value       = module.hello_world_db.db_instance_port
}

output "db_name" {
  description = "Database name"
  value       = module.hello_world_db.db_name
}

output "secret_arn" {
  description = "ARN of AWS Secrets Manager secret containing database credentials"
  value       = module.hello_world_db.secret_arn
}

output "secret_name" {
  description = "Name of AWS Secrets Manager secret"
  value       = module.hello_world_db.secret_name
}

output "security_group_id" {
  description = "Security group ID for database"
  value       = module.hello_world_db.db_security_group_id
}

output "connection_string_example" {
  description = "Example connection string (retrieve password from Secrets Manager)"
  value       = "postgres://${module.hello_world_db.db_master_username}:<PASSWORD>@${module.hello_world_db.db_instance_endpoint}/${module.hello_world_db.db_name}?sslmode=require"
}
