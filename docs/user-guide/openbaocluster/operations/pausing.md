# Pausing Reconciliation

You can temporarily stop the Operator from managing a cluster by pausing it. This is useful for manual intervention, debugging, or preventing changes during maintenance windows.

## How to Pause

Set `spec.paused` to `true` to suspend reconciliation:

```sh
kubectl -n security patch openbaocluster dev-cluster \
  --type merge -p '{"spec":{"paused":true}}'
```

!!! note "Impact of Pausing"
    While the cluster is paused:

    - **No Mutation**: The Operator will NOT update StatefulSets, Secrets, ConfigMaps, or Services.
    - **No Healing**: If a pod crashes or is deleted, the Operator will NOT take action to fix it (though Kubernetes StatefulSet controller might).
    - **Finalizers Run**: Deletion logic still operates if the CR is deleted.

## How to Resume

Set `spec.paused` to `false` (or remove the field) to resume normal operations:

```sh
kubectl -n security patch openbaocluster dev-cluster \
  --type merge -p '{"spec":{"paused":false}}'
```

The Operator will immediately reconcile the cluster state to match the CR.

## Troubleshooting

!!! tip "Is my cluster paused?"
    If a cluster seems unresponsive to updates, check the paused status:

    ```sh
    kubectl -n security get openbaocluster dev-cluster -o jsonpath='{.spec.paused}'
    ```

!!! warning "Break Glass Recovery"
    If the Operator has entered **Safe Mode** (due to quorum loss or critical failures), simply unpausing is **not** sufficient. You must follow the Break Glass procedure to explicitly acknowledge the risk.

    See [Break Glass / Safe Mode](../recovery/safe-mode.md) for details.
