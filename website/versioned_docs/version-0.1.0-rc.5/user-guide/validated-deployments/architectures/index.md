---
description: Validated OpenBaoCluster architecture references grouped by deployment environment.
---

# Validated Architectures

Use these pages when you want the tested topology, operating assumptions, and validation scope for a deployment model.

Recipes remain the procedural layer. Validated architectures describe the reference design that was actually exercised end to end.

The pages use three architecture classes:

- **Cloud reference architecture** for realistic deployable cloud topologies.
- **Local reference architecture** for intentional local Kubernetes models that are suitable for workstation validation, rehearsal, and integration testing, but are not production targets themselves.
- **Proving architecture** for focused validation lanes that prove an important capability or boundary without claiming to be the preferred long-term operating model.

<div class="grid cards" markdown>

- **Cloud**

    ---

    Validated cloud reference topologies, starting with the manually exercised Amazon EKS development and hardened lanes.

    [Open Category](cloud/index.md)

- **Local**

    ---

    Local reference topologies for k3d and similar workstation-based validation environments.

    [Open Category](local/index.md)

</div>

## How to use these pages

- Start with the architecture page to confirm the topology matches your intent.
- Use the linked recipe when you are ready to deploy the same pattern.
- Treat the listed invariants as part of the architecture contract. If you change them materially, you are no longer on the validated path.

## Current validated scope

| Architecture | Classification | Profile | Edge model | Certificate model | Integrations | Validation outcome |
| :--- | :--- | :--- | :--- | :--- | :--- | :--- |
| [Amazon EKS Development](cloud/amazon-eks-development-awskms-s3.md) | Cloud reference | `Development` | Shared terminating Gateway | Edge-managed certificate | AWS KMS, S3 | Bootstrap, JWT login, unseal, Gateway exposure, backup |
| [Amazon EKS Hardened](cloud/amazon-eks-hardened-awskms-acme.md) | Cloud reference | `Hardened` | Dedicated passthrough Gateway | OpenBao ACME | AWS KMS, S3 | Bootstrap, ACME issuance, JWT login, unseal, backup |
| [k3d Development](local/k3d-development-shared-edge-rustfs.md) | Local reference | `Development` | Shared terminating Gateway | Operator-managed TLS | RustFS | Bootstrap, JWT login, gateway exposure, backup, local blue/green lane |
| [k3d Hardened External TLS](local/k3d-hardened-transit-external-tls.md) | Local reference | `Hardened` | User-managed passthrough | External TLS Secrets | external Transit, cert-manager | Bootstrap, Transit unseal, JWT login, passthrough access |
| [k3d Hardened Internal ACME](local/k3d-hardened-transit-acme.md) | Local reference | `Hardened` | User-managed passthrough | OpenBao ACME | external trust services, ACME hostname resolution | Bootstrap, ACME issuance, Transit unseal, JWT login |
| [k3d Cross-Cluster DR](local/k3d-cross-cluster-dr-transit-rustfs.md) | Local reference | `Development` pair | Dedicated passthrough Gateways | Operator-managed TLS | external Transit, RustFS | Bootstrap, source backup, cross-cluster restore, target unseal, manual cutover proof |

<Callout type="note" title="Reference, not product matrix">

These pages describe the topologies that were manually validated in the project environment. They are stronger than examples, but they are not a promise that every controller, CNI, or cloud variation behaves identically.

</Callout>

