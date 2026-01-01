# Pausing Reconciliation

Set `spec.paused` to temporarily stop reconciliation for a cluster:

```sh
kubectl -n security patch openbaocluster dev-cluster \
  --type merge -p '{"spec":{"paused":true}}'
```

While paused:

- The Operator does not mutate StatefulSets, Secrets, or ConfigMaps for the cluster.
- Finalizers and delete handling still run if the CR is deleted.

Set `spec.paused` back to `false` to resume reconciliation.

## Recovery Notes

- If a cluster appears “stuck”, first confirm whether it is paused:

  ```sh
  kubectl -n <ns> get openbaocluster <name> -o jsonpath='{.spec.paused}'
  ```

- If the operator has entered break glass mode, unpausing is not sufficient; follow the recovery steps in `status.breakGlass`:
  - [Break Glass / Safe Mode](../recovery/safe-mode.md)
