# Cloudflare Session Operator -- System Design Document

**Status**: Approved  
**Author**: Solution Architect  
**Date**: 2026-04-14  
**Scope**: Best-practices gap analysis and improvement plan for the cloudflare-session-operator

---

## 1. Current State Assessment

### 1.1 What Exists Today

The cloudflare-session-operator is a Kubernetes controller built with controller-runtime that
manages `SessionBinding` custom resources. When a `SessionBinding` CR is created, the operator:

1. Validates the Cloudflare session via the Cloudflare API.
2. Clones a pod from a target Deployment template and labels it with the session ID.
3. Configures a Cloudflare route pointing to the pod endpoint.
4. Manages lifecycle through finalizers (cleanup on deletion).

The operator code lives in `cloudflare-session-operator/` and consists of:

| File | Purpose |
|------|---------|
| `main.go` | Manager bootstrap, health probes, leader election |
| `controllers/sessionbinding_controller.go` | Reconciliation loop, pod management, status conditions |
| `api/v1alpha1/sessionbinding_types.go` | CRD types with status conditions |
| `api/v1alpha1/cloudflareoperatorconfig_types.go` | Cluster-scoped operator configuration CRD |
| `api/v1alpha1/groupversion_info.go` | Scheme registration |
| `pkg/cloudflare/client.go` | Cloudflare API client (currently stubbed) |
| `config/crd/bases/` | CRD YAMLs |
| `config/samples/` | Example CR manifests per environment |
| `deploy/argocd/` | ArgoCD Application manifests for GitOps |
| `DEPLOYMENT_BLOCKED.md` | Known blockers |

### 1.2 What Is Good

- **Clean controller-runtime usage**: Proper `SetupWithManager`, owner references via
  `SetControllerReference`, finalizer pattern.
- **Status conditions**: Three well-defined conditions (`SessionDiscovered`, `PodReady`,
  `RouteConfigured`) following the Kubernetes API conventions.
- **Clock abstraction**: Testable time via `Clock` interface.
- **Event recording**: Kubernetes events emitted for pod creation and cleanup.
- **Leader election**: Supported via flag.
- **Health probes**: Both `/healthz` and `/readyz` registered on the manager.
- **Interface-based Cloudflare client**: `cloudflare.Client` interface enables mocking.

### 1.3 What Is Missing (Critical Gaps)

| Gap | Severity | Reference |
|-----|----------|-----------|
| Cloudflare API is 100% stubbed | CRITICAL | `pkg/cloudflare/client.go` |
| No Dockerfile | CRITICAL | Cannot build or deploy |
| No RBAC manifests (ServiceAccount, ClusterRole, etc.) | CRITICAL | No Helm chart exists |
| No Helm chart at all | CRITICAL | hello-world has a full chart |
| No namespace scoping | HIGH | Watches all namespaces |
| TTL enforcement not implemented | HIGH | `ttlSeconds` field ignored |
| No input validation (sessionID regex) | HIGH | Direct string concat in pod names |
| Silent credential degradation | HIGH | Missing creds silently accepted |
| No structured logging (zerolog) | MEDIUM | Uses `stdr` (stdlib adapter) |
| No distributed tracing (OTEL) | MEDIUM | hello-world has full OTEL |
| No Prometheus metrics beyond defaults | MEDIUM | No custom business metrics |
| No External Secrets integration | MEDIUM | hello-world has ESO |
| No CI pipeline gates | MEDIUM | Not in `.github/workflows/ci.yml` |
| No unit tests | MEDIUM | No `_test.go` files |
| No envtest integration tests | MEDIUM | Controller-runtime envtest absent |
| No feature flags | LOW | hello-world has OpenFeature |

---

## 2. Operator SDK Best Practices Analysis

### 2.1 Controller-Runtime Patterns

**Current**: The operator follows the basic pattern but misses several production patterns.

**Required improvements**:

1. **Reconciliation idempotency**: The current `ensureSessionPod` is idempotent (check-then-create).
   Good. But `patchStatus` uses a full Update instead of a strategic merge patch, risking conflicts
   under high load. Switch to `client.MergeFrom` patch.

