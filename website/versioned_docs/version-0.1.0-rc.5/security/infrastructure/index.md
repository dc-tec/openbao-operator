---
description: Infrastructure security controls in OpenBao Operator, including RBAC architecture, validating admission policies, and network security boundaries.
---

# Infrastructure Security

<Callout type="abstract" title="Platform Controls">

Infrastructure security focuses on the Kubernetes platform layer: protecting the Operator's control plane, isolating tenant namespaces, and enforcing policy compliance before workloads even start.

</Callout>

## Overview

The OpenBao Operator leverages native Kubernetes security primitives to create a **Zero Trust** environment:

1. **RBAC:** A precise, split-controller model that grants permissions only where needed (Provisioning vs. Management).
2. **Admission Policies:** Guardrails that prevent insecure configurations (like disabling TLS) from being applied.
3. **Network Security:** Isolation layers that restrict traffic flow between tenants and the internet.

## Topics

<div class="grid cards" markdown>

- **RBAC Architecture**

    ---

    Deep dive into the **Provisioner** and **Controller** role separation and the "Blind Write" pattern.

    [Explore RBAC](rbac.md)

- **Admission Policies**

    ---

    Using `ValidatingAdmissionPolicy` (CEL) to enforce security standards without webhooks.

    [View Policies](admission-policies.md)

- **Network Security**

    ---

    Default-deny `NetworkPolicies` and controlling Egress traffic for backups and upgrades.

    [Network Controls](network-security.md)

</div>

## Prerequisites

<Callout type="note" title="Cluster Requirements">

-   **Kubernetes v1.33+**: Minimum supported by the OpenBao Operator (see [Compatibility](../../reference/compatibility.md)).
    `ValidatingAdmissionPolicy` is GA since Kubernetes v1.30 and is available on all supported versions.
-   **CNI Plugin**: A CNI that enforces `NetworkPolicy` (e.g., Cilium, Calico, Antrea) is required for isolation features to work.

</Callout>

## See Also

- [Security Fundamentals](../fundamentals/index.md)
- [Workload Security](../workload/index.md)

