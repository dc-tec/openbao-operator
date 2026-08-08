---
title: Roadmap
description: Current project direction, future modules, design principles, and explicit non-goals.
eyebrow: Project · Direction
weight: 1
verifiedBy:
  - api/v1alpha1
  - internal
  - config/crd/bases
---

OpenBao Operator starts with one responsibility: operate OpenBao safely on Kubernetes. Future modules must build on
that lifecycle contract instead of bypassing it.

{{< callout type="note" title="Direction, not a release commitment" >}}
The roadmap does not commit to dates, final scope, or delivery order. The 0.5.x API contains only the core
`openbao.org` resources; planned modules are not part of the current operator contract.
{{< /callout >}}

## Capability map

| Area | Status | Direction |
| --- | --- | --- |
| Core lifecycle | Available | Deploy, secure, initialize, unseal, back up, restore, upgrade, and observe OpenBao |
| Claims and Service Offerings | Design and integration | Offer curated self-service clusters through a smaller tenant API |
| Application integration | Planned | Express workload identity, access, and an explicit delivery mode |
| Multi-cluster management | Planned | Use downstream agents with local authority and outbound trust |
| Federation | Planned | Govern policy, auth, PKI, and engines centrally while execution remains local |
| SPIFFE/SPIRE | Exploring | Support an optional advanced identity backend where it already exists |

## Core lifecycle boundary

The core creates and reconciles its own clusters. It owns deployment, TLS, bootstrap, unseal, backup, restore,
upgrade, read scaling, tenant onboarding, admission guardrails, and status. It is not a generic import API for an
arbitrary unmanaged OpenBao cluster.

## Future module boundaries

### Self-service claims

Platform operators define an approved service shape. A tenant selects that offering and supplies only the parameters
the platform exposes. The module renders a concrete `OpenBaoCluster`; it must block a shape that the lifecycle API
cannot represent safely.

### Application integration

Prefer short-lived workload identity and explicit delivery. Kubernetes ServiceAccount identity is the default
direction. CSI, Agent or Proxy delivery, compatibility-focused Secret synchronization, and SPIFFE identities remain
separate choices. Workload mutation and Kubernetes Secret copies are not implicit defaults.

### Multi-cluster and federation

Keep reconciliation and Kubernetes RBAC near each workload cluster. Prefer outbound agent connectivity and per-cluster
revocation over centrally stored broad kubeconfigs. Federation can distribute configuration intent, but tokens,
leases, root credentials, and storage do not become portable between clusters.

## Principles

1. Prefer explicit identity, least privilege, short-lived credentials, and safe failure modes.
2. Keep optional modules dependent on the lifecycle core, never the reverse.
3. Do not move secret values unless an operator explicitly configures that behavior.
4. Surface missing dependencies and unsafe states through status.
5. Keep downstream authentication, leases, audit, and recovery locally operable.
6. Make federation, SPIFFE, export, and fleet behavior explicit advanced features.

## Non-goals

- transparent storage replication across independent OpenBao clusters;
- portable tokens, leases, or root credentials;
- automatic replication of every secret from a management cluster;
- mandatory SPIFFE/SPIRE;
- a default mutating webhook for application integration; or
- unrestricted management-plane root access to downstream clusters.

Use GitHub issues and discussions to provide use cases and security constraints. A roadmap item enters the operator
handbook only after its API and runtime ship in a supported release.
