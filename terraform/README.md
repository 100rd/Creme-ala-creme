# Terraform - Service-Owned Infrastructure

This directory contains Terraform modules and configurations for service-owned infrastructure. Teams can provision and manage their own infrastructure resources following the platform's standards.

## Philosophy

**Service-Owned Infrastructure**: Each service team owns and manages their infrastructure through Terraform. This approach provides:

- Team autonomy to provision resources when needed
- Infrastructure as Code for all resources
- Consistent security and compliance through reusable modules
- Clear ownership and cost attribution

## Directory Structure

```
terraform/
├── modules/                    # Reusable Terraform modules
│   └── rds-postgres/          # RDS PostgreSQL module
│       ├── main.tf
│       ├── variables.tf
│       └── outputs.tf
├── configurations/            # Service-specific configurations
│   ├── hello-world-database/ # Database for hello-world service
│   │   ├── main.tf
│   │   ├── variables.tf
│   │   ├── outputs.tf
│   │   ├── dev.tfvars
│   │   └── prod.tfvars
│   └── cloudflare-session-operator/  # Existing operator config
└── README.md
```

## Quick Start

### Prerequisites

- Terraform >= 1.5.0
- AWS CLI configured with appropriate credentials
- S3 bucket for Terraform state (recommended)
- DynamoDB table for state locking (recommended)

### Provisioning a Database

1. **Navigate to configuration directory:**
```bash
cd terraform/configurations/hello-world-database
```

2. **Initialize Terraform:**
```bash
terraform init \
  -backend-config="bucket=my-terraform-state" \
  -backend-config="key=hello-world/database/terraform.tfstate" \
  -backend-config="region=us-east-1" \
  -backend-config="dynamodb_table=terraform-locks"
```

3. **Update variables:**
Edit `dev.tfvars` or `prod.tfvars` with your environment-specific values:
```hcl
vpc_id       = "vpc-abc123"
cluster_name = "my-eks-cluster"
```

4. **Plan and apply:**
```bash
# Development
terraform plan -var-file=dev.tfvars -out=dev.tfplan
terraform apply dev.tfplan

# Production
terraform plan -var-file=prod.tfvars -out=prod.tfplan
terraform apply prod.tfplan
```

5. **Retrieve connection details:**
```bash
# Get the Secrets Manager ARN
terraform output secret_arn

# Retrieve credentials from AWS Secrets Manager
aws secretsmanager get-secret-value \
  --secret-id $(terraform output -raw secret_name) \
  --query SecretString \
  --output text | jq .
```

## RDS PostgreSQL Module

### Features

- **Automated password generation** if not provided
- **Secrets Manager integration** for credential storage
- **Security hardening**:
  - Encryption at rest (KMS)
  - Encryption in transit (SSL required)
  - Network isolation (VPC, security groups)
- **High availability** (Multi-AZ support)
- **Monitoring**:
  - Enhanced monitoring
  - Performance Insights
  - CloudWatch log exports
  - CloudWatch alarms
- **Backup and recovery**:
  - Automated backups
  - Point-in-time recovery
  - Final snapshot protection

### Usage Example

```hcl
module "my_database" {
  source = "../../modules/rds-postgres"

  name_prefix = "my-service-prod"
  environment = "prod"

  # Network
  vpc_id                     = "vpc-abc123"
  subnet_ids                 = ["subnet-1", "subnet-2", "subnet-3"]
  allowed_security_group_ids = ["sg-eks-pods"]

  # Database
  database_name   = "mydb"
  master_username = "dbadmin"

  # Sizing
  instance_class        = "db.r6g.large"
  allocated_storage     = 100
  max_allocated_storage = 500

  # High Availability
  multi_az = true

  # Security
  deletion_protection = true
  storage_encrypted   = true
}
```

### Input Variables

See [modules/rds-postgres/variables.tf](modules/rds-postgres/variables.tf) for complete list.

Key variables:
- `name_prefix` - Prefix for all resource names
- `environment` - Environment name (dev, stage, prod)
- `vpc_id` - VPC where database will be created
- `subnet_ids` - Subnets for DB subnet group
- `instance_class` - RDS instance type
- `multi_az` - Enable Multi-AZ deployment
- `deletion_protection` - Protect against accidental deletion

