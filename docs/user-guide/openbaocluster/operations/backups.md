---
title: Backup Operations
description: Configure backup jobs, object-storage auth, retention, and verification for snapshot-based recovery.
slug: /operate/backups
hide_title: true
pageType: task
journey: operate
---

<PageHeader
  title="Backup operations and snapshot policy"
  lede="OpenBao Operator runs backups as transient Jobs that authenticate separately from the main workload, stream Raft snapshots directly to object storage, and record schedule and failure state on the cluster. Use this page to configure auth, storage, schedules, retention, and verification."
/>



<DiagramFrame
  title="Backup execution path"
  caption="A schedule or manual trigger launches a stateless Job. The Job authenticates to OpenBao, streams the Raft snapshot directly, and uploads it to object storage without sending snapshot bytes through the controller."
  code={`flowchart LR
    Trigger["Cron or manual trigger"] --> Job["Backup Job"]
    Job --> Auth["Authenticate to OpenBao"]
    Auth --> Snapshot["Stream Raft snapshot"]
    Snapshot --> Upload["Upload to object storage"]
    Upload --> Status["Update backup status and retention"]

    classDef write fill:transparent,stroke:#87d6be,stroke-width:2px,color:#e6f4ef;
    classDef read fill:transparent,stroke:#79c0ab,stroke-width:2px,color:#e6f4ef;
    classDef process fill:transparent,stroke:#fdd0a4,stroke-width:2px,color:#f8fafc;

    class Trigger read;
    class Job,Auth,Snapshot,Upload process;
    class Status write;`}
/>

<DecisionTable
  title="Choose the backup auth path"
  columns={['Path', 'Use it when', 'Operator behavior', 'Watch for']}
  rows={[
    {
      cells: [
        'JWT auth',
        'You can enable `selfInit.oidc` or already run the JWT auth method on the cluster.',
        'The operator uses a projected ServiceAccount token and can auto-configure the backup auth role when OIDC bootstrap is enabled.',
        'Keep the JWT audience aligned between the controller env vars and the OpenBao role.',
      ],
      emphasis: 'recommended',
    },
    {
      cells: [
        'Static token',
        'JWT auth is not available yet and you need a compatibility path.',
        'The backup Job reads a long-lived token from a Secret in the cluster namespace.',
        'This is a compatibility path. Treat the token as a sensitive credential and rotate it deliberately.',
      ],
      emphasis: 'caution',
    },
  ]}
/>

## Prerequisites

- Provision a bucket or container in a supported provider:
  - S3 or S3-compatible storage such as MinIO or Ceph
  - Google Cloud Storage
  - Azure Blob Storage
- Grant the backup identity write access to that storage location.
- Allow egress to the storage endpoint. This is required for the `Hardened` profile.
- Decide whether the backup and restore Jobs will use a Secret, explicit workload identity metadata, or provider-default credentials.

<Callout type="note" title="Separate identity surfaces">

The main OpenBao Pods and backup Jobs use different ServiceAccounts.
Cloud KMS unseal identity on the main workload does not automatically apply to backup or restore Jobs.
Check `CloudUnsealIdentityReady` for the main Pods and `BackupConfigurationReady` for the generated backup Job identity path.

</Callout>

<Callout type="warning" title="Custom backup images need delegated RBAC">

Setting `spec.backup.image` selects the executable used by backup Jobs. The identity applying that `OpenBaoCluster` needs `usecustomexecutables` on the cluster; existing `usehelperimages` bindings remain accepted for compatibility.

</Callout>

## First successful backup path

<DecisionTable
  title="Recommended first backup path"
  columns={['Step', 'What to do', 'What proves success']}
  rows={[
    {
      cells: [
        '1. Pick the auth path',
        'Use JWT auth when `spec.selfInit.oidc.enabled=true` or deliberately create the equivalent restore/backup roles yourself. Fall back to a static token only when JWT auth is not available.',
        'You know whether the Job will authenticate with a projected ServiceAccount token or a Secret-backed token.',
      ],
      emphasis: 'recommended',
    },
    {
      cells: [
        '2. Configure storage target',
        'Choose S3, GCS, or Azure and make the credentials or workload identity path explicit.',
        'The cluster spec contains a complete `spec.backup.target` and the referenced Secret or workload identity metadata already exists.',
      ],
    },
    {
      cells: [
        '3. Wait for backup readiness',
        'Apply the updated `OpenBaoCluster` and check status before assuming the CronJob can run.',
        '`BackupConfigurationReady=True` and no storage or identity validation failures remain.',
      ],
    },
    {
      cells: [
        '4. Force one manual run',
        'Trigger a backup from the generated CronJob before the first upgrade window.',
        'A real snapshot lands in object storage and `status.backup.lastSuccessfulBackup` advances.',
      ],
    },
  ]}
