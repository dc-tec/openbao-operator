---
description: Validated local OpenBaoCluster architectures for k3d and similar developer or validation environments.
---

# Local Architectures

Use these pages when you want a validated local topology with clear invariants, tradeoffs, and linked deployment recipes.

The current local scope comes from the k3d-based validation environment in `openbao-operator-test`.

<div class="grid cards" markdown>

- :material-test-tube: **k3d Development**

    ---

    Development profile, shared terminating edge, RustFS backups, JWT bootstrap, and blue/green upgrades.

    [:material-arrow-right: Open Architecture](k3d-development-shared-edge-rustfs.md)

- :material-shield-check: **k3d Hardened with External TLS**

    ---

    Hardened profile, Transit auto-unseal, external TLS Secrets, and user-managed passthrough.

    [:material-arrow-right: Open Architecture](k3d-hardened-transit-external-tls.md)

- :material-certificate-outline: **k3d Hardened with Internal ACME**

    ---

    Hardened profile, Transit auto-unseal, internal ACME via `infra-bao`, and user-managed passthrough.

    [:material-arrow-right: Open Architecture](k3d-hardened-transit-acme.md)

</div>

## Current validated scope

| Architecture | Profile | Edge model | Certificate model | Local integrations | Validation outcome |
| :--- | :--- | :--- | :--- | :--- | :--- |
| [k3d Development](k3d-development-shared-edge-rustfs.md) | `Development` | Shared terminating Gateway | Operator-managed TLS | RustFS | Bootstrap, JWT login, gateway exposure, backup, local blue/green lane |
| [k3d Hardened External TLS](k3d-hardened-transit-external-tls.md) | `Hardened` | User-managed passthrough | External TLS Secrets | `infra-bao`, cert-manager | Bootstrap, Transit unseal, JWT login, passthrough access |
| [k3d Hardened Internal ACME](k3d-hardened-transit-acme.md) | `Hardened` | User-managed passthrough | OpenBao ACME | `infra-bao`, CoreDNS rewrite | Bootstrap, ACME issuance, Transit unseal, JWT login |
