---
description: Step-by-step local deployment recipes for OpenBaoCluster, including self-init, Hardened, and local passthrough validation flows.
---

# Local Recipes

Use these recipes when you want a local deployment flow for development or workstation-based validation.

The current scope matches the local lanes exercised in the project test environment, including direct local bring-up and passthrough validation patterns.

<div class="grid cards" markdown>

- **Development Bootstrap**

    ---

    Create a Development-profile cluster with self-init, Operator-managed TLS, `userpass`, and JWT login.

    [Open Recipe](development-self-init-userpass.md)

- **Hardened with External TLS**

    ---

    Deploy a Hardened cluster with Transit auto-unseal, self-init, and externally managed TLS Secrets.

    [Open Recipe](hardened-transit-external-tls.md)

- **Hardened with ACME TLS**

    ---

    Deploy a Hardened cluster with Transit auto-unseal and OpenBao-managed ACME certificates.

    [Open Recipe](hardened-transit-acme-tls.md)

- **Cross-Cluster DR Bootstrap**

    ---

    Bootstrap the validated local DR proving ground with shared Transit auto-unseal, RustFS snapshots, and multiple k3d clusters.

    [Open Recipe](k3d-cross-cluster-dr-bootstrap.md)

</div>

