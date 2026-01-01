# Deletion Policy

## Prerequisites

- **Permissions**: `delete` permissions on `OpenBaoCluster` resources.

## Resource Ownership

All resources created by the operator (StatefulSet, Services, ConfigMaps, Secrets, ServiceAccounts, Ingresses, HTTPRoutes) have Kubernetes `OwnerReferences` pointing to the parent `OpenBaoCluster`. This ensures:

- **Automatic Garbage Collection:** When an `OpenBaoCluster` is deleted, Kubernetes automatically cascades the deletion to all owned resources.
- **Reconciliation on Drift:** The controller watches all owned resources, so any out-of-band modifications trigger immediate reconciliation to restore the desired state.
- **Clear Ownership:** Each resource is clearly associated with its parent cluster via the `OwnerReference`, making it easy to identify which resources belong to which cluster.

### Deletion Policies

`spec.deletionPolicy` controls additional cleanup behavior when the `OpenBaoCluster` is deleted:

- `Retain` (default):
  - Kubernetes automatically deletes StatefulSet, Services, ConfigMap, and Secrets (via OwnerReferences).
  - Leaves PVCs and external backups intact.
- `DeletePVCs`:
  - Deletes PVCs labeled `openbao.org/cluster=<name>` in addition to the automatic cleanup.
  - Retains external backups.
- `DeleteAll` (future behavior):
  - Intended to also delete backup objects. Backup deletion will be wired into the BackupManager
    as that implementation is completed.

Delete the cluster:

```sh
kubectl -n security delete openbaocluster dev-cluster
```

Verify cleanup according to the chosen `deletionPolicy`:

```sh
kubectl -n security get statefulsets,svc,cm,secrets,pvc -l openbao.org/cluster=dev-cluster
```
