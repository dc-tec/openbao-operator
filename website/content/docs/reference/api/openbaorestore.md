---
title: OpenBaoRestore API
description: Fields, defaults, and validation for the OpenBaoRestore API.
eyebrow: Reference · Generated API
weight: 2
verifiedBy:
  - api/v1alpha1 at bf538212baa79eadb65f74f4db1e204d39870651
  - docs/reference/api.md at bf538212baa79eadb65f74f4db1e204d39870651
---

{{< callout type="note" title="Generated reference" >}}

This page is synchronized from the generated API reference at `bf538212baa79eadb65f74f4db1e204d39870651` for the `0.5.x` documentation line.
{{< /callout >}}


## Packages
- [openbao.org/v1alpha1](#openbaoorgv1alpha1)


## openbao.org/v1alpha1

Package v1alpha1 contains API Schema definitions for the openbao v1alpha1 API group.

### Resource Types
- [OpenBaoRestore](#openbaorestore)



#### AzureTargetConfig



AzureTargetConfig holds Azure Blob Storage specific configuration.



_Appears in:_
- [BackupTarget](#backuptarget)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `storageAccount` _string_ | StorageAccount is the Azure storage account name.<br />Required when using Azure provider. |  | MinLength: 1 <br /> |
| `container` _string_ | Container is the blob container name. If empty, uses the Bucket field value. |  | Optional: \{\} <br /> |


#### BackupTarget



BackupTarget describes a generic, cloud-agnostic object storage destination.



_Appears in:_
- [BackupSchedule](../openbaocluster/#backupschedule)
- [RestoreSource](#restoresource)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `provider` _string_ | Provider selects the storage backend. Defaults to "s3" for backward compatibility. | s3 | Enum: [s3 gcs azure] <br />Optional: \{\} <br /> |
| `endpoint` _string_ | Endpoint is the HTTP(S) endpoint for the object storage service.<br />For S3: Required (e.g., "https://s3.amazonaws.com" or MinIO endpoint).<br />For GCS: Optional (defaults to googleapis.com).<br />For Azure: Optional (derived from StorageAccount if not specified). |  | Optional: \{\} <br /> |
| `bucket` _string_ | Bucket is the bucket or container name. |  | MinLength: 1 <br /> |
| `pathPrefix` _string_ | PathPrefix is an optional prefix within the bucket for this cluster's snapshots. |  | Optional: \{\} <br /> |
| `credentialsSecretRef` _[LocalObjectReference](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#localobjectreference-v1-core)_ | CredentialsSecretRef optionally references a Secret containing credentials for the object store.<br />The Secret must exist in the same namespace as the owning OpenBao resource.<br />Cross-namespace references are not allowed for security reasons.<br />For S3: Expected keys are "accessKeyId" and "secretAccessKey" (optional: "sessionToken", "region", "caCert").<br />For GCS: Expected key is "credentials.json" containing a service account JSON key.<br />For Azure: Expected keys are "accountKey" or "connectionString".<br />Hardened clusters require an explicit storage identity path: credentialsSecretRef,<br />workloadIdentity metadata, or roleArn for S3 targets. Omitting those paths relies<br />on ambient/default credentials and is rejected for Hardened clusters. |  | Optional: \{\} <br /> |
| `workloadIdentity` _[WorkloadIdentityConfig](#workloadidentityconfig)_ | WorkloadIdentity optionally applies provider-specific metadata required by cloud workload identity integrations.<br />Use this for ambient identity setups such as EKS Pod Identity or IRSA, GKE Workload Identity, or Azure Workload Identity.<br />When omitted, backup and restore workloads can still use any credentials exposed through the pod's default provider chain.<br />Hardened clusters reject that ambient/default path unless credentialsSecretRef is set,<br />workloadIdentity metadata is present, or an S3 target uses roleArn. |  | Optional: \{\} <br /> |
| `partSize` _integer_ | PartSize is the size of each part in multipart uploads (in bytes).<br />Defaults to 10MB (10485760 bytes). Larger values may improve performance for large snapshots<br />on fast networks, while smaller values may be better for slow or unreliable networks. | 10485760 | Minimum: 5.24288e+06 <br />Optional: \{\} <br /> |
| `concurrency` _integer_ | Concurrency is the number of concurrent parts to upload during multipart uploads.<br />Defaults to 3. Higher values may improve throughput on fast networks but increase<br />memory usage and may overwhelm slower storage backends. | 3 | Maximum: 10 <br />Minimum: 1 <br />Optional: \{\} <br /> |
| `region` _string_ | Region is the AWS region to use for S3-compatible clients.<br />For AWS, this should match the bucket region (for example, "eu-west-1").<br />For many S3-compatible stores (MinIO/Ceph), this can be any non-empty value.<br />Only used when Provider is "s3". | us-east-1 | Optional: \{\} <br /> |
| `roleArn` _string_ | RoleARN is the IAM role ARN (or S3-compatible equivalent) to assume via Web Identity.<br />When set, backup and restore Jobs mount a projected ServiceAccount token and set the<br />AWS Web Identity environment variables explicitly.<br />Only used when Provider is "s3".<br />Outside Hardened S3 targets, leave this empty when relying on ambient workload identity<br />or provider-managed default credentials instead. For Hardened S3 targets, roleArn is<br />one accepted explicit identity path. It does not satisfy Hardened identity requirements<br />for GCS or Azure. |  | Optional: \{\} <br /> |
| `usePathStyle` _boolean_ | UsePathStyle controls whether to use path-style addressing (bucket.s3.amazonaws.com/object)<br />or virtual-hosted-style addressing (bucket.s3.amazonaws.com/object).<br />Set to true for MinIO and S3-compatible stores that require path-style.<br />Set to false for AWS S3 (default, as AWS is deprecating path-style).<br />Only used when Provider is "s3". | false | Optional: \{\} <br /> |
| `gcs` _[GCSTargetConfig](#gcstargetconfig)_ | GCS contains Google Cloud Storage specific configuration.<br />Only used when Provider is "gcs". |  | Optional: \{\} <br /> |
| `azure` _[AzureTargetConfig](#azuretargetconfig)_ | Azure contains Azure Blob Storage specific configuration.<br />Only used when Provider is "azure". |  | Optional: \{\} <br /> |
| `insecureSkipVerify` _boolean_ | InsecureSkipVerify allows skipping TLS verification (useful for MinIO/LocalStack/Azurite with self-signed certs).<br />This applies to all providers that support TLS.<br />Hardened clusters reject insecureSkipVerify. |  | Optional: \{\} <br /> |


#### GCSTargetConfig



GCSTargetConfig holds Google Cloud Storage specific configuration.



_Appears in:_
- [BackupTarget](#backuptarget)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `project` _string_ | Project is the GCP project ID. Optional if using ADC with default project or<br />if the credentials JSON includes the project. |  | Optional: \{\} <br /> |


#### OpenBaoRestore



OpenBaoRestore represents a request to restore an OpenBao cluster from a snapshot.
This resource is immutable after creation - it acts as a "job request".





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `openbao.org/v1alpha1` | | |
| `kind` _string_ | `OpenBaoRestore` | | |
| `spec` _[OpenBaoRestoreSpec](#openbaorestorespec)_ |  |  |  |
| `status` _[OpenBaoRestoreStatus](#openbaorestorestatus)_ |  |  |  |


#### OpenBaoRestoreSpec



OpenBaoRestoreSpec defines the desired state for a restore operation.
An OpenBaoRestore acts as a "job request" - it is immutable after creation.



_Appears in:_
- [OpenBaoRestore](#openbaorestore)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `cluster` _string_ | Cluster is the name of the OpenBaoCluster to restore INTO.<br />Must exist in the same namespace as the OpenBaoRestore. |  | MinLength: 1 <br /> |
| `source` _[RestoreSource](#restoresource)_ | Source defines where the snapshot comes from. |  |  |
| `jwtAuthRole` _string_ | JWTAuthRole is the name of the JWT Auth role configured in OpenBao<br />for restore operations. When set, the restore executor will use JWT Auth<br />(projected ServiceAccount token) instead of a static token.<br />The role must be configured in OpenBao and must grant the "update" capability on<br />sys/storage/raft/snapshot-force. The role must bind to the restore ServiceAccount<br />(&lt;cluster-name&gt;-restore-serviceaccount) in the cluster namespace.<br />If this field is empty and the target OpenBaoCluster has OIDC enabled,<br />the operator will default to using the "openbao-operator-restore" role. |  | Optional: \{\} <br /> |
| `tokenSecretRef` _[LocalObjectReference](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#localobjectreference-v1-core)_ | TokenSecretRef optionally references a Secret containing an OpenBao API<br />token to use for restore operations (fallback method).<br />The Secret must exist in the same namespace as the OpenBaoRestore.<br />Cross-namespace references are not allowed for security reasons.<br />The token must have permission to update sys/storage/raft/snapshot-force.<br />If JWTAuthRole is set, this field is ignored in favor of JWT Auth. |  | Optional: \{\} <br /> |
| `image` _string_ | Image is the container image to use for restore operations.<br />Defaults to the same image used for backup operations if not specified.<br />If the target OpenBaoCluster has image verification enabled, the operator will verify this image and pin the restore Job to the verified digest. |  | MinLength: 1 <br />Optional: \{\} <br /> |
| `force` _boolean_ | Force allows restore even if the cluster appears unhealthy.<br />This is required for disaster recovery scenarios where the cluster<br />may be in a degraded state. | false | Optional: \{\} <br /> |
| `overrideOperationLock` _boolean_ | OverrideOperationLock allows the restore controller to clear an active cluster<br />operation lock (upgrade/backup) and proceed with restore. This is a break-glass<br />escape hatch intended for disaster recovery.<br />For safety, this requires force: true. When used, the controller emits a Warning<br />event and records a Condition on the OpenBaoRestore. | false | Optional: \{\} <br /> |


#### OpenBaoRestoreStatus



OpenBaoRestoreStatus defines the observed state of OpenBaoRestore.



_Appears in:_
- [OpenBaoRestore](#openbaorestore)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `phase` _[RestorePhase](#restorephase)_ | Phase represents the current phase of the restore operation. | Pending | Enum: [Pending Validating Running Completed Failed] <br /> |
| `startTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#time-v1-meta)_ | StartTime is when the restore operation started. |  | Optional: \{\} <br /> |
| `completionTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#time-v1-meta)_ | CompletionTime is when the restore operation completed (success or failure). |  | Optional: \{\} <br /> |
| `snapshotKey` _string_ | SnapshotKey is the key of the snapshot that was restored. |  | Optional: \{\} <br /> |
| `snapshotSize` _integer_ | SnapshotSize is the size of the restored snapshot in bytes. |  | Optional: \{\} <br /> |
| `message` _string_ | Message provides additional details about the current phase. |  | Optional: \{\} <br /> |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#condition-v1-meta) array_ | Conditions represent the latest available observations of the restore's state. |  | Optional: \{\} <br /> |


#### RestorePhase

_Underlying type:_ _string_

RestorePhase represents the current phase of a restore operation.

_Validation:_
- Enum: [Pending Validating Running Completed Failed]

_Appears in:_
- [OpenBaoRestoreStatus](#openbaorestorestatus)

| Field | Description |
| --- | --- |
| `Pending` | RestorePhasePending indicates the restore has been created but not yet started.<br /> |
| `Validating` | RestorePhaseValidating indicates the controller is validating preconditions.<br /> |
| `Running` | RestorePhaseRunning indicates the restore job is executing.<br /> |
| `Completed` | RestorePhaseCompleted indicates the restore completed successfully.<br /> |
| `Failed` | RestorePhaseFailed indicates the restore failed.<br /> |


#### RestoreSource



RestoreSource defines where the snapshot comes from.



_Appears in:_
- [OpenBaoRestoreSpec](#openbaorestorespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `target` _[BackupTarget](#backuptarget)_ | Target reuses BackupTarget for storage connection details.<br />This includes endpoint, bucket, region, credentials, etc. |  |  |
| `key` _string_ | Key is the full path to the snapshot object in the bucket.<br />For example, "clusters/prod/2025-10-14-120000.snap". |  | MinLength: 1 <br /> |


#### WorkloadIdentityConfig



WorkloadIdentityConfig configures cloud workload identity metadata for backup and restore workloads.



_Appears in:_
- [BackupTarget](#backuptarget)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `serviceAccountAnnotations` _object (keys:string, values:string)_ | ServiceAccountAnnotations are merged into the generated backup or restore ServiceAccount.<br />This is typically used for provider-specific bindings such as GKE Workload Identity<br />or webhook-based AWS/Azure workload identity integrations. |  | Optional: \{\} <br /> |
| `podLabels` _object (keys:string, values:string)_ | PodLabels are merged into the generated backup or restore Job pod template.<br />This is typically used for provider-specific selectors such as Azure Workload Identity.<br />Operator-managed labels take precedence if the same key is specified here. |  | Optional: \{\} <br /> |
