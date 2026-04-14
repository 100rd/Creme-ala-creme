# Terraform Modules

The modules in this directory have been migrated to the central platform repository:
https://github.com/100rd/platform-design/tree/main/terraform/modules

## Migration Status

| Module | Status | Remote Source |
|--------|--------|---------------|
| `rds-postgres` | Migrated | `git::https://github.com/100rd/platform-design//terraform/modules/rds-postgres?ref=v0.1.0` |
| `k8s-operator` | Migrated | `git::https://github.com/100rd/platform-design//terraform/modules/k8s-operator?ref=v0.1.0` |

The local copies are kept here temporarily for reference. All new configurations
must reference the remote modules from platform-design pinned to a git tag.

## Adding a new module

1. Develop the module in `platform-design`
2. Tag a release: `git tag terraform/modules/<name>/v1.0.0`
3. Reference it from application configs: `git::https://github.com/100rd/platform-design//terraform/modules/<name>?ref=terraform/modules/<name>/v1.0.0`

Never add new modules to application repositories.
