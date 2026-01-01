# BackupManager (Snapshots to Object Storage)

**Responsibility:** Schedule and execute Raft snapshots, stream them to object storage, and manage retention.

## Backup Execution Flow

Backups are executed using Kubernetes Jobs that run the `bao-backup` executor container. This design keeps the operator stateless and allows backups to run independently.

### 1. Pre-flight Checks

- Verify cluster is healthy (`Phase == Running`).
- Verify no upgrade is in progress (`Status.Upgrade == nil`).
- Verify no restore is in progress for the cluster (active `OpenBaoRestore`), to avoid lock contention.
- Verify no other backup is in progress (`BackingUp` condition is False).
- Verify authentication is configured (JWT Auth role or token Secret).

### 2. Create Backup Job

- Create a Kubernetes Job with a unique name: `backup-<cluster-name>-<timestamp>`
- Job runs the backup executor image (`spec.backup.executorImage`)
- If image verification is enabled (`spec.imageVerification.enabled`), the Job is pinned to the verified digest
- Job Pod uses the backup ServiceAccount
- Job Pod mounts TLS CA certificate and configures object storage access:
  - Static credentials via `spec.backup.target.credentialsSecretRef` (Secret volume mount), or
  - Web Identity via `spec.backup.target.roleArn` (projected ServiceAccount token + AWS SDK env vars)
- Job passes `spec.backup.target.region` to the executor for S3-compatible clients

### 3. Backup Executor Workflow

1. Authenticates to OpenBao (JWT Auth via projected token or static token)
2. Discovers the Raft leader
3. Streams `GET /v1/sys/storage/raft/snapshot` directly to object storage (no disk buffering)
4. Verifies upload completion

### 4. Process Job Result

- On success: Update `Status.Backup` fields
- On failure: Increment `ConsecutiveFailures` and set `LastFailureReason`

### 5. Apply Retention

- If retention policy is configured and static credentials are used, delete old backups after successful upload
- When Web Identity is used (`spec.backup.target.roleArn`), prefer storage-native lifecycle policies for retention

## Backup Naming Convention

Backups are named predictably:

```
<pathPrefix>/<namespace>/<cluster>/<timestamp>-<short-uuid>.snap
```

## Backup Scheduling

- Uses `github.com/robfig/cron/v3` for cron parsing and scheduling.
- One scheduler instance per `OpenBaoCluster`.
- `Status.Backup.NextScheduledBackup` stores the next scheduled time.
- On Operator restart, the next schedule is recalculated from the cron expression.

## Manual Backups

- A one-off backup can be triggered by setting the `openbao.org/trigger-backup` annotation on the `OpenBaoCluster`.
- After observing the trigger, the operator clears the annotation to make it edge-triggered.

## Configuration Options

| Field | Description |
|-------|-------------|
| `spec.backup.schedule` | Cron expression for backup schedule |
| `spec.backup.executorImage` | Container image for backup executor |
| `spec.backup.target.endpoint` | S3-compatible endpoint URL |
| `spec.backup.target.bucket` | Bucket name |
| `spec.backup.target.pathPrefix` | Path prefix within bucket |
| `spec.backup.target.region` | AWS region |
| `spec.backup.target.credentialsSecretRef` | Secret with static credentials |
| `spec.backup.target.roleArn` | IAM role for Web Identity |
| `spec.backup.preUpgradeSnapshot` | Create backup before upgrades |

See also:

- [Restore User Guide](../user-guide/openbaorestore/restore.md)
- [Backups User Guide](../user-guide/openbaocluster/operations/backups.md)
