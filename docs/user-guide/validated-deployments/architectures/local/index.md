---
description: Validated local OpenBaoCluster architectures for k3d and similar developer or validation environments.
---

# Local Architectures

Use these pages when you want a validated local topology with clear invariants, tradeoffs, and linked deployment recipes.

The current local scope comes from the k3d-based project validation environment.

These pages distinguish between:

- **Local reference architectures**, which are realistic local Kubernetes deployment models for workstation validation and rehearsal.
- **Proving architectures**, which validate a specific behavior or boundary but are not the preferred general-purpose local reference.

<div class="grid cards" markdown>

- :material-test-tube: **k3d Development**

    ---

    Local reference architecture for Development profile, shared terminating edge, RustFS backups, JWT bootstrap, and blue/green upgrades.

    [:material-arrow-right: Open Architecture](k3d-development-shared-edge-rustfs.md)

- :material-shield-check: **k3d Hardened with External TLS**

    ---

    Local reference architecture for Hardened profile, Transit auto-unseal, external TLS Secrets, and user-managed passthrough.

    [:material-arrow-right: Open Architecture](k3d-hardened-transit-external-tls.md)

- :material-certificate-outline: **k3d Hardened with Internal ACME**

    ---

    Local reference architecture for Hardened profile, Transit auto-unseal, internal ACME via shared trust services, validated ACME hostname resolution, and user-managed passthrough.

    [:material-arrow-right: Open Architecture](k3d-hardened-transit-acme.md)

- :material-backup-restore: **k3d Cross-Cluster DR**

    ---

    Local reference architecture for DR rehearsal with shared Transit auto-unseal, shared RustFS snapshots, Gateway API passthrough, and manual cutover across multiple k3d clusters.

    [:material-arrow-right: Open Architecture](k3d-cross-cluster-dr-transit-rustfs.md)

</div>

## Current validated scope

| Architecture | Classification | Profile | Edge model | Certificate model | Local integrations | Validation outcome |
| :--- | :--- | :--- | :--- | :--- | :--- | :--- |
| [k3d Development](k3d-development-shared-edge-rustfs.md) | Local reference | `Development` | Shared terminating Gateway | Operator-managed TLS | RustFS | Bootstrap, JWT login, gateway exposure, backup, local blue/green lane |
| [k3d Hardened External TLS](k3d-hardened-transit-external-tls.md) | Local reference | `Hardened` | User-managed passthrough | External TLS Secrets | external Transit, cert-manager | Bootstrap, Transit unseal, JWT login, passthrough access |
| [k3d Hardened Internal ACME](k3d-hardened-transit-acme.md) | Local reference | `Hardened` | User-managed passthrough | OpenBao ACME | external trust services, ACME hostname resolution | Bootstrap, ACME issuance, Transit unseal, JWT login |
| [k3d Cross-Cluster DR](k3d-cross-cluster-dr-transit-rustfs.md) | Local reference | `Development` pair | Dedicated passthrough Gateways | Operator-managed TLS | external Transit, RustFS | Bootstrap, source backup, cross-cluster restore, target unseal, manual cutover proof |
