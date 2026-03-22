# Restore Operations

The OpenBao Operator supports restoring clusters from snapshots stored in object storage (S3, GCS, Azure) using the `OpenBaoRestore` CRD.

<Callout type="tip" title="Validated deployment runbooks">

For tested restore procedures and snapshot-driven recovery flows, use the runbooks under [Validated Deployments](../validated-deployments/index.mdx).

</Callout>

<Callout type="danger" title="DATA OVERWRITE">

A Restore operation **completely overwrites** the existing data in the target OpenBaoCluster.

All secrets, policies, auth methods, and keys will be replaced by the snapshot's state. This is destructive and irreversible.

</Callout>

## 1. Prerequisites

<Callout type="tip" title="Network Requirements">

The target OpenBao cluster must be able to reach your Object Storage endpoint. Use `spec.network.egressRules` in your `OpenBaoCluster` configuration if you are running in a restricted environment.

</Callout>

- [x] A valid snapshot in your Object Storage bucket (see [Backups](../openbaocluster/operations/backups.md)).
- [x] The **Target Cluster** must exist and be initialized (even if it's just a fresh, empty cluster).
- [x] Authentication configured for the restore job:
  - JWT (`spec.jwtAuthRole`, or the default `openbao-operator-restore` role when `spec.selfInit.oidc.enabled=true`)
  - Static token (`spec.tokenSecretRef`)

<Callout type="note" title="Separate Restore Identity">

Restore Jobs use a generated restore ServiceAccount and do not inherit cloud identity from the main OpenBao Pods automatically.
If the cluster uses cloud KMS unseal identity on the main workload, configure restore storage access separately with:
- `spec.source.target.credentialsSecretRef`
- `spec.source.target.workloadIdentity.*`
- or an intentional provider default credential chain

</Callout>

---

## 2. Restore Workflow

The restore process involves multiple phases to validate, download, and inject the snapshot.

```mermaid
graph TD
    User([User]) -->|Apply CRD| Pending{Pending}
    Pending -->|Validate| Validating
    Validating -->|Download Snapshot| Job[Restore Job]
    
    subgraph Execution ["Phase: Running"]
        Job -->|Authenticate| Cluster[("fa:fa-server OpenBao Leader")]
        Job -->|Force Restore /sys/storage/raft/snapshot-force| Cluster
    end
    
    Cluster -->|Success| Completed([Completed])
    Cluster -->|Error| Failed([Failed])
    
    classDef write fill:transparent,stroke:#22c55e,stroke-width:2px,color:#fff;
    classDef process fill:transparent,stroke:#9333ea,stroke-width:2px,color:#fff;
    classDef read fill:transparent,stroke:#60a5fa,stroke-width:2px,color:#fff;
    
    class Completed write;
    class Job,Execution process;
    class User,Pending,Validating read;
```

---

## 3. Configuration

### API Contract

- `spec` is required.
- `spec` is immutable after creation.
- To change restore parameters, create a new `OpenBaoRestore` resource.

### Source Configuration

Define where your snapshot is located. The `BackupTarget` structure supports S3, GCS, and Azure storage providers.

<Tabs groupId="s3-aws-minio-gcs-google-cloud-storage-azure-blob-storage">

<TabItem value="s3-aws-minio" label="S3 (AWS/MinIO)">

```yaml
source:
  target:
    provider: s3  # Optional, defaults to "s3"
    endpoint: https://s3.amazonaws.com
    bucket: openbao-backups
    region: us-east-1
    usePathStyle: false  # Set to true for MinIO
    credentialsSecretRef:
      name: s3-credentials
  key: clusters/prod/snapshot-latest.snap
```

<Callout type="note" title="Credentials Secret">

The Secret must contain:
- `accessKeyId`: AWS access key ID
- `secretAccessKey`: AWS secret access key
- `region`: (optional) AWS region
- `sessionToken`: (optional) For temporary credentials
- `caCert`: (optional) Custom CA certificate

</Callout>

</TabItem>

<TabItem value="gcs-google-cloud-storage" label="GCS (Google Cloud Storage)">

```yaml
source:
  target:
    provider: gcs
    endpoint: https://storage.googleapis.com  # Optional, defaults to googleapis.com
    bucket: my-gcs-backups
    gcs:
      project: my-gcp-project-id  # Optional if included in credentials JSON
    credentialsSecretRef:
      name: gcs-credentials
  key: clusters/prod/snapshot-latest.snap
```

<Callout type="note" title="Credentials Secret">

The Secret must contain:
- `credentials.json`: GCS service account JSON key file

</Callout>

<Callout type="tip" title="Emulator Support">

For local testing with `fake-gcs-server`, set the endpoint to your emulator URL:
```yaml
endpoint: http://fake-gcs-server:4443
```

</Callout>

</TabItem>

<TabItem value="azure-blob-storage" label="Azure Blob Storage">

```yaml
source:
  target:
    provider: azure
    endpoint: https://myaccount.blob.core.windows.net  # Optional, auto-derived from storageAccount
    bucket: my-container  # Container name
    azure:
      storageAccount: myaccount
      container: my-container  # Optional, uses bucket if not specified
    credentialsSecretRef:
      name: azure-credentials
  key: clusters/prod/snapshot-latest.snap
```

<Callout type="note" title="Credentials Secret">

The Secret must contain one of:
- `accountKey`: Azure storage account access key
- `connectionString`: Full Azure Storage connection string

</Callout>

<Callout type="tip" title="Azurite Emulator">

For local testing with Azurite, set the endpoint to your emulator URL:
```yaml
endpoint: http://azurite:10000
provider: azure
azure:
  storageAccount: devstoreaccount1
```

</Callout>

</TabItem>

</Tabs>

### Authentication

How the Restore Job authenticates to the OpenBao cluster leader.

<Tabs groupId="jwt-auth-recommended-static-token">

<TabItem value="jwt-auth-recommended" label="JWT Auth (Recommended)">

Uses a short-lived Kubernetes ServiceAccount token. Requires `sys/auth/jwt-operator` to be enabled on the target.

```yaml
spec:
  jwtAuthRole: openbao-operator-restore  # Optional when selfInit OIDC is enabled
```

<ExpandableCallout type="example" title="OpenBao Config for JWT Auth">

Run this in OpenBao to configure the role:
```bash
bao write auth/jwt-operator/role/restore \
    role_type=jwt \
    bound_audiences=openbao-internal \
    bound_subject="system:serviceaccount:openbao:prod-cluster-restore-serviceaccount" \
    token_policies=openbao-operator-restore \
    ttl=1h
```

</ExpandableCallout>

<Callout type="note" title="JWT audience">

The restore Job uses the audience from `OPENBAO_JWT_AUDIENCE` (default: `openbao-internal`).
Set the same value in the OpenBao role `bound_audiences` and pass the env var to the operator
(`controller.extraEnv` and `provisioner.extraEnv` in Helm).

</Callout>

<Callout type="note" title="JWT bootstrap">

When `spec.selfInit.oidc.enabled` is `true`, the OpenBao Operator can create a restore role
bound to the restore ServiceAccount. The default role name is `openbao-operator-restore`.
You can omit `OpenBaoRestore.spec.jwtAuthRole` to use that default, or set it explicitly
when you use a custom role name.

</Callout>

</TabItem>

<TabItem value="static-token" label="Static Token">

Uses a long-lived OpenBao token stored in a Kubernetes Secret.

<Callout type="note" title="Same-Namespace Requirement">

The token Secret must exist in the **same namespace** as the `OpenBaoRestore` resource. Cross-namespace references are not allowed for security reasons.

</Callout>

```yaml
spec:
  tokenSecretRef:
    name: restore-token  # Must be in the same namespace as the OpenBaoRestore
```

</TabItem>

</Tabs>

---

## 4. Full Examples

<Tabs groupId="s3-example-gcs-example-azure-example">

<TabItem value="s3-example" label="S3 Example">

```yaml
apiVersion: openbao.org/v1alpha1
kind: OpenBaoRestore
metadata:
  name: dr-restore-001
  namespace: security
spec:
  cluster: prod-cluster
  force: true
  
  source:
    target:
      provider: s3
      endpoint: https://s3.amazonaws.com
      bucket: openbao-backups
      region: us-east-1
      credentialsSecretRef:
        name: s3-creds
    key: clusters/prod/backup-2024.snap
  
  jwtAuthRole: openbao-operator-restore
```

</TabItem>

<TabItem value="gcs-example" label="GCS Example">

```yaml
apiVersion: openbao.org/v1alpha1
kind: OpenBaoRestore
metadata:
  name: dr-restore-001
  namespace: security
spec:
  cluster: prod-cluster
  force: true
  
  source:
    target:
      provider: gcs
      bucket: openbao-backups
      gcs:
        project: my-gcp-project
      credentialsSecretRef:
        name: gcs-creds
    key: clusters/prod/backup-2024.snap
  
  jwtAuthRole: openbao-operator-restore
```

</TabItem>

<TabItem value="azure-example" label="Azure Example">

```yaml
apiVersion: openbao.org/v1alpha1
kind: OpenBaoRestore
metadata:
  name: dr-restore-001
  namespace: security
spec:
  cluster: prod-cluster
  force: true
  
  source:
    target:
      provider: azure
      bucket: openbao-backups
      azure:
        storageAccount: myaccount
        container: openbao-backups
      credentialsSecretRef:
        name: azure-creds
    key: clusters/prod/backup-2024.snap
  
  jwtAuthRole: openbao-operator-restore
```

</TabItem>

</Tabs>

---

## 5. Operations

### Monitoring Status

Check the phases (`Pending` -> `Running` -> `Completed`).

```bash
kubectl get obrestore -w
```

*(Shortname `obrestore` available)*

### Troubleshooting

| Phase | Common Error | Resolution |
| :--- | :--- | :--- |
| `Validating` | `cluster not found` | Ensure `spec.cluster` matches a valid `OpenBaoCluster` in the same namespace. |
| `Validating` | `snapshot not found` | Verify the `key` path is correct and the snapshot exists in the bucket/container. |
| `Running` | `403 Forbidden` | The Authentication (JWT Role/Token) lacks permission to `sys/storage/raft/snapshot-force`. |
| `Running` | `checksum mismatch` | The snapshot size/hash changed during download. Check network stability. |
| `Running` | `storage account is required` | For Azure, ensure `azure.storageAccount` is set in the target configuration. |
| `Running` | `failed to create storage client` | Verify credentials Secret exists and contains the correct keys for your provider. |
| `Failed` | `context deadline exceeded` | The restore operation timed out. Check `spec.network.egressRules` to ensure egress to storage endpoint is allowed. |
| `Failed` | `No usable temporary directory` | Internal error in restore executor. Check executor image version and pod logs. |

---

## 6. Safety Mechanics

### Operation Lock

The Operator ensures **Mutual Exclusion**. You cannot run a Restore while an Upgrade or Backup is in progress.

### Break Glass

If the cluster is stuck in a locked state (e.g., a failed upgrade) and you MUST restore:

```yaml
spec:
  force: true
  overrideOperationLock: true # (1)!
```

1. Bypasses the safety lock. Events will appear as Warnings.

## Official OpenBao Documentation

- [Operator Raft Command](https://openbao.org/docs/commands/operator/raft/)
- [Operator Unseal Command](https://openbao.org/docs/commands/operator/unseal/)
