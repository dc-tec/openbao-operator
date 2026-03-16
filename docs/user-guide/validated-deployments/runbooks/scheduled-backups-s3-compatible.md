---
description: Step-by-step recipe for scheduled OpenBao backups to S3-compatible storage using the operator-managed backup ServiceAccount and JWT auth.
---

# Scheduled Backups to S3-Compatible Storage

This recipe adds scheduled backups to an existing `OpenBaoCluster` with:

- S3-compatible object storage
- `spec.selfInit.oidc.enabled: true`
- JWT authentication for the backup Job
- backup status reporting through `status.backup`

!!! success "Validated by E2E"
    This recipe follows the storage-provider backup flows exercised by the in-repo E2E suite. The S3-compatible path is covered against RustFS in the `DR: Storage Providers Backup & Restore` suite.

## Prerequisites

- A working `OpenBaoCluster` already exists.
- The cluster uses `spec.selfInit.enabled: true` and `spec.selfInit.oidc.enabled: true`.
- Your object storage bucket already exists.
- You have credentials with write access to the bucket or prefix.
- If your environment restricts egress, backup Jobs can reach the storage endpoint.

!!! tip "Recommended starting point"
    If you do not have a cluster yet, start with [Development Profile with Self-Init and Userpass](../recipes/local/development-self-init-userpass.md).

## Inputs

Replace these values before applying the manifests:

| Placeholder | Example | Purpose |
| :--- | :--- | :--- |
| `<namespace>` | `openbaocluster-demo` | Namespace of the target cluster |
| `<cluster-name>` | `openbaocluster-demo` | Target `OpenBaoCluster` name |
| `<secret-name>` | `rustfs-secret` | Secret containing object storage credentials |
| `<s3-endpoint>` | `http://rustfs-svc.rustfs.svc.cluster.local:9000` | S3-compatible endpoint |
| `<bucket>` | `openbao-backups` | Target bucket |
| `<path-prefix>` | `clusters` | Prefix used for stored snapshots |
| `<schedule>` | `*/5 * * * *` | Backup schedule |

## Step 1: Create the object storage credentials Secret

Create the Secret referenced by `spec.backup.target.credentialsSecretRef`:

```bash
kubectl -n <namespace> create secret generic <secret-name> \
  --from-literal=accessKeyId='<access-key-id>' \
  --from-literal=secretAccessKey='<secret-access-key>'
```

!!! note "E2E validation target"
    The in-repo E2E suite uses RustFS credentials:

    - `accessKeyId`: `rustfsadmin`
    - `secretAccessKey`: `rustfsadmin`

## Step 2: Add the backup configuration to the cluster

Add this block under `spec` in your `OpenBaoCluster` manifest and re-apply it:

```yaml
backup:
  schedule: "<schedule>"
  jwtAuthRole: openbao-operator-backup
  target:
    provider: s3
    endpoint: "<s3-endpoint>"
    bucket: "<bucket>"
    pathPrefix: "<path-prefix>"
    usePathStyle: true
    credentialsSecretRef:
      name: <secret-name>
  retention:
    maxCount: 7
    maxAge: "168h"
```

The object key format is:

```text
<path-prefix>/<namespace>/<cluster-name>/<timestamp>-<uuid>.snap
```

For example:

```text
clusters/openbaocluster-demo/openbaocluster-demo/2026-03-11T12-55-00Z-279d2d60.snap
```

!!! note "JWT role bootstrap"
    When `spec.selfInit.oidc.enabled: true` is already enabled, the operator can bootstrap the `openbao-operator-backup` role and bind it to `<cluster-name>-backup-serviceaccount`.

## Operations

### Verify backup resources

Confirm that the backup `ServiceAccount` exists:

```bash
kubectl -n <namespace> get serviceaccount <cluster-name>-backup-serviceaccount
```

Check the backup schedule reported in cluster status:

```bash
kubectl -n <namespace> get openbaocluster <cluster-name> \
  -o jsonpath='{.status.backup.nextScheduledBackup}{"\n"}'
```

Check the backup configuration condition:

```bash
kubectl -n <namespace> get openbaocluster <cluster-name> \
  -o jsonpath='{range .status.conditions[*]}{.type}={.status}{" reason="}{.reason}{"\n"}{end}'
```

The important checkpoint is `BackupConfigurationReady=True`.

Watch backup Jobs:

```bash
kubectl -n <namespace> get jobs -l openbao.org/component=backup -w
```

### Verify a successful backup

After the first backup succeeds, inspect the backup status:

```bash
kubectl -n <namespace> get openbaocluster <cluster-name> \
  -o jsonpath='{.status.backup.lastBackupName}{"\n"}{.status.backup.lastBackupTime}{"\n"}{.status.backup.lastFailureReason}{"\n"}'
```

The important field for later restore operations is `status.backup.lastBackupName`.

### Trigger a manual backup

Patch the cluster with the supported manual-backup annotation:

```bash
kubectl -n <namespace> annotate openbaocluster <cluster-name> \
  openbao.org/trigger-backup="$(date -u +%Y-%m-%dT%H:%M:%SZ)" --overwrite
```

This is the manual backup path exercised by the E2E suite.

## Common Failures

- The backup Job cannot reach object storage: verify the endpoint URL and network policy or egress settings.
- Authentication fails: verify the Secret keys and credentials.
- `BackupConfigurationReady=False`: inspect the reason first. `CredentialsSecretMissing`, `AuthenticationRequired`, and `NetworkEgressRulesRequired` are the most common setup failures.
- The backup Job never starts: confirm `spec.selfInit.oidc.enabled: true` or set an explicit static `tokenSecretRef`.
- Backups are skipped while another long-running operation is active: wait for upgrade or restore activity to finish and retry.

## See Also

- [Recipes Overview](../index.md)
- [Development Profile with Self-Init and Userpass](../recipes/local/development-self-init-userpass.md)
- [Restore from an S3-Compatible Snapshot](restore-from-s3-compatible-snapshot.md)
- [Backup Operations](../../openbaocluster/operations/backups.md)
