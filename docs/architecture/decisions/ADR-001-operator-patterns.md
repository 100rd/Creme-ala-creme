# ADR-001: Kubernetes Operator Patterns

**Status**: Accepted  
**Date**: 2026-04-14  
**Decision makers**: Solution Architect, Security Reviewer

## Context

The cloudflare-session-operator manages SessionBinding custom resources. It must follow
Kubernetes operator best practices to be production-ready. The current implementation uses
controller-runtime but misses several production patterns.

## Decision

### 1. Controller-Runtime Conventions

- Use `client.MergeFrom` patch for status updates instead of full Update to avoid conflicts.
- Add `GenerationChangedPredicate` to skip reconciliation on status-only changes.
- Increase `MaxConcurrentReconciles` to 3 with a `workqueue.DefaultControllerRateLimiter()`.
- Scope the manager cache to the operator's namespace via `cache.Options.DefaultNamespaces`.

### 2. CRD Design Enhancements

- Add `+kubebuilder:printcolumn` annotations for phase, sessionID, boundPod, and age.
- Add `+kubebuilder:validation:Pattern="^[a-z0-9]([-a-z0-9]*[a-z0-9])?$"` on sessionID.
- Add `+kubebuilder:validation:MaxLength=253` on sessionID.
- Add `+kubebuilder:resource:shortName=sb`.
- Add `+kubebuilder:subresource:status` (already present).

### 3. Error Classification

- **Transient errors** (Cloudflare API timeout, network errors): Requeue with exponential backoff.
- **Permanent errors** (invalid spec, missing deployment): Set error condition, do not requeue.
- **Degraded state** (missing credentials): Log warning, emit event, set condition.

### 4. Finalizer Strategy

Keep the current finalizer pattern. On deletion:
1. Delete the session pod (owner reference handles this, but explicit delete ensures cleanup).
2. Delete the Cloudflare route.
3. Remove the finalizer.

If Cloudflare route deletion fails, keep the finalizer and requeue. This prevents orphaned routes.

### 5. Leader Election

Enable by default in production (Helm value `leaderElection.enabled: true`).
Use `Lease` objects in the operator namespace for leader election.

## Consequences

- Status patch conflicts eliminated.
- Reduced unnecessary reconciliations from status-only changes.
- Clear error handling improves debuggability.
- Namespace scoping reduces RBAC blast radius.

## Alternatives Considered

- **Operator SDK (Ansible/Helm)**: Rejected because the operator needs Go-level control for
  Cloudflare API integration and complex pod lifecycle management.
- **Metacontroller**: Rejected because it adds operational complexity and limits controller logic.
