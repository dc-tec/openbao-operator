---
description: Day-2 runbooks for validated backup, restore, and disaster recovery workflows.
---

# Runbooks

Use these runbooks for day-2 workflows that apply to an existing deployment rather than an initial bootstrap.

<div class="grid cards" markdown>

- :material-content-save: **Scheduled Backups**

    ---

    Add scheduled backups to S3-compatible storage and verify snapshot keys in cluster status.

    [:material-arrow-right: Open Runbook](scheduled-backups-s3-compatible.md)

- :material-restore: **Restore from Snapshot**

    ---

    Restore a cluster from an S3-compatible snapshot using the `OpenBaoRestore` CRD.

    [:material-arrow-right: Open Runbook](restore-from-s3-compatible-snapshot.md)

- :material-backup-restore: **Cross-Cluster DR Restore**

    ---

    Restore the validated local DR target from a source snapshot and verify source state on the target endpoint.

    [:material-arrow-right: Open Runbook](cross-cluster-dr-restore-rustfs.md)

</div>
