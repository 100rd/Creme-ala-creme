# Creme-ala-creme Security & Best Practices Investigation

**Date**: 2026-02-06
**Method**: 5 competing hypothesis investigators (parallel adversarial analysis)
**Scope**: Full codebase — Go application, K8s manifests, CI/CD pipelines, Terraform IaC, K8s operator

---

## Executive Summary

| Hypothesis | Verdict | Confidence | Score |
|-----------|---------|------------|-------|
| H1: Application Security (Go code) | **CONFIRMED** | 90% | 40/100 |
| H2: CI/CD & Supply Chain | **CONFIRMED** | 94% | 15/100 |
| H3: Kubernetes & Network Security | **PARTIALLY CONFIRMED** | 78% | 35/100 |
| H4: Terraform/IaC Gaps | **CONFIRMED** | 92% | 25/100 |
| H5: Operator Safety | **CONFIRMED — HARD NO-GO** | 95% | 10/100 |

**Overall Security Posture: 25/100 — NOT PRODUCTION READY**

### What's Good
- Dockerfile: Distroless, non-root, static binary (A+)
- Helm security context: runAsNonRoot, drop ALL caps, readOnly FS, seccomp (A)
- Static analysis in CI: gosec, govulncheck, Trivy (B)
- Prometheus alerting rules: Well-structured (B+)
- RDS module (hello-world-database): Encryption, monitoring, Secrets Manager (B)

### What's Critically Broken
- Cloudflare operator is a skeleton (~20% implemented) — **MUST NOT be deployed**
- Image signing is **fake** (echo to file)
- NetworkPolicy **blocks all ingress** including health probes (service is unreachable)
- Zero RBAC in the entire codebase
- Terraform has **no security scanning** in CI

---

## All Findings — Sorted by Severity

### CRITICAL (12 findings)

| # | Finding | Source | Fix Effort |
|---|---------|--------|-----------|
| 1 | Cloudflare client is 100% stub — all sessions auto-approved | H1, H5 | 2-3 weeks |
| 2 | Arbitrary pod creation from any deployment template (operator) | H5 | 1 week |
| 3 | No RBAC manifests for operator (no Dockerfile either) | H5 | 1 week |
| 4 | Image signing is fake (echo to file, not cosign) | H2 | 2-3 days |
| 5 | Zero SBOM generation | H2 | 1 day |
| 6 | Zero SLSA provenance/attestation (Level 0) | H2 | 1-2 days |
| 7 | Production DB migrations in CI — no backup, no rollback, no approval | H2 | 2-3 days |
| 8 | NetworkPolicy blocks ALL ingress (service unreachable, probes fail) | H3 | 1 day |
| 9 | No security scanning in Terraform CI (no checkov/tflint/tfsec) | H4 | 1-2 days |
| 10 | RDS unencrypted at rest (cloudflare-session-operator config) | H4 | 0.5 day |
| 11 | S3 uses AES256 instead of KMS CMK | H4 | 0.5 day |
| 12 | No Terraform state backend (secrets in local state file) | H4 | 0.5 day |

### HIGH (19 findings)

| # | Finding | Source | Fix Effort |
|---|---------|--------|-----------|
| 13 | Admin endpoint auth bypass when ADMIN_API_KEY unset | H1 | 1 hour |
| 14 | Timing-attack vulnerable API key comparison (== not constant-time) | H1 | 1 hour |
| 15 | SessionID used in pod names without sanitization | H1, H5 | 2 hours |
| 16 | No HTTP security headers (CSP, HSTS, X-Frame-Options) | H1 | 2 hours |
| 17 | No HTTP server timeouts (Slowloris DoS vector) | H1 | 1 hour |
| 18 | Database sslmode=disable in default connection string | H1 | 1 hour |
| 19 | Helm values use mutable `latest` tag + IfNotPresent | H2, H3 | 1 day |
| 20 | Terraform CI only covers 1 of 2 configurations | H2, H4 | 1 hour |
| 21 | Local K8s deployment has ZERO security context | H3 | 2 hours |
| 22 | Zero RBAC (Roles/RoleBindings) across entire codebase | H3 | 1 day |
| 23 | No automountServiceAccountToken: false | H3 | 1 hour |
| 24 | No Pod Security Admission on Flux-deployed namespace | H3 | 2 hours |
| 25 | TTL never enforced in operator (pods live forever) | H1, H5 | 1 day |
| 26 | Silent degradation without Cloudflare credentials | H5 | 2 hours |
| 27 | Operator watches all namespaces (no namespace scoping) | H1, H5 | 2 hours |
| 28 | Secrets Manager secret has no rotation policy | H4 | 1 day |
| 29 | RDS security group allows unrestricted egress | H4 | 1 hour |
| 30 | Kafka TLS disabled in dev environment | H4 | 1 hour |
| 31 | No IAM authentication for RDS | H4 | 0.5 day |

