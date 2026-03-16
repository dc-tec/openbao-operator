---
description: Step-by-step local deployment recipes for OpenBaoCluster, including self-init, Hardened, and local passthrough validation flows.
---

# Local Recipes

Use these recipes when you want a local deployment flow for development or workstation-based validation.

The current scope matches the local lanes exercised in the project test environment, including direct local bring-up and passthrough validation patterns.

<div class="grid cards" markdown>

- :material-test-tube: **Development Bootstrap**

    ---

    Create a Development-profile cluster with self-init, Operator-managed TLS, `userpass`, and JWT login.

    [:material-arrow-right: Open Recipe](development-self-init-userpass.md)

- :material-shield-check: **Hardened with External TLS**

    ---

    Deploy a Hardened cluster with Transit auto-unseal, self-init, and externally managed TLS Secrets.

    [:material-arrow-right: Open Recipe](hardened-transit-external-tls.md)

- :material-certificate-outline: **Hardened with ACME TLS**

    ---

    Deploy a Hardened cluster with Transit auto-unseal and OpenBao-managed ACME certificates.

    [:material-arrow-right: Open Recipe](hardened-transit-acme-tls.md)

</div>
