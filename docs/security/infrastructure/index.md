# Infrastructure Security

Platform-level security controls for the OpenBao Operator.

## Topics

| Topic | Description |
|-------|-------------|
| [RBAC](rbac.md) | Role-based access control architecture |
| [Admission Policies](admission-policies.md) | ValidatingAdmissionPolicy configuration |
| [Network Security](network-security.md) | NetworkPolicies and egress controls |

## Overview

Infrastructure security covers the Kubernetes platform controls that protect the operator and its managed workloads:

1. **RBAC** — Zero-trust permission model with Provisioner/Controller split
2. **Admission Policies** — Kubernetes-native policy enforcement via ValidatingAdmissionPolicy
3. **Network Security** — Default-deny ingress policies and controlled egress

## Prerequisites

- Kubernetes v1.33+ (for ValidatingAdmissionPolicy GA)
- Network plugin supporting NetworkPolicy (Calico, Cilium, etc.)

## See Also

- [Fundamentals](../fundamentals/index.md) — Threat model and security profiles
- [Workload Security](../workload/index.md) — Pod-level security controls