/>

<Callout type="tip" title="For most first-time production users">

The cleanest first backup path is:

1. enable `spec.selfInit.oidc.enabled: true`
2. configure `spec.backup.target`
3. wait for `BackupConfigurationReady=True`
4. trigger one manual backup and confirm the object exists in storage

</Callout>

## Configure backup auth and storage

<Tabs groupId="backup-auth-path">

<TabItem value="jwt-auth" label="JWT auth (Recommended)">

Use JWT auth when you want automatic token rotation and the cleanest separation between the cluster workload and backup jobs.

<Callout type="success" title="Automated setup">

When `spec.selfInit.oidc.enabled` is `true`, the operator automatically configures:

1. the JWT auth method (`auth/jwt-operator`)
2. OIDC discovery
3. the backup policy (`openbao-operator-backup`)
4. the backup role (`openbao-operator-backup`)

No manual OpenBao auth configuration is required.

</Callout>

<CommandBlock
  language="yaml"
  label="configure"
  title="Enable OIDC bootstrap for automatic backup auth"
  code={`spec:
  selfInit:
    enabled: true
    oidc:
      enabled: true`}
/>

<Callout type="note" title="JWT audience">

The backup Job uses the audience from `OPENBAO_JWT_AUDIENCE` (default: `openbao-internal`).
Set the same value in the OpenBao role `bound_audiences` and pass the env var to the operator
through `controller.extraEnv` and `provisioner.extraEnv` in Helm.

</Callout>

<Tabs groupId="backup-provider-jwt">

<TabItem value="s3" label="S3">

<CommandBlock
  language="yaml"
  label="configure"
  title="Configure S3 or S3-compatible storage"
  code={`apiVersion: openbao.org/v1alpha1
kind: OpenBaoCluster
metadata:
  name: backup-cluster
spec:
  backup:
    schedule: "0 3 * * *"
    target:
      provider: s3
      endpoint: "https://s3.amazonaws.com"
      bucket: "openbao-backups"
      region: "us-east-1"
      pathPrefix: "clusters/backup-cluster"
      usePathStyle: false
      # Optional explicit web identity path:
      # roleArn: "arn:aws:iam::123456789012:role/openbao-backup"
      # Optional provider metadata for the generated ServiceAccount:
      # workloadIdentity:
      #   serviceAccountAnnotations:
      #     eks.amazonaws.com/role-arn: "arn:aws:iam::123456789012:role/openbao-backup"
      credentialsSecretRef:
        name: s3-credentials`}
/>

<Callout type="note" title="S3 credentials">

Create a Secret with these keys when you are not using provider-default identity:

- `accessKeyId`
- `secretAccessKey`
- `sessionToken` (optional)
- `region` (optional)
- `caCert` (optional)

You can also omit `credentialsSecretRef` and rely on:

- `roleArn` for the operator-managed web identity flow
- ambient workload identity or default credentials
- `workloadIdentity.serviceAccountAnnotations` when your platform integration is driven by ServiceAccount metadata

</Callout>

</TabItem>

<TabItem value="gcs" label="GCS">

<CommandBlock
  language="yaml"
  label="configure"
  title="Configure Google Cloud Storage"
  code={`apiVersion: openbao.org/v1alpha1
kind: OpenBaoCluster
metadata:
  name: backup-cluster
spec:
  backup:
    schedule: "0 3 * * *"
    target:
      provider: gcs
      bucket: "openbao-backups"
      pathPrefix: "clusters/backup-cluster"
      gcs:
        project: "my-gcp-project"
      # Optional Workload Identity metadata for the generated ServiceAccount:
      # workloadIdentity:
      #   serviceAccountAnnotations:
      #     iam.gke.io/gcp-service-account: "backup@my-project.iam.gserviceaccount.com"
      credentialsSecretRef:
        name: gcs-credentials`}
