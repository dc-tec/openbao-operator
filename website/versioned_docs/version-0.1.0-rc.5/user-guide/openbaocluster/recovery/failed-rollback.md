---
slug: /recover/failed-rollback
---

# Failed Rollback Recovery

Use this runbook when a **Blue/Green rollback** enters break glass mode. This happens when rollback consensus repair fails and the operator stops automation to prevent data corruption or an unsafe Raft reconfiguration.

<Callout type="failure" title="Split Brain Risk">

Do not try to force a downgrade by patching `spec.version` back to an older release. Downgrades are blocked. Recover the cluster state first, then let the operator resume or restore from snapshot.

</Callout>

## 1. Assess the Situation

Inspect the break-glass and Blue/Green status fields:

```sh
kubectl -n security get openbaocluster prod-cluster -o jsonpath='{.status.breakGlass}' | jq
kubectl -n security get openbaocluster prod-cluster \
  -o jsonpath='{.status.blueGreen.phase}{"\n"}{.status.blueGreen.lastJobFailure}{"\n"}'
```

Expected signals:

- `status.breakGlass.reason=RollbackConsensusRepairFailed`
- `status.blueGreen.phase=RollingBack`
- `status.blueGreen.lastJobFailure=<rollback-job-name>`

Check the current conditions as well:

```sh
kubectl -n security get openbaocluster prod-cluster \
  -o jsonpath='{range .status.conditions[*]}{.type}={.status} {.reason}{"\n"}{end}'
```

## 2. Inspect the Rollback Job

Inspect the Job recorded in `status.blueGreen.lastJobFailure` first. If that field is empty, list upgrade Jobs for the cluster.

```sh
kubectl -n security get jobs -l openbao.org/cluster=prod-cluster

kubectl -n security logs job/<job-from-status>
```

Then inspect the OpenBao pods and Raft peers:

```sh
kubectl -n security get pods -l openbao.org/cluster=prod-cluster -o wide
kubectl -n security exec -it prod-cluster-0 -- bao operator raft list-peers
```

If your recovery steps require deleting or restarting managed Pods, enable maintenance mode first when your admission policies require the maintenance annotation:

```yaml
spec:
  maintenance:
    enabled: true
```

See the [Cluster Maintenance Guide](../operations/maintenance.md) for the broader maintenance workflow.
By default, the managed-resource mutation lock allows maintenance-mode bypass only for callers in the Kubernetes group `system:masters` unless you configured different break-glass groups at install time.

Check for these classes of failure:

- Network isolation between Blue and Green pods.
- Pods that are not Ready or remain sealed.
- Raft quorum loss or peer membership that no longer matches the expected rollback topology.
- Image pull or executor Job failures that prevented rollback automation from completing.

## 3. Resolution Paths

Choose the path that matches your diagnosis.

<Tabs groupId="path-a-retry-transient-path-b-pause-and-repair-path-c-restore-from-snapshot">

<TabItem value="path-a-retry-transient" label="Path A: Retry (Transient)">

If the failure was transient and you restored healthy cluster conditions:

1. Fix the underlying issue.
2. Acknowledge the break-glass nonce. This tells the operator to retry rollback automation.

    ```sh
    kubectl -n security patch openbaocluster prod-cluster --type merge \
      -p '{"spec":{"breakGlassAck":"<NONCE_FROM_STEP_1>"}}'
    ```

3. Monitor the new rollback Job and `status.blueGreen.phase`.

</TabItem>

<TabItem value="path-b-pause-and-repair" label="Path B: Pause and Repair">

If the cluster needs manual repair before you allow any further automation:

1. Pause reconciliation:

    ```sh
    kubectl -n security patch openbaocluster prod-cluster --type merge \
      -p '{"spec":{"paused":true}}'
    ```

2. Perform the required Raft or infrastructure repair.
3. Resume reconciliation and acknowledge break glass when the cluster is stable:

    ```sh
    kubectl -n security patch openbaocluster prod-cluster --type merge \
      -p '{"spec":{"paused":false,"breakGlassAck":"<NONCE_FROM_STEP_1>"}}'
    ```

</TabItem>

<TabItem value="path-c-restore-from-snapshot" label="Path C: Restore from Snapshot">

If the cluster state is corrupted beyond repair:

1. Stop further automation.
2. Identify the last known good snapshot.
3. Follow the [Emergency Restore Guide](../../openbaorestore/recovery-restore-after-upgrade.md).

</TabItem>

</Tabs>

## Preventative Measures

- Enable pre-upgrade snapshots with `spec.upgrade.preUpgradeSnapshot=true` or `spec.upgrade.blueGreen.preUpgradeSnapshot=true`.
- Verify `spec.backup` target and backup authentication before starting a production upgrade.
- Monitor `status.blueGreen.phase`, `status.blueGreen.lastJobFailure`, and cluster health during the upgrade window.