### MEDIUM (16 findings)

| # | Finding | Source |
|---|---------|--------|
| 32 | Error messages leak internal state (readiness probe, CR status) | H1 |
| 33 | No request body size limits on admin endpoint | H1 |
| 34 | Merge conflict markers in test file (tests broken) | H1 |
| 35 | OWASP ZAP runs as non-blocking (continue-on-error: true) | H2 |
| 36 | GitHub Actions not pinned to SHA (supply chain risk) | H2 |
| 37 | Self-hosted runner trust boundary (IAM + DB + kubectl access) | H2 |
| 38 | Flux GitRepository has no verification | H2, H3 |
| 39 | Secret template is dead code (empty Secret created) | H3 |
| 40 | Missing DNS egress in NetworkPolicy | H3 |
| 41 | No ResourceQuota or LimitRange | H3 |
| 42 | Secrets Manager secret not KMS-encrypted | H4 |
| 43 | Performance Insights not KMS-encrypted | H4 |
| 44 | No S3 bucket logging | H4 |
| 45 | hello-world-database missing from CI matrix | H4 |
| 46 | No cross-region backup for RDS | H4 |
| 47 | No rate limiting on operator reconciliation | H5 |

### LOW (4 findings)

| # | Finding | Source |
|---|---------|--------|
| 48 | JSON decode error silently ignored (flags.go) | H1 |
| 49 | Docker-compose default credentials (dev only) | H2 |
| 50 | CRD lacks validation constraints | H3 |
| 51 | Output exposes DB username without sensitive flag | H4 |

---

## Remediation Plan — Prioritized

### Phase 0: Immediate Blockers (Day 1-2)

These are "stop the world" fixes:

1. **Block operator deployment** — Add a STOP warning to the cloudflare-session-operator README. It is 20% implemented and actively dangerous. NO-GO until Phase 3.

2. **Fix NetworkPolicy** — Add ingress rules for service traffic, Prometheus scraping, and kubelet probes. Add DNS egress rule. Without this, the Helm deployment is DOA.

3. **Fix admin auth bypass** — Change flags.go to return 403 when ADMIN_API_KEY is unset instead of allowing unauthenticated access.

4. **Fix HTTP server timeouts** — Add ReadHeaderTimeout, ReadTimeout, WriteTimeout, IdleTimeout.

5. **Resolve merge conflict in main_test.go** — Tests cannot compile.

### Phase 1: CI/CD & Supply Chain (Week 1)

| # | Task | Effort |
|---|------|--------|
| 1 | Replace fake image signing with cosign keyless | 2-3 days |
| 2 | Add SBOM generation (syft) and attach to image | 1 day |
| 3 | Add SLSA L3 provenance via slsa-github-generator | 1-2 days |
| 4 | Remove DB migration from CI; use Helm Job exclusively | 1 day |
| 5 | Switch Helm values from `latest` to immutable digest | 1 day |
| 6 | Pin GitHub Actions to commit SHAs | 0.5 day |
| 7 | Make OWASP ZAP blocking on HIGH/CRITICAL | 0.5 day |

### Phase 2: Application & K8s Hardening (Week 2)

| # | Task | Effort |
|---|------|--------|
| 8 | Add HTTP security headers middleware | 2 hours |
| 9 | Use crypto/subtle.ConstantTimeCompare for API key | 1 hour |
| 10 | Add http.MaxBytesReader to admin endpoint | 1 hour |
| 11 | Return generic errors from readiness probe | 1 hour |
| 12 | Fix local deployment with proper security context | 2 hours |
| 13 | Create ServiceAccount template in Helm chart | 1 hour |
| 14 | Add automountServiceAccountToken: false | 1 hour |
| 15 | Add RBAC (Role, RoleBinding) to Helm chart | 0.5 day |
| 16 | Add PSA labels to Helm namespace | 1 hour |
| 17 | Add ResourceQuota and LimitRange | 2 hours |
| 18 | Enforce sslmode=require in DB connections | 1 hour |

### Phase 3: Terraform & IaC (Week 2-3)

