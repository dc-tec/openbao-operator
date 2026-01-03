# Break Glass / Safe Mode

!!! failure "Critical State: Automation Halted"
    The Operator has entered **Safe Mode** because it detected a high-risk failure (e.g., loss of quorum during an upgrade). **All automation is paused** to prevent data loss.

## Overview

Safe Mode (also known as "Break Glass") is a safety mechanism. When the Operator encounters a situation where continuing an automated workflow (like a rolling upgrade or rollback) could compromise data integrity or availability, it stops and waits for human operator intervention.

Common triggers:

* Blue/Green rollback failure (risk of split-brain).
* Quorum loss during critical reconfiguration.

When active:

1. **Automation Stops**: The Operator stops reconciling the specific `OpenBaoCluster`.
2. **Status Updates**: The `status.breakGlass` field is populated with diagnostic info.
3. **Manual Ack Required**: You must explicitly "break the glass" to resume automation.

---

## 1. Inspect the Situation

Check if your cluster is in Safe Mode by inspecting its status.

```sh
kubectl -n security get openbaocluster prod-cluster -o jsonpath='{.status.breakGlass}' | jq
```

**Example Output:**

```json
{
  "active": true,
  "reason": "QuorumRisk",
  "message": "Detected split-brain potential during rollback. Manual intervention required.",
  "nonce": "abc-123-def-456",
  "steps": "1. Verify network connectivity. 2. Restore quorum manually. 3. Acknowledge."
}
```

## 2. Fix the Underlying Issue

Follow the specific guidance provided in the `message` and `steps` fields.

* **If Quorum is lost:** See [Recovering from No Leader](no-leader.md).
* **If Sealed:** See [Recovering from Sealed Cluster](sealed-cluster.md).
* **If Network Partitioned:** Verify CNI and network policies.

## 3. Acknowledge and Resume

Once you have performed the necessary manual repairs, you must tell the Operator it is safe to proceed. This is done by acknowledging the unique **nonce**.

!!! warning "Action Required"
    Copy the `nonce` from step 1 and use it in the command below.

```sh
# Replace 'abc-123-def-456' with your actual nonce
kubectl -n security patch openbaocluster prod-cluster --type merge \
  -p '{"spec":{"breakGlassAck":"abc-123-def-456"}}'
```

If the issue persists, the Operator may re-enter Safe Mode with a **new nonce**, requiring you to repeat the diagnosis.

---

## Related Runbooks

<div class="grid cards" markdown>

* :material-alert-decagram: **[No Leader / No Quorum](no-leader.md)**

    Recovery steps when the Raft cluster loses consensus.

* :material-key-chain-variant: **[Sealed Cluster](sealed-cluster.md)**

    How to unseal a cluster manually or diagnose auto-unseal failures.

* :material-restore: **[Failed Rollback](failed-rollback.md)**

    Specific steps for handling a failed Blue/Green rollback.

</div>