/>

<CommandBlock
  language="bash"
  label="apply"
  title="Create the GCS credentials Secret"
  code={`kubectl create secret generic gcs-credentials \\
  --from-file=credentials.json=/path/to/service-account-key.json`}
>
  Omit `credentialsSecretRef` when you intentionally rely on Application Default Credentials or Workload Identity instead of a static service-account key.
</CommandBlock>

</TabItem>

<TabItem value="azure" label="Azure">

<CommandBlock
  language="yaml"
  label="configure"
  title="Configure Azure Blob Storage"
  code={`apiVersion: openbao.org/v1alpha1
kind: OpenBaoCluster
metadata:
  name: backup-cluster
spec:
  backup:
    schedule: "0 3 * * *"
    target:
      provider: azure
      bucket: "openbao-backups"
      pathPrefix: "clusters/backup-cluster"
      azure:
        storageAccount: "mystorageaccount"
        container: "openbao-backups"
      # Optional workload identity metadata:
      # workloadIdentity:
      #   serviceAccountAnnotations:
      #     azure.workload.identity/client-id: "00000000-0000-0000-0000-000000000000"
      #   podLabels:
      #     azure.workload.identity/use: "true"
      credentialsSecretRef:
        name: azure-credentials`}
/>

<Callout type="note" title="Azure credentials">

Create a Secret with one of the following:

- `accountKey`
- `connectionString`

For managed identity or Azure Workload Identity, omit `credentialsSecretRef`.
If your cluster integration requires Kubernetes metadata, use:

- `target.workloadIdentity.serviceAccountAnnotations`
- `target.workloadIdentity.podLabels`

</Callout>

</TabItem>

</Tabs>

</TabItem>

<TabItem value="static-token" label="Static token (Legacy)">

Use this path only when JWT auth is not available. The backup Job reads a long-lived OpenBao token from a Secret.

<Callout type="note" title="Same-namespace requirement">

All referenced Secrets must exist in the same namespace as the `OpenBaoCluster`. Cross-namespace references are not allowed.

</Callout>

<CommandBlock
  language="bash"
  label="apply"
  title="Create the backup token Secret"
  code={`kubectl create secret generic backup-token \\
  --from-literal=token=hvs.yourtoken...`}
/>

<Tabs groupId="backup-provider-static">

<TabItem value="s3" label="S3">

<CommandBlock
  language="yaml"
  label="configure"
  title="Configure S3 backup with a static token"
  code={`apiVersion: openbao.org/v1alpha1
kind: OpenBaoCluster
metadata:
  name: backup-cluster
spec:
  backup:
    schedule: "0 3 * * *"
    tokenSecretRef:
      name: backup-token
    target:
      provider: s3
      endpoint: "https://s3.amazonaws.com"
      bucket: "openbao-backups"
      region: "us-east-1"
      credentialsSecretRef:
        name: s3-credentials`}
/>

</TabItem>

<TabItem value="gcs" label="GCS">

<CommandBlock
  language="yaml"
  label="configure"
  title="Configure GCS backup with a static token"
  code={`apiVersion: openbao.org/v1alpha1
kind: OpenBaoCluster
metadata:
  name: backup-cluster
spec:
  backup:
    schedule: "0 3 * * *"
    tokenSecretRef:
      name: backup-token
    target:
      provider: gcs
      bucket: "openbao-backups"
      gcs:
        project: "my-gcp-project"
      credentialsSecretRef:
        name: gcs-credentials`}
/>

</TabItem>

<TabItem value="azure" label="Azure">

<CommandBlock
  language="yaml"
  label="configure"
  title="Configure Azure backup with a static token"
  code={`apiVersion: openbao.org/v1alpha1
kind: OpenBaoCluster
metadata:
  name: backup-cluster
spec:
  backup:
    schedule: "0 3 * * *"
    tokenSecretRef:
      name: backup-token
    target:
      provider: azure
      bucket: "openbao-backups"
      azure:
        storageAccount: "mystorageaccount"
      credentialsSecretRef:
        name: azure-credentials`}
/>

</TabItem>

</Tabs>

</TabItem>

</Tabs>

## Minimal working example

