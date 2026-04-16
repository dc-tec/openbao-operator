---
title: Restore from Backup
description: Create an OpenBaoRestore request, wire snapshot access and restore auth, and verify the destructive recovery workflow deliberately.
hide_title: true
pageType: task
journey: operate
---

<PageHeader
  title="Restore from a snapshot"
  lede="The operator restores snapshot state through an explicit `OpenBaoRestore` request. That request validates the target, acquires the restore operation lock, launches a dedicated restore Job, and records the outcome for audit. Use this page when snapshot restore is the right recovery workflow for the target cluster."
/>



<Callout type="danger" title="Restore overwrites the target cluster">

Restore replaces the target cluster state with the contents of the selected snapshot. All current secrets, policies, auth methods, and keys in that cluster are treated as replaceable state. Confirm the target namespace, target cluster, and snapshot key before you apply the request.

</Callout>

<DecisionTable
  title="First successful restore path"
  columns={['Step', 'What to do first', 'What proves you are ready']}
  rows={[
    {
      cells: [
        '1. Prove the backup path first',
        'Do not start here if the snapshot was never validated. Use the backup guide to confirm the object exists and the storage/auth path is already known-good.',
        'You know the exact snapshot key and can point to a successful backup object in storage.',
      ],
      emphasis: 'recommended',
    },
    {
      cells: [
        '2. Prepare the target cluster deliberately',
        'Keep the target `OpenBaoCluster` in the same namespace, and make sure it already exists and can be reached by the restore Job.',
        'The target cluster resource exists, the namespace is correct, and the restore Job can reach object storage.',
      ],
    },
    {
      cells: [
        '3. Choose restore auth',
        'Prefer JWT auth through `spec.selfInit.oidc.enabled=true` or a deliberate restore role. Use a static token only as a compatibility fallback.',
        'You know whether the restore Job will use `jwtAuthRole` or `tokenSecretRef` and the referenced auth path already exists.',
      ],
    },
    {
      cells: [
        '4. Watch the restore and cluster together',
        'Apply the `OpenBaoRestore`, then inspect both the restore resource and the target cluster until the destructive step and follow-up recovery state are clear.',
        'The restore reaches a terminal phase and you know whether the next step is normal service, unseal recovery, or Raft repair.',
      ],
    },
  ]}
/>

<DecisionTable
  title="Choose the restore auth path"
  columns={['Path', 'Use it when', 'Operator behavior', 'Watch for']}
  rows={[
    {
      cells: [
        'JWT auth',
        'You already use `selfInit.oidc` or can provision a dedicated restore JWT role on the target cluster.',
        'The restore Job uses a projected ServiceAccount token and authenticates with a short-lived, scoped identity.',
        'Keep the JWT audience and bound ServiceAccount subject aligned with the restore role.',
      ],
      emphasis: 'recommended',
    },
    {
      cells: [
        'Static token',
        'JWT auth is not available yet and you need a compatibility path for recovery.',
        'The restore Job reads a long-lived OpenBao token from a Secret in the same namespace.',
        'Treat the token as a high-sensitivity credential and rotate it intentionally.',
      ],
    },
  ]}
/>

<DiagramFrame
  title="Restore execution path"
  caption="The controller validates the request, acquires the restore lock, launches a restore Job, downloads the selected snapshot, and then forces the snapshot into the target cluster leader."
  code={`flowchart LR
    Request["OpenBaoRestore"] --> Validate["Validate target, source, and auth"]
    Validate --> Lock["Acquire restore lock"]
    Lock --> Job["Launch restore Job"]
    Job --> Auth["Authenticate to target cluster"]
    Auth --> Download["Download snapshot"]
    Download --> Restore["POST /sys/storage/raft/snapshot-force"]
    Restore --> Status["Record terminal phase and release lock"]

    classDef read fill:transparent,stroke:#79c0ab,stroke-width:2px,color:#e6f4ef;
    classDef process fill:transparent,stroke:#fdd0a4,stroke-width:2px,color:#e6f4ef;
    classDef write fill:transparent,stroke:#87d6be,stroke-width:2px,color:#e6f4ef;

    class Request,Validate read;
    class Lock,Job,Auth,Download,Restore process;
    class Status write;`}
/>

## Prepare the restore request

- Make sure the snapshot you want already exists in object storage and has been validated as usable.
- Create or keep the target `OpenBaoCluster` in the same namespace as the restore request. The target cluster must exist and be initialized, even if it is otherwise empty.
- Verify the restore Job can reach the object storage endpoint. In hardened environments, this usually means explicit `spec.network.egressRules`.
- Choose the restore auth path before the incident starts. The restore Job does not automatically inherit the cloud identity or auth material from the main OpenBao Pods.

<Callout type="note" title="Same-namespace requirement">

The `OpenBaoRestore`, the target `OpenBaoCluster`, and any referenced token Secrets all live in the same namespace. Cross-namespace references are intentionally blocked for security reasons.

</Callout>

<Callout type="tip" title="For most first-time restores">

If the target cluster already uses `spec.selfInit.oidc.enabled=true`, start with `jwtAuthRole: openbao-operator-restore` and the same object-storage provider shape you already proved in the backup flow.
That is the shortest path from a tested backup to a tested restore.

</Callout>

## Configure the snapshot source

<Tabs groupId="restore-source-provider">

<TabItem value="s3" label="S3">

