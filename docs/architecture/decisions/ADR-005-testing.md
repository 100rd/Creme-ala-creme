# ADR-005: Testing Strategy

**Status**: Accepted  
**Date**: 2026-04-14  
**Decision makers**: Solution Architect

## Context

The operator currently has zero test files. The hello-world service has `main_test.go`.
Production Kubernetes operators require comprehensive testing at multiple levels.

## Decision

### 1. Unit Tests

**Scope**: Pure Go logic without Kubernetes API calls.

| Test file | What it tests |
|-----------|--------------|
| `controllers/sessionbinding_controller_test.go` | Reconciliation logic using fake client |
| `pkg/cloudflare/client_test.go` | API client with `httptest.Server` |
| `pkg/flags/flags_test.go` | Feature flag evaluation |
| `api/v1alpha1/sessionbinding_types_test.go` | Type validation helpers |

**Fake client tests** for the controller:
- Create a `SessionBinding` with valid spec -> verify pod is created, status is updated.
- Create a `SessionBinding` with missing deployment -> verify error condition set.
- Delete a `SessionBinding` -> verify finalizer cleanup runs.
- Reconcile with Cloudflare API error -> verify requeue with backoff.
- Reconcile with TTL expired -> verify binding marked as expired.

### 2. Integration Tests (envtest)

**Scope**: Real Kubernetes API server, fake etcd, real controller logic.

Use `sigs.k8s.io/controller-runtime/pkg/envtest` to:
1. Install CRDs into the test environment.
2. Start the controller manager.
3. Create test resources and assert outcomes.

Test scenarios:
- Full reconciliation cycle: CR created -> pod created -> status conditions set.
- Finalizer cleanup: CR deleted -> pod deleted -> Cloudflare route deleted.
- Owner reference cascade: Binding deleted -> owned pod garbage collected.
- Leader election: Two controllers, only one reconciles.

**Location**: `controllers/suite_test.go` + `controllers/integration_test.go`

### 3. End-to-End Tests

**Scope**: Real cluster (kind), full operator binary, mock Cloudflare server.

Run in CI with:
```bash
kind create cluster
make deploy  # Install CRDs and operator
go test -tags=e2e ./test/e2e/
kind delete cluster
```

Test scenarios:
- Deploy operator -> verify health endpoints respond.
- Create SessionBinding -> verify pod and status.
- Delete SessionBinding -> verify cleanup.
- Verify /metrics endpoint has custom metrics.

### 4. CI Integration

Tests run in the `operator-lint-build-test` job:
```yaml
- name: Run unit tests
  run: go test -race -coverprofile=coverage.out ./...

- name: Run envtest
  run: |
    make envtest
    KUBEBUILDER_ASSETS=$(make envtest-assets) go test -tags=envtest ./controllers/...
```

Coverage target: 70% for controllers, 80% for pkg packages.

### 5. Test Fixtures

Create `testdata/` directory with:
- `testdata/sessionbinding-valid.yaml` -- valid CR for testing.
- `testdata/sessionbinding-invalid.yaml` -- CR with invalid sessionID.
- `testdata/deployment-target.yaml` -- target deployment for pod cloning.

## Consequences

- Confidence in reconciliation correctness before production.
- Regression prevention for Cloudflare API integration.
- envtest catches RBAC and CRD schema issues early.
- CI gates prevent broken code from reaching main.

## Alternatives Considered

- **Only unit tests**: Misses Kubernetes API interaction bugs.
- **Only e2e tests**: Slow, hard to debug, not suitable for rapid iteration.
- **Testcontainers**: Heavier than envtest for controller testing.
- **No tests (DEPLOYMENT_BLOCKED.md approach)**: Not acceptable for a reference platform.
