---
description: Step-by-step recipe for restoring an OpenBao cluster from an S3-compatible snapshot using OpenBaoRestore and JWT auth.
---

# Restore from an S3-Compatible Snapshot

This recipe restores a cluster from an existing snapshot with:

- an `OpenBaoRestore` resource
- S3-compatible object storage
- JWT authentication through the operator-managed restore role
- explicit destructive-restore acknowledgement through `force: true`

!!! success "Validated by E2E"
    This recipe follows the restore flows exercised by the in-repo E2E suite. The S3-compatible path is covered against RustFS in the `DR: Storage Providers Backup & Restore` suite, including controller-restart recovery during restore execution.

!!! danger "Destructive Operation"
    A restore overwrites the target cluster state. Secrets, auth methods, policies, and data are replaced by the snapshot contents.

## Prerequisites

- The target `OpenBaoCluster` already exists in the same namespace as the `OpenBaoRestore`.
- You know the full snapshot object key.
- A Secret with object storage credentials exists in the same namespace.
- No other long-running operation should be active unless you intentionally use restore break-glass options.

!!! tip "Recommended starting point"
    If you need a matching source snapshot first, follow [Scheduled Backups to S3-Compatible Storage](../../openbaocluster/recipes/scheduled-backups-s3-compatible.md).

## Inputs

Replace these values before applying the manifests:

| Placeholder | Example | Purpose |
| :--- | :--- | :--- |
| `<namespace>` | `openbaocluster-demo` | Namespace of the target cluster |
| `<cluster-name>` | `openbaocluster-demo` | Cluster to restore into |
| `<restore-name>` | `openbaocluster-demo-restore` | `OpenBaoRestore` name |
| `<secret-name>` | `rustfs-secret` | Secret containing object storage credentials |
| `<s3-endpoint>` | `http://rustfs-svc.rustfs.svc.cluster.local:9000` | S3-compatible endpoint |
| `<bucket>` | `openbao-backups` | Bucket containing the snapshot |
| `<snapshot-key>` | `clusters/openbaocluster-demo/openbaocluster-demo/2026-03-11T12-55-00Z-279d2d60.snap` | Full object key to restore |

## Step 1: Enable restore JWT auth on the cluster

If the target cluster does not already configure restore auth, add this block under `spec` in the `OpenBaoCluster` manifest and re-apply it:

```yaml
restore:
  jwtAuthRole: openbao-operator-restore
```

Verify that the generated restore `ServiceAccount` exists:

```bash
kubectl -n <namespace> get serviceaccount <cluster-name>-restore-serviceaccount
```

## Step 2: Capture the snapshot key

If you are restoring the most recent scheduled backup from the same cluster, read the snapshot key from status:

```bash
kubectl -n <namespace> get openbaocluster <cluster-name> \
  -o jsonpath='{.status.backup.lastBackupName}{"\n"}'
```

Use that value as `<snapshot-key>`.

## Step 3: Verify the storage credentials Secret

If you followed the backup recipe, you can reuse the same Secret. Otherwise create it now:

```bash
kubectl -n <namespace> create secret generic <secret-name> \
  --from-literal=accessKeyId='<access-key-id>' \
  --from-literal=secretAccessKey='<secret-access-key>'
```

## Step 4: Apply the OpenBaoRestore

Apply the restore manifest:

```yaml
apiVersion: openbao.org/v1alpha1
kind: OpenBaoRestore
metadata:
  name: <restore-name>
  namespace: <namespace>
spec:
  cluster: <cluster-name>
  force: true
  source:
    target:
      provider: s3
      endpoint: "<s3-endpoint>"
      bucket: "<bucket>"
      usePathStyle: true
      credentialsSecretRef:
        name: <secret-name>
    key: "<snapshot-key>"
  jwtAuthRole: openbao-operator-restore
```

!!! warning "Force is intentional here"
    The E2E-backed restore flow uses `force: true` because restore is a destructive recovery workflow. Do not apply this resource unless you intend to overwrite the target cluster state.

## Operations

### Watch the restore progress

Watch the `OpenBaoRestore` resource:

```bash
kubectl -n <namespace> get openbaorestore <restore-name> -w
```

Inspect the restore conditions:

```bash
kubectl -n <namespace> get openbaorestore <restore-name> \
  -o jsonpath='{range .status.conditions[*]}{.type}={.status}{" reason="}{.reason}{"\n"}{end}'
```

Before the restore Job starts, the important checkpoint is `RestoreConfigurationReady=True`. After success, expect `RestoreComplete=True` with reason `RestoreSucceeded`.

Then inspect the final phase and message:

```bash
kubectl -n <namespace> get openbaorestore <restore-name> \
  -o jsonpath='{.status.phase}{"\n"}{.status.snapshotKey}{"\n"}{.status.message}{"\n"}'
```

The steady-state expectation is `Completed`.

### Verify the cluster after restore

Check the target cluster conditions:

```bash
kubectl -n <namespace> get openbaocluster <cluster-name> \
  -o jsonpath='{range .status.conditions[*]}{.type}={.status}{" reason="}{.reason}{"\n"}{end}'
```

If you restored a snapshot taken from the same Development cluster, verify access again:

- Use the restored `userpass` credentials in the UI.
- Repeat the JWT login flow from [Development Profile with Self-Init and Userpass](../../openbaocluster/recipes/development-self-init-userpass.md).

## Common Failures

- `Failed` with object storage errors: verify the endpoint, bucket, Secret, and object key.
- `Failed` because another operation holds the lock: wait for backup or upgrade activity to finish, or follow the documented break-glass restore flow.
- `RestoreConfigurationReady=False`: inspect the reason first. `CredentialsSecretMissing`, `AuthenticationRequired`, and `NetworkEgressRulesRequired` are the most common setup failures.
- The restored cluster is sealed or degraded afterward: the snapshot may contain a different runtime state; inspect cluster conditions and pod logs before retrying.
- The restored auth methods differ from the current cluster: the snapshot state wins.

## See Also

- [Scheduled Backups to S3-Compatible Storage](../../openbaocluster/recipes/scheduled-backups-s3-compatible.md)
- [Restore Operations](../restore.md)
- [Restore After Partial Upgrade](../recovery-restore-after-upgrade.md)