### Outputs

- `db_instance_endpoint` - Full connection endpoint
- `db_instance_address` - Database hostname
- `db_instance_port` - Database port (5432)
- `secret_arn` - ARN of Secrets Manager secret
- `secret_name` - Name of Secrets Manager secret

## Integration with Kubernetes

### External Secrets Operator

Use External Secrets Operator to synchronize database credentials from AWS Secrets Manager to Kubernetes secrets:

```yaml
apiVersion: external-secrets.io/v1beta1
kind: SecretStore
metadata:
  name: aws-secretsmanager
  namespace: hello-world
spec:
  provider:
    aws:
      service: SecretsManager
      region: us-east-1
      auth:
        jwt:
          serviceAccountRef:
            name: hello-world
---
apiVersion: external-secrets.io/v1beta1
kind: ExternalSecret
metadata:
  name: hello-world-db
  namespace: hello-world
spec:
  refreshInterval: 1h
  secretStoreRef:
    name: aws-secretsmanager
    kind: SecretStore
  target:
    name: hello-world-db
    creationPolicy: Owner
  data:
    - secretKey: url
      remoteRef:
        key: hello-world-prod-db-credentials
        property: database_url
```

### Helm Values

Enable External Secrets in your Helm values:

```yaml
externalSecret:
  enabled: true
  secretStoreRef:
    name: "aws-secretsmanager"
    kind: "SecretStore"
  secretPath: "hello-world-prod-db-credentials"
  refreshInterval: "1h"
```

## Cost Optimization

### Development Environments

- Use `db.t3.micro` or `db.t3.small` instances
- Disable Multi-AZ (`multi_az = false`)
- Reduce backup retention (3-7 days)
- Consider stopping databases during off-hours

### Production Environments

- Right-size instances based on actual usage
- Enable storage autoscaling
- Use Reserved Instances for predictable workloads
- Monitor Performance Insights to optimize queries

## Security Best Practices

1. **Never commit secrets** to version control
   - Use AWS Secrets Manager
   - Let Terraform generate passwords
   - Rotate credentials regularly

2. **Network isolation**
   - Place databases in private subnets
   - Use security groups to restrict access
   - Never make databases publicly accessible

3. **Encryption**
   - Enable encryption at rest
   - Enforce SSL/TLS for connections
   - Use KMS customer-managed keys for sensitive data

4. **Monitoring and auditing**
   - Enable CloudWatch logging
   - Set up alerts for anomalous activity
   - Review Performance Insights regularly

5. **Backup and disaster recovery**
   - Enable automated backups
   - Test restore procedures
   - Use Multi-AZ for production
   - Consider cross-region replicas for critical data

## Troubleshooting

### Connection Issues

1. **Verify security group rules:**
```bash
aws ec2 describe-security-groups \
  --group-ids $(terraform output -raw security_group_id)
```

2. **Check subnet routing:**
```bash
aws ec2 describe-route-tables \
  --filters "Name=association.subnet-id,Values=subnet-xxx"
```

3. **Test from EKS pod:**
```bash
kubectl run -it --rm psql-test \
  --image=postgres:16 \
  --restart=Never \
  -- psql "$DATABASE_URL"
```

### State Management

If you need to import existing resources:
```bash
terraform import module.hello_world_db.aws_db_instance.main my-db-identifier
```

## Maintenance

### Upgrading PostgreSQL Version

1. Review [PostgreSQL release notes](https://www.postgresql.org/docs/release/)
2. Test upgrade in development environment
3. Plan maintenance window
4. Update `engine_version` variable
5. Apply with `terraform apply`

### Modifying Instance Class

```bash
# Scale up
terraform apply -var="instance_class=db.r6g.xlarge"

# RDS will perform rolling upgrade with minimal downtime
```

## Contributing

When adding new modules:

1. Follow existing module structure
2. Include comprehensive variable descriptions
3. Add examples to documentation
4. Test in dev environment first
5. Update this README

## Support

For questions or issues:
- Check module documentation
- Review Terraform output messages
- Consult AWS RDS documentation
- Contact platform team
