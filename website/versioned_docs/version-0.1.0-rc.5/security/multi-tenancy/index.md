---
description: Multi-tenancy security model for OpenBao Operator, describing tenant isolation, namespace boundaries, and least-privilege access control.
---

# Multi-Tenancy Security

<Callout type="abstract" title="Shared Platform, Isolated Tenants">

OpenBao Operator is designed for **Hard Multi-Tenancy**. It allows multiple independent teams to share a single Kubernetes cluster and Operator installation while maintaining strict cryptographic, network, and identity isolation.

</Callout>

<Callout type="success" title="Recommended Production Model">

Multi-tenant mode is the recommended production operating model. It combines Provisioner/Controller separation, admission guardrails, and namespace introduction controls to keep tenant onboarding and workload management isolated.

</Callout>

## Security Pillars

<div class="grid cards" markdown>

- **Tenant Isolation**

    ---

    How the Provisioner controller enforces strict namespace boundaries and prevents cross-tenant access.

    [Isolation Model](tenant-isolation.md)

- **RBAC Boundaries**

    ---

    The Zero Trust split-controller architecture that ensures no single credential has total cluster control.

    [RBAC Architecture](../infrastructure/rbac.md)

- **Network Isolation**

    ---

    Default Deny NetworkPolicies that prevent tenants from discovering or accessing each other's pods.

    [Network Security](../infrastructure/network-security.md)

</div>

## The Split-Controller Model

To achieve secure multi-tenancy, the Operator splits responsibilities between two distinct controllers:

1. **The Provisioner:**
    - **Scope:** Cluster-wide.
    - **Power:** Can create Roles/RoleBindings but **cannot** read Secrets or manage Workloads.
    - **Role:** The "Landlord" who hands out keys but can't enter apartments.

2. **The Controller:**
    - **Scope:** Namespace-restricted (per tenant).
    - **Power:** Can manage Workloads/Secrets but **only** in namespaces where the Provisioner issued a key.
    - **Role:** The "Tenant" who manages their own apartment.

## See Also

- [User Guide: Multi-Tenancy](../../user-guide/openbaotenant/multi-tenancy.md)
- [User Guide: Onboarding](../../user-guide/openbaotenant/onboarding.md)

