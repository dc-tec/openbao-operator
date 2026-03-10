# Break Glass / Safe Mode

!!! failure "Critical State: Automation Halted"
    The OpenBao Operator has entered **Break Glass / Safe Mode** because automated rollback is no longer safe. The operator halts risky upgrade automation and waits for human intervention.

## Overview

Break Glass is a safety mechanism for high-risk upgrade recovery. Currently, the operator enters this mode when Blue/Green rollback consensus repair fails and continuing automatically could compromise availability or Raft safety.

When active:

1. **Risky automation stops**: The operator halts the affected upgrade or rollback flow.
2. **Status Updates**: The `status.breakGlass` field is populated with diagnostic info.
3. **Manual Ack Required**: You must explicitly "break the glass" to resume automation.

## 1. Inspect the Situation

Inspect the break-glass status on the `OpenBaoCluster`.

```sh
kubectl -n security get openbaocluster prod-cluster -o jsonpath='{.status.breakGlass}' | jq
```

**Example Output:**

```json
{
  "active": true,
  "reason": "RollbackConsensusRepairFailed",
  "message": "Rollback consensus repair Job upgrade-prod-cluster-rollback-retry-1 failed; manual intervention required.",
  "nonce": "abc-123-def-456",
  "steps": [
    "Inspect rollback Job logs: kubectl -n security logs job/upgrade-prod-cluster-rollback-retry-1",
    "Inspect pod status: kubectl -n security get pods -l openbao.org/cluster=prod-cluster -o wide",
    "Perform any required Raft recovery steps, then acknowledge the nonce."
  ]
}
```

## 2. Fix the Underlying Issue

Follow the guidance in `status.breakGlass.message` and `status.breakGlass.steps`.

Use these checks first:

- Inspect the last failed rollback Job:

  ```sh
  kubectl -n security get openbaocluster prod-cluster \
    -o jsonpath='{.status.blueGreen.lastJobFailure}{"\n"}'
  kubectl -n security logs job/<job-from-status>
  ```

- Inspect Blue and Green pod health:

  ```sh
  kubectl -n security get pods -l openbao.org/cluster=prod-cluster -o wide
  ```

- Inspect current Raft membership:

  ```sh
  kubectl -n security exec -it prod-cluster-0 -- bao operator raft list-peers
  ```

If manual repair requires deleting or restarting managed Pods, enable maintenance mode first when your admission policies require the `openbao.org/maintenance=true` signal:

```yaml
spec:
  maintenance:
    enabled: true
```

See the [Cluster Maintenance Guide](../operations/maintenance.md) for the broader maintenance workflow.

If you need a deeper recovery workflow, continue with [Failed Rollback Recovery](failed-rollback.md).

## 3. Acknowledge and Resume

After you repair the underlying issue, acknowledge the unique nonce to allow the operator to retry rollback automation.

!!! warning "Action Required"
    Copy the `nonce` from step 1 and use it in the command below.

```sh
# Replace 'abc-123-def-456' with your actual nonce
kubectl -n security patch openbaocluster prod-cluster --type merge \
  -p '{"spec":{"breakGlassAck":"abc-123-def-456"}}'
```

If the issue persists, the operator can re-enter break glass with a **new nonce**, requiring you to repeat the diagnosis and acknowledgment flow.

## Related Runbooks

<div class="grid cards" markdown>

* :material-alert-decagram: **[No Leader / No Quorum](no-leader.md)**

    Recovery steps when the Raft cluster loses consensus.

* :material-key-chain-variant: **[Sealed Cluster](sealed-cluster.md)**

    How to unseal a cluster manually or diagnose auto-unseal failures.

* :material-restore: **[Failed Rollback](failed-rollback.md)**

    Specific steps for handling a failed Blue/Green rollback.

</div>
