# DEPLOYMENT BLOCKED — DO NOT DEPLOY THIS OPERATOR

## Status: NOT SAFE FOR ANY ENVIRONMENT

This operator is approximately 20% implemented. The Cloudflare API integration is entirely stubbed — all session validation returns `true` without making any API call. Deploying this operator would create a pod-creation oracle where any user with SessionBinding CR permissions can spawn unlimited pods with zero validation.

## Critical Issues

1. **Cloudflare API is 100% stub** — `EnsureSession`, `EnsureRoute`, `DeleteRoute` all return success without calling Cloudflare
2. **No RBAC manifests** — No ClusterRole, RoleBinding, or ServiceAccount defined
3. **No Dockerfile** — Cannot be built or deployed
4. **No namespace scoping** — Watches all namespaces by default
5. **TTLSeconds never enforced** — Field defined in CRD but ignored by controller
6. **No input validation** — SessionID used directly in pod names without sanitization
7. **Silent credential degradation** — Missing Cloudflare credentials silently accepted

## Minimum Requirements Before Deployment

- [ ] Implement actual Cloudflare session validation API
- [ ] Implement actual route management API
- [ ] Fail startup if Cloudflare credentials are missing
- [ ] Generate and ship RBAC manifests scoped to a single namespace
- [ ] Create operator Deployment manifest + Dockerfile
- [ ] Implement TTL enforcement
- [ ] Add CRD validation (sessionID pattern, maxLength)
- [ ] Add namespace-scoped cache in manager options
- [ ] Write unit and integration tests
