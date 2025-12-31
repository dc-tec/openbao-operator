# Security: OpenBao Operator

This section provides a comprehensive security overview for the OpenBao Operator, covering the security model, RBAC architecture, and threat analysis.

## Security Model Overview

The security model relies on a **Supervisor Pattern**, where the operator orchestrates security-critical configuration (TLS, unseal keys, network policies) from the outside, while delegating data plane security to OpenBao itself.

### Secure by Default

The Operator enforces a "Secure by Default" posture:

- **Non-Root Execution:** Operator and OpenBao pods run as non-root users
- **Read-Only Filesystem:** OpenBao pods use read-only root filesystem
- **Network Isolation:** Automatic NetworkPolicies enforce default-deny ingress
- **Least-Privilege RBAC:** Split-controller design with minimal permissions
- **Supply Chain Security:** Optional Cosign image verification

## Security Topics

| Topic | Description |
|-------|-------------|
| [Admission Policies](admission-policies.md) | ValidatingAdmissionPolicy, configuration allowlist, RBAC delegation |
| [Network Security](network-security.md) | NetworkPolicies, egress control, PDB, rate limiting |
| [Workload Security](workload-security.md) | Pod security context, ServiceAccount tokens, PSS labels |
| [Secrets Management](secrets-management.md) | Auto-unseal, root tokens, JWT/OIDC authentication |
| [TLS](tls.md) | Operator-Managed, External, and ACME TLS modes |
| [Supply Chain](supply-chain.md) | Cosign image verification, Rekor, digest pinning |
| [Security Profiles](profiles.md) | Hardened vs Development profiles |
| [RBAC](rbac.md) | Role-based access control architecture |
| [Tenant Isolation](tenant-isolation.md) | Multi-tenant security boundaries |
| [Threat Model](threat-model.md) | Threat analysis and mitigations |

## See Also

- User guide: [Security Profiles](../user-guide/security-profiles.md)
- User guide: [Security Considerations](../user-guide/security-considerations.md)
- User guide: [Multi-Tenancy](../user-guide/multi-tenancy.md)
