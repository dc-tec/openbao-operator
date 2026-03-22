---
description: Day-2 runbooks for validated backup, restore, and disaster recovery workflows.
---

# Runbooks

Use these runbooks for day-2 workflows that apply to an existing deployment rather than an initial bootstrap.

<div class="grid cards" markdown>

- **Scheduled Backups**

    ---

    Add scheduled backups to S3-compatible storage and verify snapshot keys in cluster status.

    [Open Runbook](scheduled-backups-s3-compatible.md)

- **Restore from Snapshot**

    ---

    Restore a cluster from an S3-compatible snapshot using the `OpenBaoRestore` CRD.

    [Open Runbook](restore-from-s3-compatible-snapshot.md)

- **Cross-Cluster DR Restore**

    ---

    Restore the validated local DR target from a source snapshot and verify source state on the target endpoint.

    [Open Runbook](cross-cluster-dr-restore-rustfs.md)

</div>

