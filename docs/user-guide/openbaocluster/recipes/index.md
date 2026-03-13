---
description: Step-by-step, validated recipes for development, backup, restore, and Hardened OpenBao cluster deployments.
---

# Recipes

These recipes are grounded in the project's manual validation environment. Use them when you want a complete, step-by-step flow instead of a feature reference page.

<div class="grid cards" markdown>

- :material-test-tube: **Development Bootstrap**

    ---

    Create a Development-profile cluster with self-init, Operator-managed TLS, `userpass`, and JWT login.

    [:material-arrow-right: Open Recipe](development-self-init-userpass.md)

- :material-content-save: **Scheduled Backups**

    ---

    Add scheduled backups to S3-compatible storage and verify snapshot keys in cluster status.

    [:material-arrow-right: Open Recipe](scheduled-backups-s3-compatible.md)

- :material-restore: **Restore from Snapshot**

    ---

    Restore a cluster from an S3-compatible snapshot using the `OpenBaoRestore` CRD.

    [:material-arrow-right: Open Recipe](../../openbaorestore/recipes/restore-from-s3-compatible-snapshot.md)

- :material-shield-check: **Hardened with External TLS**

    ---

    Deploy a Hardened cluster with Transit auto-unseal, self-init, and externally managed TLS Secrets.

    [:material-arrow-right: Open Recipe](hardened-transit-external-tls.md)

- :material-certificate-outline: **Hardened with ACME TLS**

    ---

    Deploy a Hardened cluster with Transit auto-unseal and OpenBao-managed ACME certificates.

    [:material-arrow-right: Open Recipe](hardened-transit-acme-tls.md)

</div>
