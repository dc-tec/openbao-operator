# Security Profiles

The Operator supports two security profiles via `spec.profile` to enforce different security postures.

## Hardened Profile

!!! important "Production Requirement"
    The Hardened profile is **REQUIRED** for all production deployments. The Development profile stores root tokens in Kubernetes Secrets, which creates a significant security risk.

The `Hardened` profile enforces strict security requirements suitable for production environments:

| Requirement | Configuration |
|-------------|---------------|
| TLS | `spec.tls.mode` MUST be `External` or `ACME` |
| Unseal | MUST use external KMS (`awskms`, `gcpckms`, `azurekeyvault`, `transit`) |
| Self-Init | `spec.selfInit.enabled` MUST be `true` |
| TLS Verification | `tlsSkipVerify=true` is rejected |

**Security Benefits:**

- **No Root Tokens:** Root tokens are never created or stored, eliminating the risk of token compromise.
- **External KMS:** Provides stronger root of trust than Kubernetes Secrets for unseal keys.
- **External TLS:** Integrates with organizational PKI and certificate management systems.
- **Optional JWT Bootstrap:** When `spec.selfInit.bootstrapJWTAuth=true`, the operator can bootstrap OpenBao JWT auth during self-init to reduce configuration drift.

## Development Profile

The `Development` profile allows relaxed security for development and testing:

- **Flexible Configuration:** Allows operator-managed TLS, static unseal, and optional self-init.
- **Security Warning:** Sets `ConditionSecurityRisk=True` to indicate relaxed security posture.
- **Root Token Storage:** Creates and stores root tokens in Kubernetes Secrets.
- **Use Case:** Suitable for development, testing, and non-production environments.

!!! warning "Development Profile Warning"
    Development profile clusters **MUST NOT** be used in production. The Development profile stores root tokens in Kubernetes Secrets, which can be compromised through Secret enumeration, etcd access, or RBAC misconfiguration.

See also:

- User guide: [Security Profiles](../../user-guide/openbaocluster/configuration/security-profiles.md)
- User guide: [Server Configuration](../../user-guide/openbaocluster/configuration/server.md)
