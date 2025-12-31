# Security Fundamentals

Core security concepts and models for the OpenBao Operator.

## Topics

| Topic | Description |
|-------|-------------|
| [Threat Model](threat-model.md) | Threat analysis, attack vectors, and mitigations |
| [Security Profiles](profiles.md) | Hardened vs Development profile comparison |
| [Secrets Management](secrets-management.md) | Auto-unseal, root tokens, and authentication |

## Overview

The OpenBao Operator implements a defense-in-depth security model:

1. **Threat Modeling** — Identifies attack vectors and implements corresponding mitigations
2. **Security Profiles** — Provides preconfigured security postures for different environments
3. **Secrets Management** — Controls how sensitive material is generated, stored, and rotated

## See Also

- [Infrastructure Security](../infrastructure/index.md) — RBAC, admission policies, network controls
- [Workload Security](../workload/index.md) — Pod security, TLS, supply chain
