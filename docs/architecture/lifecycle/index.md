# Lifecycle Management Overview

The OpenBao Operator manages the full lifecycle of OpenBao clusters, from initial provisioning (Day 0) to complex upgrades (Day 2) and disaster recovery (Day N).

These flows describe manager-level responsibilities. In the code, `OpenBaoCluster` reconciliation is split across:

- `openbaocluster-workload` (certs, infra, init)
- `openbaocluster-adminops` (upgrade, backup)
- `openbaocluster-status` (conditions, finalizers)

<div class="grid cards" markdown>

- :material-domain: **[Day 0: Provisioning](day0-provisioning.md)**

    Tenant onboarding, Namespace creation, and RBAC delegation via the Provisioner Controller.

- :material-creation: **[Day 1: Creation](day1-creation.md)**

    Cluster initialization strategies (Self-Init vs Standard), PKI bootstrapping, and Unsealing.

- :material-cog: **[Day 2: Operations](day2-operations.md)**

    Details on Upgrade strategies (Rolling/Blue-Green), Maintenance Mode, and Version Management.

- :material-backup-restore: **[Day N: Backups](dayN-backups.md)**

    Backup scheduling, retention policies, and restoration workflows.

</div>
