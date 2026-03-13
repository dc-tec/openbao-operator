---
description: Security overview for OpenBao Operator covering zero-trust model, RBAC boundaries, admission policy guardrails, workload hardening, and tenant isolation.
---

# Security: OpenBao Operator

This section provides a security overview for OpenBao Operator, covering the trust model, RBAC and admission boundaries, workload hardening, and tenant isolation.

## Security Model Overview

The security model relies on a **Supervisor Pattern**. The operator manages lifecycle, guardrails, and integration points around OpenBao, while OpenBao remains responsible for data-plane consistency, seal semantics, and runtime cryptography.

### Secure by Default

The Operator enforces a "Secure by Default" posture:

- **Non-Root Execution:** Operator and OpenBao pods run as non-root users
- **Read-Only Filesystem:** OpenBao pods use read-only root filesystem
- **Network Isolation:** Automatic NetworkPolicies enforce default-deny ingress
- **Least-Privilege RBAC:** Split-controller design with minimal permissions
- **Supply Chain Security:** Optional Cosign image verification

## Security Conditions To Watch

These conditions are the fastest operator-visible checks for security posture and integration drift:

- `ProductionReady`
- `CloudUnsealIdentityReady`
- `BackupConfigurationReady`
- `RestoreConfigurationReady`
- `GatewayIntegrationReady`
- `APIServerNetworkReady`

Use [Status Conditions and Events](../reference/status-and-events.md) for the full reason list.

### Tenancy Security Models

- **Multi-Tenant (Zero Trust):** The Controller is treated as untrusted across namespaces. It cannot enumerate Secrets and only gets name-scoped Secret access via Provisioner-managed RBAC. With admission policies enabled, this creates a strong boundary between tenants.
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
