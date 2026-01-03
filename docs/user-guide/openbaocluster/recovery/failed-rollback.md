# Recovering From a Failed Rollback

This runbook addresses a **Failed Blue/Green Rollback**. This occurs when you attempt to rollback an upgrade (e.g., changing `spec.version` back to an older version), but the Operator halts the process to prevent data corruption.

!!! failure "Split Brain Risk"
    The Operator stops automation because it suspects a **Split Brain** scenario where both the Blue (Old) and Green (New) clusters might have accepted writes, or Raft consensus cannot be safely transferred back.

---

## 1. Assess the Situation

Check the **Safe Mode / Break Glass** status to understand why the Operator halted.

```sh
kubectl -n security get openbaocluster prod-cluster -o jsonpath='{.status.breakGlass}' | jq
```

**Common Reasons:**

- `ConsensusFailure`: The Blue cluster could not rejoin the Raft quorum.
- `DirtyWrite`: The Green cluster accepted writes that would be lost on rollback (if configured to check).
- `Timeout`: The rollback job took too long.

## 2. Inspect the Rollback Job

The logic for transferring leadership back to the Blue cluster runs in a Kubernetes Job named `inter-cluster-rev<Revision>`.

Find and log the failed job:

```sh
# Find the job
kubectl -n security get jobs -l openbao.org/cluster=prod-cluster

# View logs
kubectl -n security logs job/inter-cluster-rev4-rollback
```

**Log Analysis:**

| Log Message | Meaning | Resolution |
| :--- | :--- | :--- |
| `failed to join blue pilot to green leader` | Network connectivity issue between clusters. | Check NetworkPolicies / DNS. Retry. |
| `raft log index mismatch` | The Blue cluster is too far behind (Green accepted too many writes). | **Cannot Rollback**. You must proceed with Green or Restore from Snapshot. |
| `context deadline exceeded` | Operation timed out. | Retry (Acknowledge Break Glass). |

## 3. Resolution Paths

Choose the path that matches your diagnosis.

=== "Path A: Retry (Transient)"

    If the failure was due to a network blip or timeout, and you believe the clusters are healthy:

    1. **Acknowledge** the Break Glass nonce. This tells the Operator to "Try Again".

        ```sh
        kubectl -n security patch openbaocluster prod-cluster --type merge \
          -p '{"spec":{"breakGlassAck":"<NONCE_FROM_STEP_1>"}}'
        ```

    2. The Operator will spawn a **new** Rollback Job. Monitor it closely.

=== "Path B: Abort (Stay Green)"

    If the rollback is impossible (e.g., data divergence), it may be safer to **stay on the new version** (Green) and fix the application issues forward, or perform a fresh restore.

    1. **Update Spec**: Set `spec.version` back to the **Green** (New) version.
    2. **Acknowledge**: Acknowledge the nonce.
    3. The Operator will reconcile the Green cluster as the primary.

=== "Path C: Restore (Data Loss)"

    If the cluster state is corrupted beyond repair:

    1. **Stop Operator**: Scale down the operator deployment.
    2. **Delete PVCs**: Clean up the corrupted data.
    3. **Restore**: Follow the [Restore Guide](../../openbaorestore/recovery-restore-after-upgrade.md) to bring back the cluster from the last known good snapshot (Pre-Upgrade).

---

## Preventative Measures

- Always ensure **Snapshots** are taken before triggering a Rollback (The Operator does this automatically if `spec.backup.enabled` is true).
- Monitor `etcd` or `raft` metrics during upgrades.
