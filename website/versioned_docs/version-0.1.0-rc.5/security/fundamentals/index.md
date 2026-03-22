---
description: Core security fundamentals for OpenBao Operator including threat model, security profiles, and secrets management practices.
---

# Security Fundamentals

<Callout type="abstract" title="Core Concepts">

This section defines the foundational security models and mechanisms of the OpenBao Operator, establishing the baseline for secure operations.

</Callout>

## Security Model

The Operator implements a **Defense-in-Depth** strategy, ensuring security at multiple layers:

1. **Threat Modeling:** Proactive identification of attack vectors and mitigations.
2. **Profiles:** Pre-configured security postures (Development vs. Hardened).
3. **Secrets:** Secure lifecycle management for root tokens and auto-unseal keys.

## Topics

<div class="grid cards" markdown>

- **Threat Model**

    ---

    Detailed analysis of trust boundaries, potential threats, and architectural mitigations.

    [Read Analysis](threat-model.md)

- **Security Profiles**

    ---

    Comparison of `development` versus `hardened` profiles and their impact on cluster configuration.

    [Compare Profiles](profiles.md)

- **Secrets Management**

    ---

    How the Operator generates, encrypts, and rotates sensitive credentials like Root Tokens and Recovery Keys.

    [Manage Secrets](secrets-management.md)

</div>

## See Also

- [Infrastructure Security](../infrastructure/index.md) — RBAC and Network Policies.
- [Workload Security](../workload/index.md) — Pod Security and TLS.

