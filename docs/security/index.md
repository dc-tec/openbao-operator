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

### Fundamentals

Core security concepts and threat modeling.

| Topic | Description |
|-------|-------------|
| [Threat Model](fundamentals/threat-model.md) | Attack vectors and mitigations |
| [Security Profiles](fundamentals/profiles.md) | Hardened vs Development profiles |
| [Secrets Management](fundamentals/secrets-management.md) | Auto-unseal, root tokens, authentication |

---

### Infrastructure

Platform-level security controls.

| Topic | Description |
|-------|-------------|
| [RBAC](infrastructure/rbac.md) | Role-based access control architecture |
| [Admission Policies](infrastructure/admission-policies.md) | ValidatingAdmissionPolicy configuration |
| [Network Security](infrastructure/network-security.md) | NetworkPolicies and egress controls |

---

### Workload

Pod and container-level security.

| Topic | Description |
|-------|-------------|
| [Pod Security](workload/workload-security.md) | Security contexts, PSS labels, ServiceAccounts |
| [TLS](workload/tls.md) | Operator-Managed, External, and ACME TLS modes |
| [Supply Chain](workload/supply-chain.md) | Cosign image verification, digest pinning |

---

### Multi-Tenancy

Tenant isolation for shared deployments.

| Topic | Description |
|-------|-------------|
| [Tenant Isolation](multi-tenancy/tenant-isolation.md) | Namespace isolation, RBAC boundaries |

## See Also

- User guide: [Security Profiles](../user-guide/openbaocluster/configuration/security-profiles.md)
- User guide: [Security Considerations](../user-guide/openbaocluster/security-considerations.md)
- User guide: [Multi-Tenancy](../user-guide/openbaotenant/multi-tenancy.md)
- User guide: [Production Checklist](../user-guide/openbaocluster/operations/production-checklist.md)
