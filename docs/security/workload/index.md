# Workload Security

Pod and container-level security controls for OpenBao deployments.

## Topics

| Topic | Description |
|-------|-------------|
| [Pod Security](workload-security.md) | Security contexts, PSS labels, ServiceAccounts |
| [TLS](tls.md) | Operator-managed, external, and ACME TLS modes |
| [Supply Chain](supply-chain.md) | Cosign image verification and digest pinning |

## Overview

Workload security ensures that OpenBao pods run with minimal privileges and verified images:

1. **Pod Security** — Non-root execution, read-only filesystem, restricted capabilities
2. **TLS** — Automated certificate management with multiple provider options
3. **Supply Chain** — Image verification using Sigstore ecosystem

## Security Context Defaults

All OpenBao pods are configured with:

- `runAsNonRoot: true`
- `readOnlyRootFilesystem: true`
- `allowPrivilegeEscalation: false`
- `seccompProfile: RuntimeDefault`

## See Also

- [Fundamentals](../fundamentals/index.md) — Threat model and security profiles
- [Infrastructure Security](../infrastructure/index.md) — RBAC and network controls
