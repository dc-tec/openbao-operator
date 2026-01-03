# Security Fundamentals

!!! abstract "Core Concepts"
    This section defines the foundational security models and mechanisms of the OpenBao Operator, establishing the baseline for secure operations.

## Security Model

The Operator implements a **Defense-in-Depth** strategy, ensuring security at multiple layers:

1. **Threat Modeling:** Proactive identification of attack vectors and mitigations.
2. **Profiles:** Pre-configured security postures (Development vs. Hardened).
3. **Secrets:** Secure lifecycle management for root tokens and auto-unseal keys.

## Topics

<div class="grid cards" markdown>

- :material-shield-alert: **Threat Model**

    ---

    Detailed analysis of trust boundaries, potential threats, and architectural mitigations.

    [:material-arrow-right: Read Analysis](threat-model.md)

- :material-server-security: **Security Profiles**

    ---

    Comparison of `development` versus `hardened` profiles and their impact on cluster configuration.

    [:material-arrow-right: Compare Profiles](profiles.md)

- :material-key: **Secrets Management**

    ---

    How the Operator generates, encrypts, and rotates sensitive credentials like Root Tokens and Recovery Keys.

    [:material-arrow-right: Manage Secrets](secrets-management.md)

</div>

## See Also

- [:material-server-network: Infrastructure Security](../infrastructure/index.md) — RBAC and Network Policies.
- [:material-docker: Workload Security](../workload/index.md) — Pod Security and TLS.
