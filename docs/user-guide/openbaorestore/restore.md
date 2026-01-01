# Restore Operations

The OpenBao Operator supports restoring clusters from snapshots stored in S3-compatible object storage using the `OpenBaoRestore` CRD.

!!! important "Destructive Operation"
    Force-restoring a snapshot replaces **all** Raft data. After restore, the cluster will have the exact state from the snapshot, including all secrets, policies, auth configurations, and unseal keys.

## Prerequisites

- A functioning backup in S3-compatible storage (see [Backups](../openbaocluster/operations/backups.md))
- The target OpenBao cluster must exist and be initialized
- Restore authentication configured (JWT Auth or static token)

If image verification is enabled on the target cluster (`spec.imageVerification.enabled: true`), the operator verifies the restore executor image and pins the restore Job to the verified digest.

## Creating a Restore Request

The `OpenBaoRestore` CRD acts as a "job request" - it is immutable after creation. Each restore operation is a separate Kubernetes resource.

### Basic Example

```yaml
apiVersion: openbao.org/v1alpha1
kind: OpenBaoRestore
metadata:
  name: disaster-recovery-restore
  namespace: openbao
spec:
  # Target cluster to restore INTO (must exist in same namespace)
  cluster: my-cluster
  
  # Source snapshot location
  source:
    target:
      endpoint: https://s3.amazonaws.com
      bucket: openbao-backups
      region: us-east-1
      credentialsSecretRef:
        name: s3-credentials
    # Full path to the snapshot in the bucket
    key: clusters/my-cluster/2025-01-01-120000.snap
  
  # Authentication to OpenBao
  jwtAuthRole: restore
  
  # Optional: Override executor image
  # executorImage: openbao/backup-executor:v0.1.0
  
  # Force restore even if cluster appears unhealthy
  force: false
```

## Authentication

### JWT Auth (Recommended)

Configure a restore-specific JWT role in OpenBao:

```yaml
# Add to selfInit.requests on your OpenBaoCluster
- name: create-restore-policy
  operation: update
  path: sys/policies/acl/restore
  policy:
    policy: |
      path "sys/storage/raft/snapshot-force" {
        capabilities = ["update"]
      }

- name: create-restore-jwt-role
  operation: update
  path: auth/jwt/role/restore
  data:
    role_type: jwt
    bound_audiences: ["openbao-internal"]
    bound_subject: "system:serviceaccount:<namespace>:<cluster>-restore-serviceaccount"
    token_policies: ["restore"]
    ttl: 1h
```

### Static Token

For clusters that cannot use JWT Auth:

```yaml
spec:
  tokenSecretRef:
    name: restore-token-secret
    namespace: openbao-operator-system
```

## Monitoring Restore Progress

### Watch Status

```bash
kubectl get openbaorestore disaster-recovery-restore -w
```

### Status Phases

| Phase | Description |
| :--- | :--- |
| `Pending` | Restore created, waiting to start |
| `Validating` | Checking preconditions |
| `Running` | Restore job executing |
| `Completed` | Restore succeeded |
| `Failed` | Restore failed (check message) |

### View Details

```bash
kubectl describe openbaorestore disaster-recovery-restore
```

Key status fields:

- `status.phase`: Current phase
- `status.message`: Detailed status or error message
- `status.startTime`: When restore started
- `status.completionTime`: When restore completed
- `status.snapshotKey`: Which snapshot was restored
- `status.snapshotSize`: Size of restored snapshot

## Post-Restore Considerations

### Unsealing

| Seal Type | Same Cluster | Different Cluster |
| :--- | :--- | :--- |
| Auto-unseal (KMS/Transit) | ✅ Works if seal config unchanged | ✅ Works if seal config unchanged |
| Static unseal | ✅ Existing unseal key works | ❌ Must update unseal Secret |

### Raft Cluster Recovery

After a force restore:

1. The cluster's Raft configuration is from the snapshot
2. If pod IPs have changed, the `retry_join` configuration allows pods to re-discover each other
3. For best results, restore to a cluster with the **same replica count** as when the backup was taken

## Safety Features

### Mutual Exclusion

Only one restore can run per cluster at a time.

### Operation Lock  

The operator enforces mutual exclusion across upgrades, backups, and restores using a status-based lock on the `OpenBaoCluster` (`status.operationLock`).

Restores do not start while an upgrade or backup is active.

### Break-Glass Override

If you must restore while a cluster is stuck behind an upgrade/backup lock, set:

- `spec.force: true`
- `spec.overrideOperationLock: true`

When used, the operator emits a Warning event on the `OpenBaoRestore` and records a Condition.

### Force Flag

Set `force: true` to restore even if:

- The cluster appears unhealthy
- The cluster is sealed (disaster recovery scenario)

## Example: Disaster Recovery Flow

1. **Identify the backup to restore:**

   ```bash
   # List available backups
   aws s3 ls s3://openbao-backups/clusters/my-cluster/
   ```

2. **Create restore request:**

   ```yaml
   apiVersion: openbao.org/v1alpha1
   kind: OpenBaoRestore
   metadata:
     name: dr-$(date +%Y%m%d-%H%M%S)
     namespace: openbao
   spec:
     cluster: my-cluster
     source:
       target:
         endpoint: https://s3.amazonaws.com
         bucket: openbao-backups
         region: us-east-1
         credentialsSecretRef:
           name: s3-credentials
       key: clusters/my-cluster/2025-01-01-120000.snap
     jwtAuthRole: restore
     force: true
   ```

3. **Monitor progress:**

   ```bash
   kubectl get openbaorestore -w
   ```

4. **Verify cluster health:**

   ```bash
   kubectl exec -it my-cluster-0 -- bao status
   ```
