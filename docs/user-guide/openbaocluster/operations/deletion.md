# Deletion Policy

When an `OpenBaoCluster` is deleted, the Operator uses Kubernetes standard cascading deletion (`OwnerReferences`) to clean up resources. You can configure how aggressive this cleanup is.

## Deletion Policies

Control what happens to critical data (Persistent Volumes) and external backups when a cluster is removed.

=== "Retain (Default)"
    **Best for:** Production settings where data safety is paramount.

    The Operator cleans up compute resources (StatefulSet, Services, ConfigMaps) but **LEAVES** the storage intact.

    ```yaml
    spec:
      deletionPolicy: Retain  # Default behavior
    ```

    **Cleanup Scope:**
    
    - **Deleted:** Pods, StatefulSets, Services, ConfigMaps, Secrets
    - **Retained:** PVCs (Data), S3 Backups

=== "DeletePVCs"
    **Best for:** CI/CD pipelines, ephemeral dev environments.

    The Operator actively deletes the associated PersistentVolumeClaims (PVCs) when the cluster is deleted.

    ```yaml
    spec:
      deletionPolicy: DeletePVCs
    ```

    !!! danger "Data Loss Warning"
        This policy **permanently deletes** the underlying disk volumes when the Custom Resource is deleted. This action cannot be undone.

    **Cleanup Scope:**

    - **Deleted:** Pods, StatefulSets, Services, ConfigMaps, Secrets, **PVCs (Data)**
    - **Retained:** S3 Backups

## Performing Deletion

To delete a cluster:

```sh
kubectl -n security delete openbaocluster dev-cluster
```

## Verifying Cleanup

After deletion, you can check for any leftover resources (like PVCs if using `Retain` policy):

```sh
kubectl -n security get pvc -l openbao.org/cluster=dev-cluster
```
