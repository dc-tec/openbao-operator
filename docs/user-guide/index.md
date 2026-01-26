# OpenBao Operator – User Guide

This guide covers everything you need to know to successfully operate OpenBao on Kubernetes, from installation to Day 2 operations.

<div class="grid cards" markdown>

- :material-download: **1. Install Operator**

    ---

    Deploy the operator using Helm or manifests. Supports **Multi-Tenant** (Default) and **Single-Tenant** modes.

    [:material-arrow-right: Installation](operator/installation.md)
    [:material-arrow-right: Single-Tenant Mode](operator/single-tenant-mode.md)

- :material-account-multiple-plus: **2. Onboard Tenants**

    ---

    Provision namespaces, RBAC, and quotas for teams using the `OpenBaoTenant` resource.

    [:material-arrow-right: Tenant Onboarding](openbaotenant/overview.md)

- :material-server-plus: **3. Deploy Cluster**

    ---

    Create and configure highly available OpenBao clusters with the `OpenBaoCluster` resource.

    [:material-arrow-right: Create Cluster](openbaocluster/overview.md)

- :material-backup-restore: **4. Operate & Restore**

    ---

    Manage backups, upgrades, resizing, and disaster recovery operations.

    [:material-arrow-right: Upgrades](../user-guide/openbaocluster/operations/upgrades.md)

    [:material-arrow-right: Backup Operations](../user-guide/openbaocluster/operations/backups.md)

    [:material-arrow-right: Restore Operations](openbaorestore/overview.md)

</div>

## Advanced Topics

- [**Architecture**](../architecture/index.md) – Understand the internal controller design.
- [**Security Model**](../security/index.md) – Learn about the zero-trust security model.
- [**Troubleshooting**](openbaocluster/operations/troubleshooting.md) – Common issues and remediation steps.
