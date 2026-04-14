# ADR-004: Secrets Management

**Status**: Accepted  
**Date**: 2026-04-14  
**Decision makers**: Solution Architect, Security Reviewer

## Context

The operator requires Cloudflare API credentials (`CLOUDFLARE_ACCOUNT_ID` and
`CLOUDFLARE_API_TOKEN`). Currently these are read from environment variables via
`os.Getenv()` in `NewClientFromEnv()`. Missing credentials are silently accepted,
which is a security risk in production.

The hello-world service uses External Secrets Operator (ESO) to sync secrets from
AWS Secrets Manager into Kubernetes Secrets.

## Decision

### 1. External Secrets Operator (ESO) Integration

Mirror the hello-world `externalsecret.yaml` pattern:

```yaml
apiVersion: external-secrets.io/v1beta1
kind: ExternalSecret
metadata:
  name: cloudflare-session-operator-credentials
spec:
  refreshInterval: "1h"
  secretStoreRef:
    name: aws-secretsmanager
    kind: SecretStore
  target:
    name: cloudflare-session-operator-credentials
    creationPolicy: Owner
  dataFrom:
    - extract:
        key: cloudflare-session-operator/${ENVIRONMENT}/cloudflare-api
```

### 2. Secret Structure in AWS Secrets Manager

```json
{
  "account_id": "cf-account-id-here",
  "api_token": "cf-api-token-here"
}
```

Path convention: `cloudflare-session-operator/{env}/cloudflare-api`
- `cloudflare-session-operator/dev/cloudflare-api`
- `cloudflare-session-operator/stage/cloudflare-api`
- `cloudflare-session-operator/prod/cloudflare-api`

### 3. Startup Validation

When `ENABLE_CLOUDFLARE_API=true` (default), the operator MUST fail startup if credentials
are missing. This replaces the current silent degradation:

```go
func NewClientFromEnv() (Client, error) {
    accountID := os.Getenv("CLOUDFLARE_ACCOUNT_ID")
    apiToken := os.Getenv("CLOUDFLARE_API_TOKEN")
    if accountID == "" || apiToken == "" {
        return nil, fmt.Errorf("CLOUDFLARE_ACCOUNT_ID and CLOUDFLARE_API_TOKEN are required")
    }
    ...
}
```

When `ENABLE_CLOUDFLARE_API=false` (dry-run mode), a no-op client is used.

### 4. IRSA for ESO Access

The operator's ServiceAccount gets an IAM role (via IRSA) with read-only access to the
specific Secrets Manager path. Policy:

```json
{
  "Effect": "Allow",
  "Action": ["secretsmanager:GetSecretValue"],
  "Resource": "arn:aws:secretsmanager:*:*:secret:cloudflare-session-operator/*"
}
```

## Consequences

- No hardcoded secrets in code, Helm values, or Terraform.
- Credentials auto-rotate via ESO refresh interval.
- Fail-fast on missing credentials prevents silent degradation.
- IRSA provides pod-level IAM without node credentials.
- Consistent pattern with hello-world for platform team reference.

## Alternatives Considered

- **Vault with CSI driver**: More operational overhead; ESO is already deployed for hello-world.
- **SOPS-encrypted secrets in git**: Viable but ESO provides better rotation and audit trail.
- **Kubernetes Secrets directly**: No rotation, no audit trail, manual management.
