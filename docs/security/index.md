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

### Tenancy Security Models

- **Multi-Tenant (Zero Trust):** The Controller is untrusted. It cannot read Secrets and must request permissions via the Provisioner. This creates a hard security boundary between tenants.
- **Single-Tenant (Direct Admin):** The Controller is fully trusted within its namespace. It has `ClusterRole` permissions bound to that specific namespace, simplifying operations but removing the Zero Trust isolation.

## Security Topics

<div class="grid cards" markdown>

- :material-shield-check: **Fundamentals**

    ---

    Threat model, profiles, and secrets management.

    [:material-arrow-right: Threat Model](fundamentals/threat-model.md)

    [:material-arrow-right: Profiles](fundamentals/profiles.md)

    [:material-arrow-right: Secrets](fundamentals/secrets-management.md)

- :material-server-network: **Infrastructure**

    ---

    RBAC, Admission Policies, and Network Security.

    [:material-arrow-right: RBAC](infrastructure/rbac.md)

    [:material-arrow-right: Policies](infrastructure/admission-policies.md)

    [:material-arrow-right: Networking](infrastructure/network-security.md)

- :material-docker: **Workload**

    ---

    Pod security, TLS, and Supply Chain.

    [:material-arrow-right: Pod Security](workload/workload-security.md)

    [:material-arrow-right: TLS](workload/tls.md)

    [:material-arrow-right: Supply Chain](workload/supply-chain.md)

- :material-account-group: **Multi-Tenancy**

    ---

    Namespace isolation and tenant boundaries.

    [:material-arrow-right: Tenant Isolation](multi-tenancy/tenant-isolation.md)

</div>

## See Also

- User guide: [Security Profiles](../user-guide/openbaocluster/configuration/security-profiles.md)
- User guide: [Security Considerations](../user-guide/openbaocluster/security-considerations.md)
- User guide: [Multi-Tenancy](../user-guide/openbaotenant/multi-tenancy.md)
- User guide: [Production Checklist](../user-guide/openbaocluster/operations/production-checklist.md)
