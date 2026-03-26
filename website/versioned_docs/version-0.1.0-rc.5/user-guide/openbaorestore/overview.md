---
title: Restore Overview
description: Understand when restore is the right tool, why it is modeled as OpenBaoRestore, and what safety boundaries it keeps in place.
hide_title: true
pageType: concept
journey: operate
---

<PageHeader
  title="Treat restore as an explicit destructive workflow."
  lede="Restore is modeled as `OpenBaoRestore`, an immutable request object that keeps disaster recovery visible, auditable, and separate from normal cluster reconciliation. Use this page to understand when restore is appropriate and what boundaries it enforces before you run it."
/>

<DecisionTable
  title="Use restore when"
  columns={['Situation', 'Why restore fits', 'Watch for']}
  rows={[
    {
      cells: [
        'Disaster recovery',
        'You need to reintroduce known-good state after severe corruption, cluster loss, or a failed repair path.',
        'Restore overwrites the target cluster. Validate the snapshot and target before you start.',
      ],
      emphasis: 'recommended',
    },
    {
      cells: [
        'Environment cloning',
        'You want to move a realistic snapshot of production state into staging or another controlled environment.',
        'Be deliberate about secrets, tokens, and any sensitive data you are copying.',
      ],
    },
    {
      cells: [
        'Migration between clusters or regions',
        'The restore workflow gives you an explicit, auditable way to land snapshot state on a different target cluster.',
        'The target cluster must already exist and the storage/auth path must work from that environment.',
      ],
    },
    {
      cells: [
        'Ordinary troubleshooting',
        'Restore is usually not the first move. Start with the incident recovery or troubleshooting guides first.',
        'Do not overwrite state while a narrower repair path is still viable.',
      ],
    },
  ]}
/>

<DiagramFrame
  title="Restore control flow"
  caption="A restore request is validated, acquires the cluster operation lock, then launches a restore Job that pulls the snapshot and injects it into the target cluster. The workflow is explicit so destructive recovery does not hide inside normal reconciliation."
  code={`flowchart LR
    Request["OpenBaoRestore request"] --> Validate["Validate target, source, and auth"]
    Validate --> Lock["Acquire restore lock"]
    Lock --> Job["Launch restore Job"]
    Job --> Pull["Pull snapshot from object storage"]
    Pull --> Apply["Restore into target OpenBao cluster"]
    Apply --> Status["Record terminal status and release lock"]

    classDef read fill:transparent,stroke:#79c0ab,stroke-width:2px,color:#e6f4ef;
    classDef process fill:transparent,stroke:#fdd0a4,stroke-width:2px,color:#e6f4ef;
    classDef write fill:transparent,stroke:#87d6be,stroke-width:2px,color:#e6f4ef;

    class Request,Validate read;
    class Lock,Job,Pull,Apply process;
    class Status write;`}
/>

<DecisionTable
  kind="reference"
  title="What OpenBaoRestore guarantees"
  columns={['Contract', 'Why it exists']}
  rows={[
    {
      cells: [
        'Explicit request object',
        'Restore is visible and auditable instead of being hidden inside cluster status or imperative scripts.',
      ],
      emphasis: 'recommended',
    },
    {
      cells: [
        'Immutable spec',
        'Changing restore intent requires a new request so the audit trail stays stable and the controller does not reinterpret a running job.',
      ],
    },
    {
      cells: [
        'Operation lock ownership',
        'Restore blocks conflicting upgrades and backups while destructive work is in flight.',
      ],
    },
    {
      cells: [
        'Separate restore identity',
        'The restore Job authenticates separately from the main workload so recovery credentials are deliberate rather than inherited accidentally.',
      ],
    },
  ]}
/>

<Callout type="warning" title="Restore overwrites cluster state">

Restore is not a read-only diagnostic step. It replaces the target cluster state with the contents of the selected snapshot and may still leave the cluster needing follow-up work such as unseal or Raft repair.

</Callout>

## Minimal restore request

<CommandBlock
  language="yaml"
  label="configure"
  title="Create a minimal OpenBaoRestore request"
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
  The target `OpenBaoCluster` and the `OpenBaoRestore` request live in the same namespace. To change the restore intent later, create a new resource instead of editing the running one.
</CommandBlock>

<NextActions
  title="Continue with the restore path"
  items={[
    {
      label: 'Run a restore',
      description: 'Use the task page when you are ready to configure the source, auth path, and verification steps in full.',
      docId: 'user-guide/openbaorestore/restore',
    },
    {
      label: 'Recover after upgrade restore',
      description: 'Use the override-lock runbook only when a failed upgrade blocks the normal restore workflow.',
      docId: 'user-guide/openbaorestore/recovery-restore-after-upgrade',
    },
    {
      label: 'Restore manager architecture',
      description: 'See how validation, operation locks, and restore job execution fit together inside the controller.',
      docId: 'architecture/restore-manager',
    },
  ]}
/>
