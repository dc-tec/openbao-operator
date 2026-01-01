# Backups

> **See also:** [Restore from Backup](../../openbaorestore/restore.md) for restore procedures.
>
## Prerequisites

- **Object Storage**: S3-compatible endpoint (AWS S3, MinIO, GCS, Azure)
- **Permissions**: Credentials to write to the bucket

## Basic Backup Configuration

The operator supports scheduled backups of OpenBao Raft snapshots to S3-compatible object storage. Backups are executed using Kubernetes Jobs with a dedicated backup executor container.

If image verification is enabled (`spec.imageVerification.enabled: true`), the operator verifies `spec.backup.executorImage` and pins the Job to the verified digest.

### Configuration Example

```yaml
apiVersion: openbao.org/v1alpha1
kind: OpenBaoCluster
metadata:
  name: backup-cluster
spec:
  profile: Development
  # ... other spec fields ...
  backup:
    schedule: "0 3 * * *"  # Daily at 3 AM
    executorImage: "openbao/backup-executor:v0.1.0"
    target:
      endpoint: "https://s3.amazonaws.com"
      bucket: "openbao-backups"
      region: "us-east-1"
      pathPrefix: "clusters/backup-cluster"
      # Storage authentication:
      # - For static credentials, set credentialsSecretRef.
      # - For Web Identity (OIDC federation), set roleArn and omit credentialsSecretRef.
      credentialsSecretRef:
        name: s3-credentials
        namespace: default
      # roleArn: "arn:aws:iam::123456789012:role/openbao-backup"
      # Optional: Tune multipart upload performance
      # partSize: 10485760  # 10MB (default) - larger values may improve performance on fast networks
      # concurrency: 3  # Default - higher values may improve throughput but increase memory usage
    retention:
      maxCount: 7
      maxAge: "168h"  # 7 days
```

## Authentication Methods

The operator supports two authentication methods for backup operations:

### JWT Auth (Preferred)

JWT Auth uses projected ServiceAccount tokens that are automatically rotated by Kubernetes, providing better security than static tokens.

**Prerequisites:**

1. Enable JWT authentication in OpenBao (via `spec.selfInit.requests` or automatically for Hardened profile)
2. Create a JWT Auth role that binds to the backup ServiceAccount
3. Configure `spec.backup.jwtAuthRole`

**Example Configuration:**

```yaml
apiVersion: openbao.org/v1alpha1
kind: OpenBaoCluster
metadata:
  name: backup-cluster
  namespace: openbao
spec:
  profile: Development
  selfInit:
    enabled: true
    requests:
      # Enable JWT authentication
      - name: enable-jwt-auth
        operation: update
        path: sys/auth/jwt
        authMethod:
          type: jwt
      # Configure JWT auth using the cluster's JWKS public keys.
      - name: configure-jwt-auth
        operation: update
        path: auth/jwt/config
        data:
          bound_issuer: "https://kubernetes.default.svc"
          jwt_validation_pubkeys:
            - "<K8S_JWT_PUBLIC_KEY_PEM>"
      # Create backup policy
      - name: create-backup-policy
        operation: update
        path: sys/policies/acl/backup
        policy:
          policy: |
            path "sys/storage/raft/snapshot" {
              capabilities = ["read"]
            }
      # Create JWT Auth role for backups
      # The ServiceAccount name is automatically <cluster-name>-backup-serviceaccount
      - name: create-backup-jwt-role
        operation: update
        path: auth/jwt/role/backup
        data:
          role_type: jwt
          bound_audiences: ["openbao-internal"]
          bound_claims:
            kubernetes.io/namespace: openbao
            kubernetes.io/serviceaccount/name: backup-cluster-backup-serviceaccount
          token_policies: backup
          ttl: 1h
  backup:
    schedule: "0 3 * * *"
    jwtAuthRole: backup  # Reference to the role created above
    target:
      endpoint: "https://s3.amazonaws.com"
      bucket: "openbao-backups"
```

**Note:** For `Hardened` profile clusters, JWT authentication is automatically bootstrapped during self-init, so you only need to create the backup policy and role.

### Static Token (Fallback)

**Important:** Root tokens are no longer used for backup operations. All clusters (both standard and self-init) must use either JWT Auth or a dedicated backup token Secret.

For clusters that cannot use JWT Auth, you must create a backup token Secret with appropriate permissions and reference it via `spec.backup.tokenSecretRef`.

