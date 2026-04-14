# ADR-003: Observability Stack

**Status**: Accepted  
**Date**: 2026-04-14  
**Decision makers**: Solution Architect

## Context

The hello-world service has structured JSON logging (zerolog), distributed tracing (OTEL),
and Prometheus metrics. The operator currently uses `stdr` (Go stdlib log adapter) with no
tracing and only controller-runtime's default metrics.

## Decision

### 1. Structured Logging with zerolog

Replace `stdr.New(os.Stdout)` with `zerologr` adapter for controller-runtime:

```go
import "github.com/go-logr/zerologr"

zl := zerolog.New(os.Stdout).With().
    Timestamp().
    Str("controller", "sessionbinding").
    Logger()
ctrl.SetLogger(zerologr.New(&zl))
```

Every reconciliation log line includes:
- `controller`, `namespace`, `name` (from controller-runtime context)
- `sessionID` (from spec, added manually)
- `trace_id`, `span_id` (from OTEL context, when tracing enabled)
- `reconcileID` (injected by controller-runtime)

### 2. Distributed Tracing with OTEL

Mirror hello-world's `initTracer()` pattern:
- OTLP HTTP exporter pointing to `OTEL_EXPORTER_OTLP_ENDPOINT`
- Service name: `cloudflare-session-operator`
- Gated behind `ENABLE_TRACING` feature flag

Spans created for:
- `Reconcile` (root span per reconciliation)
- `EnsureSession` (child span for Cloudflare API call)
- `EnsureRoute` (child span for Cloudflare API call)
- `ensureSessionPod` (child span for pod creation)
- `handleDeletion` (root span for cleanup)

### 3. Custom Prometheus Metrics

Beyond controller-runtime built-ins, register:

| Metric | Type | Labels |
|--------|------|--------|
| `sessionbinding_active_total` | Gauge | `namespace` |
| `sessionbinding_phase` | GaugeVec | `namespace`, `phase` |
| `cloudflare_api_requests_total` | Counter | `operation`, `status` |
| `cloudflare_api_latency_seconds` | Histogram | `operation` |
| `session_pod_creation_total` | Counter | `namespace`, `result` |

Metrics are registered in `init()` and recorded in the reconciler and Cloudflare client.

### 4. ServiceMonitor

Helm chart includes a ServiceMonitor CR that scrapes the operator's `:8080/metrics` endpoint.
Matches the hello-world pattern exactly.

## Consequences

- Log output is machine-parseable JSON, consistent with hello-world.
- Trace correlation across operator and Cloudflare API calls.
- SRE dashboards can track operator health, Cloudflare API reliability, and session lifecycle.
- Alerting possible on `cloudflare_api_requests_total{status="error"}` rate.

## Alternatives Considered

- **klog (Kubernetes default)**: Structured but not JSON by default; zerolog is the platform standard.
- **Jaeger direct exporter**: OTLP is vendor-neutral; hello-world uses OTLP already.
- **StatsD for metrics**: Prometheus is the platform standard; StatsD adds another dependency.