| # | Task | Effort |
|---|------|--------|
| 19 | Add Checkov + tflint to Terraform CI | 1 day |
| 20 | Add all configs to CI matrix | 1 hour |
| 21 | Add S3 backend for cloudflare-session-operator state | 0.5 day |
| 22 | Enable RDS encryption (cloudflare-session-operator) | 0.5 day |
| 23 | Switch S3 from AES256 to KMS CMK | 0.5 day |
| 24 | Add Secrets Manager rotation policy | 1 day |
| 25 | Add IAM authentication for RDS | 0.5 day |
| 26 | Restrict RDS security group egress | 1 hour |
| 27 | Add S3 access logging | 1 hour |
| 28 | Add Performance Insights KMS encryption | 1 hour |
| 29 | Evaluate Terragrunt for DRY multi-env | 2 days |

### Phase 4: Operator Rewrite (Week 3-6) — If Needed

| # | Task | Effort |
|---|------|--------|
| 30 | Implement actual Cloudflare session validation API | 1 week |
| 31 | Implement route management API | 1 week |
| 32 | Add startup credential validation (fail if no creds) | 2 hours |
| 33 | Add namespace scoping to manager | 2 hours |
| 34 | Generate RBAC manifests + Deployment + Dockerfile | 1 day |
| 35 | Implement TTL enforcement with requeue | 1 day |
| 36 | Add CRD validation (sessionID pattern, maxLength) | 0.5 day |
| 37 | Add validating webhook | 1 day |
| 38 | Write unit + integration tests | 1 week |

---

## Cross-Reference with Existing TBD Checklist

| TBD Item | Investigation Findings | Phase |
|----------|----------------------|-------|
| JSON structured logs with trace IDs | Not a security issue, but related to error message leakage (F32) | 2 |
| Secrets via ESO+SOPS or Vault | Helm Secret template is dead code (F39), ExternalSecret disabled by default | 2 |
| CI immutability (digests, GitOps PR, env approvals) | CRITICAL gaps — fake signing, no SBOM, mutable tags (F4-6, F19) | 1 |
| Ingress/Gateway with HTTPS via cert-manager | No ingress exists anywhere, NetworkPolicy blocks all ingress (F8) | 0 |
| Alerts & runbooks (Slack, SLOs) | Alert rules exist and are good. Runbook URLs and Slack integration TBD | 3+ |
| RBAC & quotas | Zero RBAC, zero quotas (F22, F41) | 2 |
| Ephemeral PR preview environments | Not security-related | 4+ |
| Increased test coverage | Tests are broken (merge conflict) (F34). ~10% coverage | 2 |

---

## Strengths Highlighted by Investigation

The investigation wasn't all bad news. These areas are well-implemented:

1. **Container image**: Distroless, non-root UID 65532, static binary, no shell — textbook Docker hardening
2. **Helm security context**: Comprehensive — runAsNonRoot, readOnlyRootFilesystem, drop ALL capabilities, seccomp RuntimeDefault
3. **Prometheus alerting**: 5 well-configured alert rules with sensible thresholds
4. **SAST in CI**: golangci-lint, gosec, govulncheck, Trivy (filesystem + image scans) — good static analysis coverage
5. **RDS module (hello-world-database)**: Encryption enabled, Secrets Manager integration, monitoring, proper security groups
6. **Pre-commit hooks**: gitleaks for secret detection + Go linting
7. **Feature flags**: OpenFeature/flagd integration for dynamic operational control
8. **Structured logging**: zerolog with trace context injection
9. **Graceful shutdown**: 10s timeout, proper signal handling
10. **PDB + HPA**: Pod disruption budget and horizontal autoscaling configured

---

## Investigator Agreement Matrix

| Finding Area | H1 | H2 | H3 | H4 | H5 | Consensus |
|-------------|----|----|----|----|-----|-----------|
| Operator must not deploy | X | | | | X | STRONG |
| Fake image signing | | X | | | | UNANIMOUS (single investigator) |
| NetworkPolicy blocks all ingress | | | X | | | CONFIRMED |
| No RBAC anywhere | X | | X | | X | STRONG |
| No security scanning in TF CI | | X | | X | | STRONG |
| sslmode=disable in default DB URL | X | | | X | X | STRONG |
| Mutable `latest` image tags | | X | X | | | STRONG |
| Test suite is broken | X | | | | | CONFIRMED |

---

*Generated by 5-agent competing hypothesis investigation. Each investigator worked independently and adversarially before synthesis.*
