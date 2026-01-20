# Infrastructure Security

!!! abstract "Platform Controls"
    Infrastructure security focuses on the Kubernetes platform layer: protecting the Operator's control plane, isolating tenant namespaces, and enforcing policy compliance before workloads even start.

## Overview

The OpenBao Operator leverages native Kubernetes security primitives to create a **Zero Trust** environment:

1. **RBAC:** A precise, split-controller model that grants permissions only where needed (Provisioning vs. Management).
2. **Admission Policies:** Guardrails that prevent insecure configurations (like disabling TLS) from being applied.
3. **Network Security:** Isolation layers that restrict traffic flow between tenants and the internet.

## Topics

<div class="grid cards" markdown>

- :material-account-lock: **RBAC Architecture**

    ---

    Deep dive into the **Provisioner** and **Controller** role separation and the "Blind Write" pattern.

    [:material-arrow-right: Explore RBAC](rbac.md)

- :material-policy: **Admission Policies**

    ---

    Using `ValidatingAdmissionPolicy` (CEL) to enforce security standards without webhooks.

    [:material-arrow-right: View Policies](admission-policies.md)

- :material-lan-check: **Network Security**

    ---

    Default-deny `NetworkPolicies` and controlling Egress traffic for backups and upgrades.

    [:material-arrow-right: Network Controls](network-security.md)

</div>

## Prerequisites

!!! note "Cluster Requirements"
    -   **Kubernetes v1.30+**: Required for `ValidatingAdmissionPolicy` (GA in 1.30).
    -   **CNI Plugin**: A CNI that enforces `NetworkPolicy` (e.g., Cilium, Calico, Antrea) is required for isolation features to work.

## See Also

- [:material-shield-check: Security Fundamentals](../fundamentals/index.md)
- [:material-docker: Workload Security](../workload/index.md)
