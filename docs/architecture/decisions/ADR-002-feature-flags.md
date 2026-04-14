# ADR-002: Feature Flags for the Operator

**Status**: Accepted  
**Date**: 2026-04-14  
**Decision makers**: Solution Architect

## Context

The hello-world service uses OpenFeature with flagd for dynamic feature flags, allowing
runtime control of tracing, metrics, and admin features. The operator needs a similar
capability but adapted for controller reconciliation context rather than HTTP request context.

## Decision

### Approach: Environment Variables as Primary, flagd as Optional

1. **Environment-based flags** for coarse-grained operator behavior:
   - `ENABLE_CLOUDFLARE_API` (default: `true`) -- gates actual API calls vs dry-run
   - `ENABLE_TTL_ENFORCEMENT` (default: `true`) -- gates the TTL reaper
   - `ENABLE_METRICS` (default: `true`) -- gates custom metrics recording
   - `ENABLE_TRACING` (default: `false`) -- gates OTEL span creation

2. **flagd integration** (optional) for dynamic flags:
   - `max_sessions_per_namespace` (default: 100)
   - `reconcile_dry_run` (default: false)
   - `cloudflare_api_timeout_seconds` (default: 10)

3. **No admin HTTP endpoints** for the operator. Operators should not expose mutation
   endpoints beyond the Kubernetes API. Flag changes go through:
   - Kubernetes ConfigMap update (env var change + rollout restart)
   - flagd configuration update (dynamic, no restart)

### Implementation

Create `pkg/flags/flags.go` with:
- `IsCloudflareAPIEnabled(ctx) bool`
- `IsTTLEnforcementEnabled(ctx) bool`
- `IsTracingEnabled(ctx) bool`
- `MaxSessionsPerNamespace(ctx) int`

Use `sync/atomic` for env-based defaults, OpenFeature client for dynamic overrides.

## Consequences

- Operators can safely deploy with `ENABLE_CLOUDFLARE_API=false` for testing.
- TTL enforcement can be toggled without redeployment.
- No new attack surface (no admin endpoints).
- Consistent pattern with hello-world for the platform team.

## Alternatives Considered

- **Only environment variables**: Simpler but requires pod restart for every change.
- **Kubernetes ConfigMap watch**: More Kubernetes-native but adds complexity and RBAC surface.
- **Full OpenFeature for everything**: Over-engineered for an operator's simpler flag needs.
