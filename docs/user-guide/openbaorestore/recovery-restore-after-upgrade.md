# Restore After a Partial Upgrade

This runbook applies when an upgrade is partially applied (or stuck) and you need to restore from a snapshot.

## Mutual Exclusion and the Operation Lock

The operator enforces mutual exclusion across:

- upgrades (rolling and blue/green),
- backups, and
- restores

using a status-based lock on the `OpenBaoCluster` (`status.operationLock`).

Restores do not start while an upgrade or backup is active.

## Break-Glass Restore Override

If you must restore while the cluster is stuck behind an upgrade/backup lock, use the explicit override flag on the `OpenBaoRestore`.

> [!WARNING]
> This is a break-glass escape hatch. It clears the existing lock and proceeds with restore.

Requirements:

- `spec.force: true`
- `spec.overrideOperationLock: true`

Example:

```yaml
apiVersion: openbao.org/v1alpha1
kind: OpenBaoRestore
metadata:
  name: dr-restore
  namespace: <ns>
spec:
  cluster: <name>
  source:
    target:
      endpoint: <s3-endpoint>
      bucket: <bucket>
    key: <snapshot-key>
  jwtAuthRole: restore
  force: true
  overrideOperationLock: true
```

When used, the operator emits a Warning event on the `OpenBaoRestore` and records a Condition.

## After Restore

- Re-check cluster health and seal state:
  - [Recovering From a Sealed Cluster](../openbaocluster/recovery/sealed-cluster.md)
- If the operator is in break glass mode, acknowledge it to resume automation:
  - [Break Glass / Safe Mode](../openbaocluster/recovery/safe-mode.md)