<CommandBlock
  language="yaml"
  label="configure"
  title="Use a minimal JWT-backed S3 backup baseline"
  code={`apiVersion: openbao.org/v1alpha1
kind: OpenBaoCluster
metadata:
  name: my-cluster
  namespace: openbao-prod
spec:
  selfInit:
    enabled: true
    oidc:
      enabled: true
  backup:
    schedule: "0 3 * * *"
    target:
      provider: s3
      endpoint: "https://s3.amazonaws.com"
      bucket: "openbao-backups"
      region: "us-east-1"
      pathPrefix: "clusters/my-cluster"
      credentialsSecretRef:
        name: s3-credentials`}
>
  This is the smallest supported production-oriented starting point. The namespace must already contain the referenced Secret, and the backup Job still needs network egress to the object storage endpoint.
</CommandBlock>

## Advanced backup settings

### Provider-specific options

<Tabs groupId="backup-provider-options">

<TabItem value="s3-options" label="S3">

<DecisionTable
  kind="reference"
  title="S3-specific options"
  columns={['Option', 'Default', 'What it changes']}
  rows={[
    {
      cells: ['`region`', '`us-east-1`', 'Sets the AWS region or any placeholder value needed by an S3-compatible implementation.'],
      emphasis: 'recommended',
    },
    {
      cells: ['`usePathStyle`', '`false`', 'Switch to path-style addressing for MinIO and some S3-compatible endpoints.'],
    },
    {
      cells: ['`roleArn`', 'none', 'Enables the explicit AWS web identity path managed by the operator.'],
    },
    {
      cells: ['`pathPrefix`', 'cluster-scoped default', 'Controls the object prefix used for backup keys so clusters stay separated inside a shared bucket.'],
    },
  ]}
/>

<CommandBlock
  language="yaml"
  label="configure"
  title="Set S3 provider-specific options"
  code={`spec:
  backup:
    target:
      provider: s3
      region: "eu-west-1"
      usePathStyle: true
      roleArn: "arn:aws:iam::123456789012:role/backup-role"
      pathPrefix: "clusters/prod-a"`}
/>

</TabItem>

<TabItem value="gcs-options" label="GCS">

<DecisionTable
  kind="reference"
  title="GCS-specific options"
  columns={['Option', 'What it changes']}
  rows={[
    {
      cells: ['`project`', 'Pins the GCP project when credentials or ADC do not already provide it.'],
      emphasis: 'recommended',
    },
    {
      cells: ['`endpoint`', 'Overrides the storage endpoint for emulators such as `fake-gcs-server`.'],
    },
  ]}
/>

<CommandBlock
  language="yaml"
  label="configure"
  title="Set GCS provider-specific options"
  code={`spec:
  backup:
    target:
      provider: gcs
      endpoint: "http://fake-gcs-server:4443"
      gcs:
        project: "my-gcp-project"`}
/>

</TabItem>

<TabItem value="azure-options" label="Azure">

<DecisionTable
  kind="reference"
  title="Azure-specific options"
  columns={['Option', 'What it changes']}
  rows={[
    {
      cells: ['`storageAccount`', 'Selects the Azure storage account. This is required when `provider: azure` is used.'],
      emphasis: 'recommended',
    },
    {
      cells: ['`container`', 'Overrides the container name when it should differ from `bucket`.'],
    },
    {
      cells: ['`endpoint`', 'Overrides the blob endpoint for testing tools such as Azurite.'],
    },
  ]}
/>

<CommandBlock
  language="yaml"
  label="configure"
  title="Set Azure provider-specific options"
  code={`spec:
  backup:
    target:
      provider: azure
      endpoint: "http://127.0.0.1:10000"
      azure:
        storageAccount: "mystorageaccount"
        container: "backups"`}
/>

</TabItem>

</Tabs>

### Workload identity metadata

Use `target.workloadIdentity` when your cloud identity integration depends on ServiceAccount annotations or pod labels on the generated backup and restore workloads.

<CommandBlock
  language="yaml"
  label="configure"
  title="Attach identity metadata to backup and restore workloads"
  code={`spec:
  backup:
    target:
      workloadIdentity:
        serviceAccountAnnotations:
          iam.gke.io/gcp-service-account: "backup@my-project.iam.gserviceaccount.com"
          azure.workload.identity/client-id: "00000000-0000-0000-0000-000000000000"
        podLabels:
          azure.workload.identity/use: "true"`}
