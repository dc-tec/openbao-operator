# Emergency Restore (Override Lock)

This guide describes how to **Force Restore** a cluster that is stuck in a locked state, typically after a [Failed Upgrade](../openbaocluster/recovery/failed-rollback.md) or a crashed automation loop.

!!! failure "Operation Locked"
    The OpenBao Operator enforces **Mutual Exclusion** using `status.operationLock` on the `OpenBaoCluster`.

    Normally, you cannot start a Restore while an Upgrade or Backup is active (or presumed active). This prevents split-brain scenarios.

## 1. When to use this

Use this procedure **ONLY** if:

1. Use are recovering from a **Failed Rollback** where the cluster state is corrupted.
2. The Operator is blocking your standard `OpenBaoRestore` with a "Cluster is locked" error.
3. You accept **Total Data Loss** of the current cluster state (replacing it with the snapshot).

## 2. Break-Glass Configuration

To bypass the safety check, you must perform a "Break Glass" procedure by setting `spec.overrideOperationLock: true`.

!!! danger "Data Overwrite & Lock Removal"
    This action will:

    1.  **Ignore** the existing Operation Lock (clearing it).
    2.  **Overwrite** the cluster data with the snapshot.
    3.  **Reset** the cluster status to match the snapshot.

```yaml
apiVersion: openbao.org/v1alpha1
kind: OpenBaoRestore
metadata:
  name: emergency-restore-001
  namespace: security
spec:
  cluster: prod-cluster
  
  # Standard Source Config (see Restore Guide)
  source:
    target:
      type: s3
      s3:
        bucket: openbao-backups
        region: us-east-1
        credentialsSecretRef:
          name: s3-credentials
    key: clusters/prod/last-good-snapshot.snap
  
  jwtAuthRole: restore
  
  # --- BREAK GLASS CONFIGURATION ---
  force: true                  # (1)!
  overrideOperationLock: true  # (2)!
```

1. Required to run restore on an "Unhealthy" cluster.
2. Required to bypass the `status.operationLock`.

## 3. Post-Restore Recovery

After the emergency restore completes:

1. **Check Seal Status**: The restored cluster may be sealed.
    - [:material-arrow-right: Recover Sealed Cluster](../openbaocluster/recovery/sealed-cluster.md)
2. **Check Raft Quorum**: If the restored snapshot had different peers, you might need to fix the peer list.
    - [:material-arrow-right: Recover No Leader](../openbaocluster/recovery/no-leader.md)
3. **Acknowledge Safe Mode**: If the operator was in Safe Mode, you may need to patch the `breakGlassAck`.
    - [:material-arrow-right: Safe Mode Guide](../openbaocluster/recovery/safe-mode.md)