2. **Finalizer handling**: Correct pattern used. No changes needed.

3. **Owner references**: Correctly set via `SetControllerReference`. Good.

4. **Status conditions**: Uses `meta.SetStatusCondition` from apimachinery. Correct.

5. **MaxConcurrentReconciles**: Set to 1. For production, consider increasing to 3-5 with proper
   rate limiting via `controller.Options.RateLimiter`.

6. **Predicate filters**: Missing. Add `GenerationChangedPredicate` to skip status-only updates.

7. **Namespace scoping**: Manager cache should be scoped to operator namespace(s).

### 2.2 CRD Design

**Improvements needed**:

- Add `+kubebuilder:printcolumn` markers for `kubectl get sessionbindings` output.
- Add validation markers: `+kubebuilder:validation:Pattern`, `+kubebuilder:validation:MaxLength`.
- Add `+kubebuilder:resource:shortName=sb` for convenience.
- Add `additionalPrinterColumns` for phase, session-id, bound-pod, age.

### 2.3 Error Handling

- Distinguish transient vs permanent errors (requeue vs don't requeue).
- Add exponential backoff for Cloudflare API failures.
- Emit warning events for permanent errors.

---

## 3. Feature Flags in Operator Context

### 3.1 Strategy

Unlike the hello-world HTTP service where flags gate per-request behavior, an operator's feature
flags gate reconciliation behavior. Two patterns apply:

1. **Environment-based gates** (simple, restart-on-change): Control coarse features like
   "enable TTL enforcement", "enable dry-run mode", "enable Cloudflare API calls".

2. **OpenFeature/flagd** (dynamic, no restart): Control fine-grained behavior like
   "max concurrent sessions per namespace", "enable route caching".

### 3.2 Recommended Flags

| Flag | Type | Default | Purpose |
|------|------|---------|---------|
| `ENABLE_CLOUDFLARE_API` | env | `true` | Gate actual Cloudflare calls (false = dry-run) |
| `ENABLE_TTL_ENFORCEMENT` | env | `true` | Gate TTL reaper goroutine |
| `MAX_SESSIONS_PER_NAMESPACE` | env/flagd | `100` | Safety limit on session pods |
| `ENABLE_METRICS` | env | `true` | Gate custom metrics recording |
| `ENABLE_TRACING` | env | `false` | Gate OTEL span creation in reconciler |

### 3.3 Implementation

Add a `pkg/flags/` package mirroring hello-world's pattern but simpler (no admin endpoints).
Use environment variables as defaults with optional flagd override for dynamic flags.

---

## 4. Structured Logging and Distributed Tracing

### 4.1 Logging (zerolog)

Replace `stdr.New(os.Stdout)` with a zerolog-based `logr.Logger` adapter. Use
`github.com/go-logr/zerologr` to bridge controller-runtime's logr interface with zerolog.

Key fields to include in every log line:
- `controller`: "sessionbinding"
- `namespace`: from request
- `name`: from request
- `sessionID`: from spec
- `reconcileID`: from context (controller-runtime injects this)

### 4.2 Tracing (OTEL)

Add OpenTelemetry tracing to the reconciliation loop:
- Span per `Reconcile()` call
- Child spans for `EnsureSession`, `ensureSessionPod`, `EnsureRoute`
- Propagate trace context to Cloudflare API calls
- Gate behind `ENABLE_TRACING` flag

Mirror hello-world's `initTracer()` pattern with OTLP HTTP exporter.

---

## 5. Metrics

### 5.1 Controller-Runtime Built-in Metrics

controller-runtime exposes these automatically on `:8080/metrics`:
- `controller_runtime_reconcile_total`
- `controller_runtime_reconcile_errors_total`
- `controller_runtime_reconcile_time_seconds`
- `workqueue_*` metrics

### 5.2 Custom Business Metrics

Add these custom metrics:

| Metric | Type | Labels | Purpose |
|--------|------|--------|---------|
| `sessionbinding_active_total` | Gauge | `namespace` | Count of active bindings |
| `sessionbinding_phase` | GaugeVec | `namespace`, `phase` | Count by phase |
| `cloudflare_api_requests_total` | Counter | `operation`, `status` | API call tracking |
| `cloudflare_api_latency_seconds` | Histogram | `operation` | API latency |
| `session_pod_creation_total` | Counter | `namespace`, `status` | Pod creation tracking |

### 5.3 Exposure

Metrics bind address is already `:8080` via `--metrics-bind-address` flag.
The Helm chart ServiceMonitor will scrape this.

---

## 6. Secrets Management

### 6.1 Current State

Cloudflare credentials (`CLOUDFLARE_ACCOUNT_ID`, `CLOUDFLARE_API_TOKEN`) are read from
environment variables in `NewClientFromEnv()`. Missing credentials are silently accepted.

### 6.2 Target State

Mirror hello-world's External Secrets Operator (ESO) pattern:

1. **ExternalSecret CR** in Helm chart syncs credentials from AWS Secrets Manager to a
   Kubernetes Secret.
2. **Deployment** mounts the Secret as env vars.
3. **Startup validation**: Fail fast if credentials are missing when `ENABLE_CLOUDFLARE_API=true`.

### 6.3 Secret Path Convention

```
cloudflare-session-operator/{environment}/cloudflare-api
```

Containing:
```json
{
  "account_id": "...",
  "api_token": "..."
}
```

---

## 7. Helm Chart Design

### 7.1 Parity with hello-world

The operator Helm chart must include all the templates that hello-world has, adapted for an
operator deployment:

| Template | hello-world | operator (new) | Notes |
|----------|-------------|----------------|-------|
| `_helpers.tpl` | yes | yes | Standard helpers |
| `deployment.yaml` | yes | yes | Operator deployment |
| `serviceaccount.yaml` | yes | yes | With IRSA annotation |
| `rbac.yaml` | Role | ClusterRole | Operator needs cluster-wide CRD access |
| `hpa.yaml` | yes | yes | Target CPU 70% |
| `pdb.yaml` | yes | yes | minAvailable: 1 |
| `networkpolicy.yaml` | yes | yes | Deny-all + allow metrics/webhook |
| `servicemonitor.yaml` | yes | yes | Scrape /metrics |
| `externalsecret.yaml` | yes | yes | Cloudflare API creds |
| `service.yaml` | yes | yes | Metrics port exposure |
| `resourcequota.yaml` | yes | no | Operator doesn't need per-ns quota |

### 7.2 Operator-Specific Additions

- **ClusterRole**: Operator needs `sessionbindings`, `pods`, `deployments`, `events` access.
- **ClusterRoleBinding**: Bind ClusterRole to operator ServiceAccount.
- **Security context**: Non-root (65532), read-only rootFS, drop ALL capabilities, seccomp RuntimeDefault.
- **Probes**: `/healthz` for liveness, `/readyz` for readiness (already implemented in main.go).
- **Leader election RBAC**: Needs `leases` access in operator namespace.

---

## 8. Terraform IRSA Module

### 8.1 Purpose

Provision Kubernetes resources for the operator via Terraform:
- Namespace with Pod Security Standards labels
- ServiceAccount with IRSA annotation
- RBAC (ClusterRole + ClusterRoleBinding)
- ResourceQuota and LimitRange

### 8.2 Module Interface

```hcl
module "k8s_operator" {
  source = "git::https://github.com/100rd/platform-design//terraform/modules/k8s-operator?ref=v0.1.0"

  operator_name      = "cloudflare-session-operator"
  namespace          = "cloudflare-system"
  iam_role_arn       = module.irsa.iam_role_arn  # From IRSA module
  pod_security_level = "restricted"
  
  resource_quota = {
    requests_cpu    = "1"
    requests_memory = "512Mi"
    limits_cpu      = "2"
    limits_memory   = "1Gi"
    pods            = "10"
  }
}
```

### 8.3 Separation of Concerns

Terraform provisions **infrastructure only**: AWS resources (RDS, S3), Kubernetes namespace,
RBAC, IRSA, quotas. Application-level runtime configuration (Kafka brokers, topic names,
feature flags, reconciliation tuning) is managed **inside the cluster** via the
`CloudflareOperatorConfig` CRD (see section 9).

This separation was enforced by removing all app-config variables (`kafka_*`, `db_name`,
`db_username`) from the Terraform configuration. The Kafka provider and topic resources
were also removed -- Kafka topic management belongs to the data platform team or a dedicated
Kafka operator, not the application Terraform configuration.

### 8.4 IRSA

The operator needs an IAM role for:
- Secrets Manager read access (for ESO to pull Cloudflare creds)
- S3 access (for the operator's S3 bucket)

The IRSA annotation on the ServiceAccount enables pod-level IAM without node-level credentials.

---

## 9. In-Cluster Configuration

### 9.1 Why Not Terraform for App Config

Terraform's purpose is infrastructure provisioning: creating AWS resources, Kubernetes
namespaces, RBAC bindings, and IAM roles. Application-level runtime configuration -- such as
Kafka broker addresses, topic names, feature flags, and reconciliation tuning -- does not
belong in Terraform for several reasons:

1. **Lifecycle mismatch**: Infrastructure changes require `plan` + `apply` with state locking.
   Changing a Kafka topic name should not require a Terraform run.
2. **Blast radius**: A Terraform apply touches all resources in the configuration. Changing a
   feature flag should not risk modifying RDS or S3 resources.
3. **Operational friction**: Developers who need to toggle dry-run mode or adjust reconciliation
   concurrency should not need Terraform access or state file permissions.
4. **GitOps incompatibility**: Terraform state is not a Kubernetes-native concept. ArgoCD cannot
   manage Terraform variables, but it can manage CRs natively.

### 9.2 The CRD-Based Config Pattern

The operator uses a cluster-scoped Custom Resource (`CloudflareOperatorConfig`) as its single
source of truth for runtime behavior. By convention, exactly one instance named `default`
exists in the cluster.

```yaml
apiVersion: cloudflare.example.com/v1alpha1
kind: CloudflareOperatorConfig
metadata:
  name: default
spec:
  kafka:
    bootstrapServers: "b-1.msk-prod.example:9094,b-2.msk-prod.example:9094"
    tlsEnabled: true
    topics:
      sessions: "sessions"
      ids: "id"
  features:
    tracingEnabled: true
    metricsEnabled: true
    cloudflareAPIEnabled: true
    dryRunMode: false
  reconciliation:
    requeueDuration: 30s
    maxConcurrentReconciles: 3
```

The operator watches this CR and reloads configuration dynamically without requiring a pod
restart. Status conditions on the CR reflect whether the configuration was successfully
applied, providing clear observability into config state.

### 9.3 How Configuration Flows

```
Git repo (config/samples/)
       |
       v
  ArgoCD Application  --sync-->  CloudflareOperatorConfig CR
       |                                    |
       |                         Operator watches via informer
       |                                    |
       v                                    v
  Git history provides          Operator reloads config,
  audit trail + rollback        updates status.observedGeneration
```

**GitOps path (standard)**: Config YAML lives in `config/samples/`. ArgoCD watches the repo
and applies changes to the cluster. The operator detects the CR update via its informer and
reloads.

**Direct path (ad-hoc)**: For urgent changes, operators can `kubectl apply` the CR directly.
ArgoCD self-heal will eventually reconcile it back to the Git state, so any direct change
must also be committed to Git.

### 9.4 ArgoCD Integration

An ArgoCD Application manifest (`deploy/argocd/operator-config-app.yaml`) manages the
`CloudflareOperatorConfig` CR via GitOps:

- **Source**: `cloudflare-session-operator/config/samples/` in Git
- **Destination**: `cloudflare-system` namespace in the target cluster
- **Sync policy**: Automated with prune and self-heal enabled

This means changing operator configuration is a standard Git workflow: edit the YAML, open a
PR, merge, and ArgoCD applies it.

### 9.5 Environment-Specific Configuration

Each environment has its own config sample:

| File | Environment | Key differences |
|------|-------------|-----------------|
| `config_default_dev.yaml` | Development | Local Kafka, dry-run enabled, low concurrency |
| `config_default_prod.yaml` | Production | MSK brokers with TLS, live API, higher concurrency |

ArgoCD selects the correct file per-cluster via its Application `path` or overlay mechanism.

### 9.6 Why Not Consul, flagd, or ConfigMap

| Approach | Verdict | Reason |
|----------|---------|--------|
| **CRD (chosen)** | Best fit | Zero new dependencies; operator already watches CRDs; ArgoCD-native; typed schema with validation; status feedback; `kubectl get cfoc` visibility |
| **ConfigMap** | Acceptable but weaker | No schema validation; no status feedback; no printer columns; untyped string data; still Kubernetes-native |
| **Consul** | Rejected | Requires deploying and operating a Consul cluster; overkill for single-operator config; adds network dependency; not Kubernetes-native |
| **flagd/OpenFeature** | Complementary | Good for dynamic per-request feature flags in HTTP services; the operator's config is structural (broker addresses, concurrency) not per-request; flagd can be added later for fine-grained flags |

The CRD pattern is the standard Kubernetes operator approach. The operator already has a
controller-runtime manager watching CRDs, so adding a config watch is a natural extension
with zero new infrastructure.

---

## 10. CI/CD Pipeline Parity

### 10.1 Current State

The CI pipeline (`ci.yml`) only covers `hello-world/`. The operator has zero CI coverage.

### 10.2 Required Pipeline Additions

Add a new job `operator-lint-build-test` mirroring `lint-build-test` but for the operator:

1. `golangci-lint` on `cloudflare-session-operator/`
2. `go vet ./...`
3. `gofmt -s -l .`
4. `gosec ./...`
5. `govulncheck ./...`
6. `go test ./...` (unit + envtest)
7. Trivy filesystem scan
8. Docker build
9. Trivy image scan
10. cosign signing (on main branch push)

### 10.3 Helm Chart Validation

Add `helm lint` for the operator chart in CI:
```yaml
- name: Lint operator Helm chart
  run: helm lint cloudflare-session-operator/helm/cloudflare-session-operator/
```

---

## 11. Testing Strategy

### 11.1 Unit Tests

- `controllers/sessionbinding_controller_test.go`: Test reconciliation logic with fake client.
- `pkg/cloudflare/client_test.go`: Test API client with HTTP test server.
- `pkg/flags/flags_test.go`: Test feature flag evaluation.

### 11.2 Integration Tests (envtest)

Use controller-runtime's `envtest` package to spin up a real API server:
- Test full reconciliation cycle: create binding -> verify pod created -> verify status.
- Test deletion with finalizer cleanup.
- Test error scenarios: missing deployment, Cloudflare API errors.
- Test TTL enforcement.

### 11.3 End-to-End Tests

- Deploy operator to a kind cluster.
- Create `SessionBinding` CRs and verify pod creation.
- Verify Cloudflare API calls (with mock server).
- Verify metrics and health endpoints.

---

## 12. Implementation Plan

### Phase 1: Foundation (Files to create/modify)

| Action | File | Description |
|--------|------|-------------|
| Create | `helm/cloudflare-session-operator/Chart.yaml` | Helm chart metadata |
| Create | `helm/cloudflare-session-operator/values.yaml` | Default values |
| Create | `helm/cloudflare-session-operator/templates/_helpers.tpl` | Template helpers |
| Create | `helm/cloudflare-session-operator/templates/deployment.yaml` | Operator deployment |
| Create | `helm/cloudflare-session-operator/templates/serviceaccount.yaml` | SA with IRSA |
| Create | `helm/cloudflare-session-operator/templates/rbac.yaml` | ClusterRole + binding |
| Create | `helm/cloudflare-session-operator/templates/service.yaml` | Metrics service |
| Create | `helm/cloudflare-session-operator/templates/hpa.yaml` | Autoscaler |
| Create | `helm/cloudflare-session-operator/templates/pdb.yaml` | Disruption budget |
| Create | `helm/cloudflare-session-operator/templates/networkpolicy.yaml` | Network rules |
| Create | `helm/cloudflare-session-operator/templates/servicemonitor.yaml` | Prometheus scrape |
| Create | `helm/cloudflare-session-operator/templates/externalsecret.yaml` | ESO for CF creds |

### Phase 2: Terraform Module

| Action | File | Description |
|--------|------|-------------|
| Create | `terraform/modules/k8s-operator/main.tf` | Namespace, RBAC, quotas |
| Create | `terraform/modules/k8s-operator/variables.tf` | Module inputs |
| Create | `terraform/modules/k8s-operator/outputs.tf` | Module outputs |
| Create | `terraform/modules/k8s-operator/README.md` | Module documentation |
| Update | `terraform/configurations/cloudflare-session-operator/dev/terraform.tfvars` | Infra-only vars |
| Update | `terraform/configurations/cloudflare-session-operator/stage/terraform.tfvars` | Infra-only vars |
| Update | `terraform/configurations/cloudflare-session-operator/prod/terraform.tfvars` | Infra-only vars |

### Phase 3: In-Cluster Configuration

| Action | File | Description |
|--------|------|-------------|
| Create | `api/v1alpha1/cloudflareoperatorconfig_types.go` | Config CRD Go types |
| Create | `config/crd/bases/cloudflare.example.com_cloudflareoperatorconfigs.yaml` | Config CRD manifest |
| Create | `config/samples/config_default_dev.yaml` | Dev config CR |
| Create | `config/samples/config_default_prod.yaml` | Prod config CR |
| Create | `deploy/argocd/operator-config-app.yaml` | ArgoCD Application |

### Phase 4: CI/CD Updates

| Action | File | Description |
|--------|------|-------------|
| Update | `.github/workflows/ci.yml` | Add operator job |

---

## 13. Risk Assessment

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|------------|
| Cloudflare API integration complexity | Medium | High | Start with dry-run mode, feature-flag API calls |
| RBAC over-permissioning | Low | High | Audit with `kubectl auth can-i`, principle of least privilege |
| Pod sprawl from operator | Medium | High | ResourceQuota, MaxSessionsPerNamespace flag |
| Breaking existing Terraform state | Low | Medium | New module only, no changes to existing resources |
| Config CR not found at startup | Low | Medium | Operator falls back to sane defaults, logs warning |

---

## Appendix: Reference Architecture Diagram

```
                    +-------------------+
                    |   GitHub Actions  |
                    |   CI/CD Pipeline  |
                    +--------+----------+
                             |
                    +--------v----------+
                    |    ECR Registry   |
                    |  (signed images)  |
                    +--------+----------+
                             |
              +--------------v--------------+
              |     Kubernetes Cluster      |
              |                             |
              |  +------------------------+ |
              |  | cloudflare-system ns   | |
              |  |                        | |
              |  | +--------------------+ | |
              |  | | Operator Deployment| | |
              |  | | (leader-elected)   | | |
              |  | +--------+-----------+ | |
              |  |          |             | |
              |  | +--------v-----------+ | |
              |  | | SessionBinding CRs | | |
              |  | +--------+-----------+ | |
              |  |          |             | |
              |  | +--------v-----------+ | |
              |  | |  Session Pods      | | |
              |  | +--------------------+ | |
              |  +------------------------+ |
              |                             |
              |  +------------------------+ |
              |  | CloudflareOperator     | |
              |  | Config CR (cluster)    | |
              |  | Managed by ArgoCD      | |
              |  +------------------------+ |
              |                             |
              |  +------------------------+ |
              |  | monitoring ns          | |
              |  | Prometheus -> scrape   | |
              |  | /metrics :8080        | |
              |  +------------------------+ |
              +-----------------------------+
                             |
                    +--------v----------+
                    |  Cloudflare API   |
                    |  (session mgmt)   |
                    +-------------------+
```

---

## 14. Terraform Module and State Strategy

### 14.1 Modules in platform-design

All reusable Terraform modules live in the central platform repository
(`github.com/100rd/platform-design`). This provides a single source of truth for
infrastructure patterns, versioned via git tags and consumed by all application repositories.

Application repos reference modules using the git source syntax with a pinned tag:

```hcl
module "k8s_operator" {
  source = "git::https://github.com/100rd/platform-design//terraform/modules/k8s-operator?ref=v0.1.0"
  # ...
}
```

**Tagging convention**: Modules are tagged at the repo level using semantic versioning.
For simple setups, a single tag (e.g., `v0.1.0`) covers all modules in the repo. As the
platform grows, per-module tags (e.g., `terraform/modules/k8s-operator/v1.0.0`) allow
independent versioning per module.

**Why centralize modules**:

- **Single source of truth**: One place to fix bugs or apply security patches; all consumers
  pick up the fix by bumping the ref.
- **Versioned contracts**: Pinning to a git tag means application repos are never broken by
  upstream changes until they explicitly upgrade.
- **Consistency**: Every service uses the same RDS, K8s namespace, and RBAC patterns.
- **Review efficiency**: Module changes go through a single review process in platform-design
  rather than being duplicated across dozens of app repos.

### 14.2 Per-Environment State Isolation

Terraform state is stored in a central S3 bucket (`creme-terraform-state`) with DynamoDB
locking (`terraform-locks`), but each environment gets its own state key. This prevents a
plan or apply in dev from reading or corrupting prod state.

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

The backend configuration uses partial config (`backend "s3" {}` in `versions.tf`) with
per-environment backend files that supply the bucket, key, region, and lock table:

```
terraform/configurations/cloudflare-session-operator/
├── versions.tf              # backend "s3" {} (partial)
├── main.tf                  # module calls + provider
├── variables.tf             # variable declarations
├── outputs.tf               # output declarations
├── dev/
│   ├── backend.hcl          # key = "cloudflare-session-operator/dev/terraform.tfstate"
│   └── terraform.tfvars     # dev-specific variable values
├── stage/
│   ├── backend.hcl          # key = "cloudflare-session-operator/stage/terraform.tfstate"
│   └── terraform.tfvars     # stage-specific variable values
└── prod/
    ├── backend.hcl          # key = "cloudflare-session-operator/prod/terraform.tfstate"
    └── terraform.tfvars     # prod-specific variable values
```

### 14.3 How to Initialize and Plan

To work with a specific environment:

```bash
# Initialize with the correct backend for the target environment
terraform init -backend-config=dev/backend.hcl

# Plan with environment-specific variables
terraform plan -var-file=dev/terraform.tfvars

# Combined init + plan (typical CI workflow)
terraform init -backend-config=dev/backend.hcl -reconfigure
terraform plan -var-file=dev/terraform.tfvars -out=plan.tfplan
```

To switch environments, re-initialize with a different backend config:

```bash
terraform init -backend-config=prod/backend.hcl -reconfigure
terraform plan -var-file=prod/terraform.tfvars
```

### 14.4 What Stays in the Application Repository

Application repositories contain only service-specific configuration. No reusable module
source code lives here. The following files make up a complete Terraform configuration in
an app repo:

| File | Purpose |
|------|---------|
| `main.tf` | Provider config, remote module calls, service-specific AWS resources |
| `variables.tf` | Variable declarations with descriptions and defaults |
| `outputs.tf` | Output values surfaced from module calls |
| `versions.tf` | `required_version`, `required_providers`, partial `backend "s3" {}` |
| `{env}/backend.hcl` | Per-environment backend config (bucket, key, region, lock table) |
| `{env}/terraform.tfvars` | Per-environment variable values |

Anything that is reusable across services (RDS provisioning, K8s namespace setup, IRSA
roles) belongs in `platform-design` as a module, not in the app repo.