**Example for Self-Init Cluster:**

```yaml
apiVersion: openbao.org/v1alpha1
kind: OpenBaoCluster
metadata:
  name: backup-cluster
spec:
  profile: Development
  selfInit:
    enabled: true
    requests:
      # Create backup policy and AppRole (see full example in samples)
      - name: create-backup-policy
        operation: update
        path: sys/policies/acl/backup
        policy:
          policy: |
            path "sys/storage/raft/snapshot" {
              capabilities = ["read"]
            }
  backup:
    schedule: "0 3 * * *"
    tokenSecretRef:
      name: backup-token-secret
      namespace: openbao-operator-system
    target:
      endpoint: "https://s3.amazonaws.com"
      bucket: "openbao-backups"
```

## Backup ServiceAccount

The operator automatically creates `<cluster-name>-backup-serviceaccount` when backups are enabled. This ServiceAccount:

- Is used by backup Jobs for JWT Auth (via projected ServiceAccount token)
- Has its token mounted at `/var/run/secrets/tokens/openbao-token` (projected volume with audience `openbao-internal`)
- Is owned by the OpenBaoCluster for automatic cleanup

## Backup Execution

When a backup is scheduled:

1. The operator creates a Kubernetes Job with the backup executor container
2. The Job runs as the backup ServiceAccount
3. The executor:
   - Authenticates to OpenBao (JWT Auth via projected token or static token)
   - Discovers the Raft leader
   - Streams the snapshot directly to object storage
   - Verifies the upload
4. Job status is monitored and backup status is updated in `Status.Backup`

## Backup Status

Monitor backup status via the cluster status:

```sh
kubectl get openbaocluster backup-cluster -o yaml
```

Status fields:

- `Status.Backup.LastBackupTime`: Timestamp of last successful backup
- `Status.Backup.NextScheduledBackup`: Next scheduled backup time
- `Status.Backup.ConsecutiveFailures`: Number of consecutive failures
- `Status.Conditions`: `BackingUp` condition shows current backup state

## Retention Policies

Configure automatic cleanup of old backups:

```yaml
backup:
  retention:
    maxCount: 7      # Keep only the 7 most recent backups
    maxAge: "168h"   # Delete backups older than 7 days
```

Retention is applied after successful backup upload. Deletion failures are logged but do not fail the backup.

**Note:** When using Web Identity for object storage (`spec.backup.target.roleArn`), the controller does not perform
retention deletes. Prefer enforcing retention via storage-native lifecycle policies (for example, S3 Lifecycle Rules).

## Pre-Upgrade Snapshots

Enable automatic snapshots before upgrades:

```yaml
spec:
  upgrade:
    preUpgradeSnapshot: true
  backup:
    # Backup configuration is required when preUpgradeSnapshot is enabled
    target:
      endpoint: "https://s3.amazonaws.com"
      bucket: "openbao-backups"
    executorImage: "openbao/bao-backup:latest"
```

This ensures you have a backup before any version upgrade begins. The pre-upgrade backup uses the same backup configuration (`spec.backup.target`, `spec.backup.executorImage`, etc.) as regular scheduled backups.

## S3 Upload Performance Tuning

For large snapshots or specific network conditions, you can tune the multipart upload parameters:

```yaml
backup:
  target:
    endpoint: "https://s3.amazonaws.com"
    bucket: "openbao-backups"
    # Tune multipart upload performance
    partSize: 20971520      # 20MB parts (default: 10MB)
    concurrency: 5          # 5 concurrent parts (default: 3)
```

**Configuration Options:**

- `partSize` (int64, optional): Size of each part in multipart uploads in bytes. Defaults to 10MB (10485760). Minimum: 5MB (5242880). Larger values may improve performance for large snapshots on fast networks, while smaller values may be better for slow or unreliable networks.
- `concurrency` (int32, optional): Number of concurrent parts to upload during multipart uploads. Defaults to 3. Range: 1-10. Higher values may improve throughput on fast networks but increase memory usage and may overwhelm slower storage backends.

**When to Tune:**

- **Slow networks**: Reduce `partSize` and `concurrency` to avoid timeouts
- **Fast networks with large snapshots**: Increase `partSize` and `concurrency` for better throughput
- **Memory-constrained environments**: Reduce `concurrency` to lower memory usage
