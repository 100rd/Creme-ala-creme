# ADR-006: Terraform Module and State Strategy

**Status**: Accepted  
**Date**: 2026-04-14  
**Decision makers**: Solution Architect, Terraform Engineer

## Context

The Creme-ala-creme platform has two Terraform module categories:

1. **Reusable modules** (`rds-postgres`, `k8s-operator`) that encode infrastructure patterns
   applicable to any service in the organization.
2. **Service configurations** (`cloudflare-session-operator`, `hello-world-database`) that
   wire up modules with service-specific values and per-environment backends.

Previously, reusable modules lived locally in `terraform/modules/` inside this application
repository and were referenced via relative paths (`../../modules/rds-postgres`). This
approach does not scale: every application repo that needs the same RDS pattern must copy
the module, and bug fixes must be applied to every copy independently.

Additionally, the Terraform backend was configured with a single hardcoded state key for
all environments (`cloudflare-session-operator/terraform.tfstate`). Running `terraform plan`
in dev and prod operated against the same state file, creating a risk of cross-environment
state corruption.

## Decision 1: Modules in platform-design, pinned refs in app repos

All reusable Terraform modules are developed and maintained in the central `platform-design`
repository (`github.com/100rd/platform-design`). Application repositories reference these
modules via git source URLs pinned to a release tag:

```hcl
module "k8s_operator" {
  source = "git::https://github.com/100rd/platform-design//terraform/modules/k8s-operator?ref=v0.1.0"
}

module "hello_world_db" {
  source = "git::https://github.com/100rd/platform-design//terraform/modules/rds-postgres?ref=v0.1.0"
}
```

### Rationale

- **Single source of truth**: One codebase for each module. Security patches and best-practice
  improvements are applied once and propagated by bumping the ref in consumer repos.
- **Versioned contracts**: Pinning `?ref=v0.1.0` guarantees that consumers are not broken by
  upstream changes. Upgrades are explicit and reviewable via PR diffs.
- **Consistency**: Every service that provisions an RDS instance or K8s namespace uses the same
  hardened pattern, reducing configuration drift.
- **Clear ownership**: The platform team owns the module lifecycle (develop, test, tag). Service
  teams own the configuration (which module version, what variable values).
- **Review efficiency**: Module changes go through a single review process rather than being
  scattered across application repos.

### Alternatives considered

| Alternative | Verdict | Reason |
|-------------|---------|--------|
| Local modules in each app repo | Rejected | Duplication, no central fixes, drift between copies |
| Terraform Registry (private) | Deferred | Adds registry infrastructure; git refs are sufficient for our scale |
| Git submodules | Rejected | Complex merge workflows, version pinning less explicit |
| Mono-repo (modules + configs together) | Rejected | Couples module releases to application deploys |

## Decision 2: Separate state per environment

Each environment (dev, stage, prod) uses a separate Terraform state file within the same
S3 bucket. The backend block in `versions.tf` is a partial config (`backend "s3" {}`), and
the actual bucket, key, region, and lock table are supplied via per-environment backend
config files at `terraform init` time:

```bash
terraform init -backend-config=dev/backend.hcl
terraform init -backend-config=prod/backend.hcl
```

State key structure:

```
s3://creme-terraform-state/
├── cloudflare-session-operator/
│   ├── dev/terraform.tfstate
│   ├── stage/terraform.tfstate
│   └── prod/terraform.tfstate
└── hello-world/
    ├── dev/terraform.tfstate
    └── prod/terraform.tfstate
```

### Rationale

- **Blast radius isolation**: A `terraform apply` in dev cannot read, modify, or corrupt
  prod state. Each environment is an independent state file with its own lock.
- **Independent lifecycle**: Dev can be freely destroyed and recreated without affecting
  stage or prod. Environment promotion is an explicit process, not an accident.
- **CI/CD safety**: Pipeline jobs for each environment use different backend configs,
  preventing cross-environment operations.
- **Audit clarity**: State history in S3 versioning is per-environment, making it easy to
  identify what changed where and when.

### Alternatives considered

| Alternative | Verdict | Reason |
|-------------|---------|--------|
| Single state file for all envs | Rejected | Cross-env corruption risk, cannot destroy dev independently |
| Terraform workspaces | Rejected | Workspaces share the same backend config; switching is error-prone; state key naming is implicit rather than explicit |
| Separate S3 buckets per env | Acceptable | More isolation but adds operational overhead for bucket management; single bucket with separate keys is sufficient |

## Consequences

### Positive

- Application repos become thin configuration layers: `main.tf` + `variables.tf` +
  `outputs.tf` + `versions.tf` + per-env `backend.hcl` and `terraform.tfvars`.
- Module upgrades are explicit PRs that show exactly what changed in the ref.
- No risk of cross-environment state corruption.
- CI/CD pipelines can safely run plan for all environments in parallel.

### Negative

- `terraform init` requires network access to clone the platform-design repo. Offline
  development needs a local module override (via `TF_MODULE_SOURCES` or `.terraformrc`).
- Switching environments requires `terraform init -reconfigure`, which is slower than
  workspace switching. This is an acceptable trade-off for the safety gain.

### Migration

Local module copies in `terraform/modules/` are retained temporarily for reference. All
configurations have been updated to use remote sources. The local copies should be removed
once all teams have verified the migration.
