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
