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
| `api/v1alpha1/groupversion_info.go` | Scheme registration |
| `pkg/cloudflare/client.go` | Cloudflare API client (currently stubbed) |
| `config/crd/bases/` | CRD YAML |
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
  source = "../../modules/k8s-operator"

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

### 8.3 IRSA

The operator needs an IAM role for:
- Secrets Manager read access (for ESO to pull Cloudflare creds)
- S3 access (for the operator's S3 bucket)

The IRSA annotation on the ServiceAccount enables pod-level IAM without node-level credentials.

---

## 9. CI/CD Pipeline Parity

### 9.1 Current State

The CI pipeline (`ci.yml`) only covers `hello-world/`. The operator has zero CI coverage.

### 9.2 Required Pipeline Additions

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

### 9.3 Helm Chart Validation

Add `helm lint` for the operator chart in CI:
```yaml
- name: Lint operator Helm chart
  run: helm lint cloudflare-session-operator/helm/cloudflare-session-operator/
```

---

## 10. Testing Strategy

### 10.1 Unit Tests

- `controllers/sessionbinding_controller_test.go`: Test reconciliation logic with fake client.
- `pkg/cloudflare/client_test.go`: Test API client with HTTP test server.
- `pkg/flags/flags_test.go`: Test feature flag evaluation.

### 10.2 Integration Tests (envtest)

Use controller-runtime's `envtest` package to spin up a real API server:
- Test full reconciliation cycle: create binding -> verify pod created -> verify status.
- Test deletion with finalizer cleanup.
- Test error scenarios: missing deployment, Cloudflare API errors.
- Test TTL enforcement.

### 10.3 End-to-End Tests

- Deploy operator to a kind cluster.
- Create `SessionBinding` CRs and verify pod creation.
- Verify Cloudflare API calls (with mock server).
- Verify metrics and health endpoints.

---

## 11. Implementation Plan

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
| Update | `terraform/configurations/cloudflare-session-operator/dev/terraform.tfvars` | Add module vars |
| Update | `terraform/configurations/cloudflare-session-operator/stage/terraform.tfvars` | Add module vars |
| Update | `terraform/configurations/cloudflare-session-operator/prod/terraform.tfvars` | Add module vars |

### Phase 3: CI/CD Updates

| Action | File | Description |
|--------|------|-------------|
| Update | `.github/workflows/ci.yml` | Add operator job |

---

## 12. Risk Assessment

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|------------|
| Cloudflare API integration complexity | Medium | High | Start with dry-run mode, feature-flag API calls |
| RBAC over-permissioning | Low | High | Audit with `kubectl auth can-i`, principle of least privilege |
| Pod sprawl from operator | Medium | High | ResourceQuota, MaxSessionsPerNamespace flag |
| Breaking existing Terraform state | Low | Medium | New module only, no changes to existing resources |

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