<CommandBlock
  language="yaml"
  label="configure"
  title="Configure an S3 or S3-compatible restore source"
  code={`spec:
  cluster: prod-cluster
  source:
    target:
      provider: s3
      endpoint: https://s3.amazonaws.com
      bucket: openbao-backups
      region: us-east-1
      usePathStyle: false
      credentialsSecretRef:
        name: s3-credentials
    key: clusters/prod/backup-2026-03-20.snap`}
>
  The Secret can provide `accessKeyId`, `secretAccessKey`, `sessionToken`, `region`, and an optional `caCert` for custom trust chains.
</CommandBlock>

</TabItem>

<TabItem value="gcs" label="GCS">

<CommandBlock
  language="yaml"
  label="configure"
  title="Configure a Google Cloud Storage restore source"
  code={`spec:
  cluster: prod-cluster
  source:
    target:
      provider: gcs
      bucket: openbao-backups
      gcs:
        project: my-gcp-project
      credentialsSecretRef:
        name: gcs-credentials
    key: clusters/prod/backup-2026-03-20.snap`}
>
  The Secret typically contains `credentials.json`. You can also use a provider-default identity path when your environment already supports it.
</CommandBlock>

</TabItem>

<TabItem value="azure" label="Azure">

<CommandBlock
  language="yaml"
  label="configure"
  title="Configure an Azure Blob Storage restore source"
  code={`spec:
  cluster: prod-cluster
  source:
    target:
      provider: azure
      bucket: openbao-backups
      azure:
        storageAccount: mystorageaccount
        container: openbao-backups
      credentialsSecretRef:
        name: azure-credentials
    key: clusters/prod/backup-2026-03-20.snap`}
>
  The Secret can provide an `accountKey` or a `connectionString`. For workload identity or managed identity, omit the credentials Secret and rely on the platform-native path intentionally.
</CommandBlock>

</TabItem>

</Tabs>

## Configure restore auth

<Tabs groupId="restore-auth-path">

<TabItem value="jwt" label="JWT auth (Recommended)">

<CommandBlock
  language="yaml"
  label="configure"
  title="Use a JWT role for restore"
  code={`spec:
  jwtAuthRole: openbao-operator-restore`}
>
  When `spec.selfInit.oidc.enabled=true` on the target cluster, the operator can create and use the default restore role automatically. Otherwise, create the role deliberately and keep the JWT audience aligned with `OPENBAO_JWT_AUDIENCE`.
</CommandBlock>

</TabItem>

<TabItem value="token" label="Static token">

<CommandBlock
  language="yaml"
  label="configure"
  title="Use a static token Secret for restore"
  code={`spec:
  tokenSecretRef:
    name: restore-token`}
>
  Use this path only when JWT auth is not available. The Secret must be in the same namespace as the restore request.
</CommandBlock>

</TabItem>

</Tabs>

## Apply the restore request

<CommandBlock
  language="yaml"
  label="configure"
  title="Create a full restore request"
  code={`apiVersion: openbao.org/v1alpha1
kind: OpenBaoRestore
metadata:
  name: prod-restore
  namespace: security
spec:
  cluster: prod-cluster
  source:
    target:
      provider: s3
      endpoint: https://s3.amazonaws.com
      bucket: openbao-backups
      region: us-east-1
      credentialsSecretRef:
        name: s3-credentials
    key: clusters/prod/backup-2026-03-20.snap
  jwtAuthRole: openbao-operator-restore`}
>
  The restore spec is immutable. If you need to change the target, snapshot key, or auth path, create a new `OpenBaoRestore` resource instead of editing the existing one.
</CommandBlock>

<CommandBlock
  language="bash"
  label="apply"
  title="Apply the restore"
  code={`kubectl apply -f restore.yaml`}
/>

## Watch the restore phases

<CommandBlock
  language="bash"
  label="verify"
  title="Inspect restore status"
  code={`kubectl get openbaorestore <name> -n <namespace> -o yaml
kubectl get jobs -n <namespace>
kubectl describe openbaocluster <cluster> -n <namespace>`}
>
  Watch the restore resource and the target cluster together. The restore phase tells you what the restore controller is doing, while the cluster status shows what still needs follow-up after the destructive step finishes.
</CommandBlock>

<DecisionTable
  kind="reference"
  title="Restore phases to expect"
  columns={['Phase', 'What it means']}
  rows={[
    {
      cells: [
        'Pending / Validating',
        'The operator is checking the target cluster, storage source, auth path, and conflicting operations before it starts destructive work.',
      ],
      emphasis: 'recommended',
    },
    {
      cells: [
        'Running',
        'The restore Job owns the cluster operation lock and is actively downloading or applying the snapshot.',
      ],
    },
    {
      cells: [
        'Completed',
        'The restore workflow finished and the operator recorded the result. The cluster may still need unseal or follow-up recovery work.',
      ],
    },
    {
      cells: [
        'Failed',
        'The restore did not succeed. Inspect the restore resource, related Job, and target cluster state before retrying with a new request.',
      ],
    },
  ]}
/>

<NextActions
  title="After the restore"
  items={[
    {
      label: 'Recover a sealed cluster',
      description: 'Go here when the restored cluster starts but remains sealed because the trust or unseal path still needs attention.',
      docId: 'user-guide/openbaocluster/recovery/sealed-cluster',
    },
    {
      label: 'Recover from no leader',
      description: 'Go here when the restored cluster needs Raft peer or leadership repair before it can return to service.',
      docId: 'user-guide/openbaocluster/recovery/no-leader',
    },
    {
      label: 'Recover after upgrade restore',
      description: 'Use the override-lock path only when a failed upgrade or rollback blocks the normal restore request.',
      docId: 'user-guide/openbaorestore/recovery-restore-after-upgrade',
    },
  ]}
/>
