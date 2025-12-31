# Multi-Tenancy Security

Tenant isolation and security boundaries for multi-tenant deployments.

## Topics

| Topic | Description |
|-------|-------------|
| [Tenant Isolation](tenant-isolation.md) | Namespace isolation, RBAC boundaries, resource quotas |

## Overview

The OpenBao Operator supports multi-tenant deployments where multiple teams share a single operator installation:

1. **Namespace Isolation** — Each tenant operates in dedicated namespaces
2. **RBAC Boundaries** — Controller permissions are scoped per-tenant by the Provisioner
3. **Network Isolation** — Default NetworkPolicies prevent cross-tenant traffic

## Provisioner/Controller Split

The multi-tenant security model relies on the separation of:

- **Provisioner** — Cluster-scoped, creates namespaces and grants RBAC
- **Controller** — Namespace-scoped, operates only within granted permissions

This ensures no single component has both the ability to expand its own permissions AND access tenant secrets.

## See Also

- [RBAC Architecture](../infrastructure/rbac.md) — Detailed permission model
- [User Guide: Multi-Tenancy](../../user-guide/multi-tenancy.md) — Configuration guide
- [User Guide: Tenant Namespaces](../../user-guide/tenant-namespaces.md) — Provisioner setup
