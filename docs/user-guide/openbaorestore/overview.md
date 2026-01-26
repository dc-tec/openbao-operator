# OpenBaoRestore

`OpenBaoRestore` is an operational CRD used to request a detailed restore operation from object storage into an existing `OpenBaoCluster`.

It is designed to be **declarative** and **immutable**—once a restore is completed, the CR status captures the result for audit purposes.

## Restore Flow

The restore process involves downloading a specific snapshot from Cloud Storage and injecting it into the cluster's leader.

```mermaid
graph LR
    Bucket[("fa:fa-bucket Cloud Storage\n(S3/GCS/Azure)")] -->|Download| Job[["fa:fa-file-arrow-down Restore Job"]]
    Job -->|Inject Snapshot| Cluster[("fa:fa-server OpenBao Cluster")]
    Cluster -->|Reset| Key["fa:fa-key New Unseal Key"]
    
    classDef read fill:transparent,stroke:#60a5fa,stroke-width:2px,color:#fff;
    classDef write fill:transparent,stroke:#22c55e,stroke-width:2px,color:#fff;
    classDef process fill:transparent,stroke:#9333ea,stroke-width:2px,color:#fff;
    classDef security fill:transparent,stroke:#dc2626,stroke-width:2px,color:#fff;
    
    class Bucket read;
    class Job process;
    class Cluster write;
    class Key security;
```

## Use Cases

<div class="grid cards" markdown>

- :material-ambulance: **Disaster Recovery**

    Recover from total cluster loss or data corruption by restoring from the latest healthy backup.

- :material-content-copy: **Environment Cloning**

    Clone a **Production** dataset into a **Staging** or **Dev** environment for realistic testing.

- :material-truck-fast: **Migration**

    Move data between Kubernetes clusters or regions by backing up Source and restoring Target.

</div>

!!! warning "Data Overwrite"
    A Restore operation **completely overwrites** the existing data in the target OpenBaoCluster. Ensure you are targeting the correct cluster.

## Configuration

```yaml
apiVersion: openbao.org/v1alpha1
kind: OpenBaoRestore
metadata:
  name: restore-job-001
  namespace: security
spec:
  # The cluster to overwrite
  cluster: prod-cluster
  
  # The source of the backup
  source:
    target:
      provider: s3  # s3, gcs, or azure
      endpoint: https://s3.amazonaws.com
      bucket: my-backups
      region: us-east-1
      credentialsSecretRef:
        name: s3-credentials
    key: clusters/prod/backup-2024-01-01.snap
```

## Next Steps

- [Performing a Restore](restore.md)
- [Backup Configuration](../openbaocluster/operations/backups.md)
