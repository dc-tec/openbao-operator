# OpenBaoRestore Controller (Restore Lifecycle)

**Responsibility:** Reconcile `OpenBaoRestore` resources and orchestrate snapshot restores via a Kubernetes Job.

## Design Summary

- Restores are modeled as a separate CRD (`OpenBaoRestore`) rather than an `OpenBaoCluster` “mode”, to keep GitOps intent (`OpenBaoCluster.spec`) stable and make restores auditable and repeatable.
- The controller delegates most logic to `internal/restore.Manager`.
- The restore controller does not use `Owns()` watches for Jobs. It polls Job state via `GET` + `RequeueAfter` to stay compatible with the tenant-scoped RBAC model.

## Restore Workflow (High Level)

1. **Create `OpenBaoRestore`:** User creates an `OpenBaoRestore` in the same namespace as the target `OpenBaoCluster`.
2. **Phase machine:** The controller advances `status.phase` through `Pending` → `Validating` → `Running` → (`Completed` | `Failed`).
3. **Validation:** Ensures the target cluster exists, checks preconditions (unless `spec.force`), and prepares ServiceAccount/RBAC for the restore Job.
4. **Operation lock:** Acquires `OpenBaoCluster.status.operationLock` for `Restore` to avoid concurrent backups/upgrades; `spec.overrideOperationLock` provides a break-glass override (requires `spec.force`).
5. **Run restore Job:** Creates a Kubernetes Job that pulls the snapshot from object storage and restores it to the cluster.
6. **Observe completion:** Polls the Job to update `OpenBaoRestore.status` and move to a terminal phase.

## Interactions With Backups/Upgrades

- While a restore is in progress, the backup manager detects the in-progress restore and skips starting new backups to avoid lock contention.
- Restores are intended to be run while `OpenBaoCluster.spec.paused=true` to prevent workload reconciliation from fighting the restore process.

See also:

- [Backup Manager](backup-manager.md)
- [Lifecycle Flows](lifecycle.md)
- [Restore User Guide](../user-guide/openbaorestore/restore.md)
