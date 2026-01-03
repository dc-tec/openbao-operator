# Multi-Tenancy Security

!!! abstract "Shared Platform, Isolated Tenants"
    OpenBao Operator is designed for **Hard Multi-Tenancy**. It allows multiple independent teams to share a single Kubernetes cluster and Operator installation while maintaining strict cryptographic, network, and identity isolation.

## Security Pillars

<div class="grid cards" markdown>

- :material-domain: **Tenant Isolation**

    ---

    How the "Provisioner" controller enforces strict namespace boundaries and prevents cross-tenant access.

    [:material-arrow-right: Isolation Model](tenant-isolation.md)

- :material-account-key: **RBAC Boundaries**

    ---

    The "Zero Trust" split-controller architecture that ensures no single credential has total cluster control.

    [:material-arrow-right: RBAC Architecture](../infrastructure/rbac.md)

- :material-lan-check: **Network Isolation**

    ---

    Default Deny NetworkPolicies that prevent tenants from discovering or accessing each other's pods.

    [:material-arrow-right: Network Security](../infrastructure/network-security.md)

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

- [:material-account-multiple-plus: User Guide: Multi-Tenancy](../../user-guide/openbaotenant/multi-tenancy.md)
- [:material-door-open: User Guide: Onboarding](../../user-guide/openbaotenant/onboarding.md)