>
  `serviceAccountAnnotations` are applied to the generated backup and restore ServiceAccounts.
  `podLabels` are applied to backup and restore Job pods without replacing operator-managed labels.
</CommandBlock>

<Callout type="tip" title="Emulator support">

GCS and Azure support custom endpoints for local testing with `fake-gcs-server` and Azurite.
When those endpoints use self-signed certificates, include the CA certificate in the credentials Secret.

</Callout>

### Retention policy

Retention cleanup runs after a successful backup and works across S3, GCS, and Azure.

<CommandBlock
  language="yaml"
  label="configure"
  title="Keep a limited number of recent snapshots"
  code={`spec:
  backup:
    retention:
      maxCount: 7
      maxAge: "168h"`}
/>

### Performance tuning

<DecisionTable
  kind="reference"
  title="Multipart upload tuning"
  columns={['Parameter', 'Default', 'When to change it']}
  rows={[
    {
      cells: ['`partSize`', '`10MB`', 'Increase it for high-bandwidth links and large datasets when larger chunks reduce upload overhead.'],
      emphasis: 'recommended',
    },
    {
      cells: ['`concurrency`', '`3`', 'Increase for throughput, or reduce it when memory pressure or object-store throttling becomes the limiting factor.'],
    },
  ]}
/>

<CommandBlock
  language="yaml"
  label="configure"
  title="Tune upload chunking and parallelism"
  code={`spec:
  backup:
    target:
      partSize: 20971520
      concurrency: 5`}
/>

### Pre-upgrade snapshots

Take a snapshot immediately before the rolling update or blue-green cutover begins.

<CommandBlock
  language="yaml"
  label="configure"
  title="Require a snapshot before upgrades start"
  code={`spec:
  upgrade:
    preUpgradeSnapshot: true
  backup:
    target: { ... }`}
/>

<Callout type="note" title="Upgrade safety">

`preUpgradeSnapshot: true` only works when `spec.backup.target` is already configured.
Confirm backup status before you start the upgrade rather than assuming the pre-upgrade snapshot can be taken on demand.

</Callout>

## Verify and operate

<CommandBlock
  language="bash"
  label="verify"
  title="Check backup readiness"
  code={`kubectl get openbaocluster my-cluster -n <namespace> \\
  -o jsonpath='{range .status.conditions[*]}{.type}={.status}{\"\\n\"}{end}'`}
>
  Confirm `BackupConfigurationReady=True` before you rely on the schedule or trigger a manual run.
</CommandBlock>

<CommandBlock
  language="bash"
  label="inspect"
  title="Inspect backup status on the cluster"
  code={`kubectl get openbaocluster my-cluster \\
  -o jsonpath='{.status.backup}'`}
>
  Check `lastSuccessfulBackup`, `nextScheduledBackup`, and failure counters before you rely on the policy as a recovery control.
</CommandBlock>

<CommandBlock
  language="bash"
  label="apply"
  title="Trigger a manual backup from the generated CronJob"
  code={`kubectl create job --from=cronjob/my-cluster-backup manual-backup-1`}
>
  Use a manual run to prove the full path: identity, cluster auth, storage reachability, and object naming before the first production upgrade.
</CommandBlock>

## Official OpenBao background

- [OpenBao Backups](https://openbao.org/docs/concepts/storage/#backups)
- [Operator Raft Command](https://openbao.org/docs/commands/operator/raft/)
- [JWT/OIDC Auth Method](https://openbao.org/docs/auth/jwt/)

<NextActions
  title="Next operating steps"
  items={[
    {
      label: 'Restore from backup',
      description: 'Use the restore guide when you need to consume one of the snapshots this page configures.',
      docId: 'user-guide/openbaorestore/restore',
    },
    {
      label: 'Plan upgrades',
      description: 'Validate backups as part of pre-upgrade snapshots and cutover safety.',
      docId: 'user-guide/openbaocluster/operations/upgrades',
    },
    {
      label: 'Open the production checklist',
      description: 'Use the checklist to confirm backups, restore readiness, and day 2 controls before calling the cluster production-ready.',
      docId: 'user-guide/openbaocluster/operations/production-checklist',
    },
  ]}
/>
