---
title: Restore Manager
hide_title: true
pageType: concept
journey: architecture
description: Reconcile OpenBaoRestore requests, acquire operation locks, and orchestrate restore jobs as explicit destructive workflows.
---

<PageHeader
  title="Treat restore as a destructive, explicit, lock-aware workflow."
  lede="The restore manager keeps disaster recovery separate from normal cluster reconciliation. It models restore as an immutable CRD-backed request, coordinates execution through a dedicated controller path, and protects the cluster with explicit validation and lock ownership."
/>

<ManagerAtAGlance
  sections={[
    {
      label: 'Control path',
      items: [
        'dedicated openbaorestore controller',
        'internal/app/openbaorestore',
        'internal/service/restore',
      ],
    },
    {
      label: 'Owns',
      items: [
        'restore request validation',
        'operation lock lifecycle for restore',
        'restore job creation and terminal cleanup',
      ],
    },
    {
      label: 'Writes',
      items: [
        'OpenBaoRestore phase progression',
        'OpenBaoCluster.status.operationLock for restore ownership',
        'restore job launch and cleanup state',
      ],
    },
    {
      label: 'Depends on',
      items: [
        'snapshot source accessibility',
        'restore authentication and token strategy',
        'backup provider configuration and cluster lock state',
      ],
    },
  ]}
/>

## Request Model

<DecisionTable
  kind="reference"
  title="Restore request contract"
  columns={['Contract', 'Why it exists']}
  rows={[
    {
      cells: ['CRD-based request', 'Restore is visible, declarative, and auditable instead of being hidden inside OpenBaoCluster status or imperative scripts.'],
      emphasis: 'recommended',
    },
    {
      cells: ['Immutable spec', 'Changing restore inputs requires a new request so the audit trail and execution intent stay stable.'],
    },
    {
      cells: ['Stateless controller', 'The controller polls the restore job rather than depending on broad watch permissions across every child object.'],
    },
    {
      cells: ['Operation lock ownership', 'Restore must block upgrades and backups while destructive data-plane changes are in flight.'],
    },
  ]}
/>

## Restore Lifecycle

<DiagramFrame
  title="Restore lifecycle"
  caption="Restore validates first, acquires the cluster lock second, and only then launches a restore job. Terminal phases keep retrying lock cleanup until the cluster is no longer marked as restore-owned."
  code={`graph TD
    Start["OpenBaoRestore created"] --> Pending["Pending"]
    Pending --> Validate{"Validate request"}
    Validate --> Failed["Failed"]
    Validate --> Lock{"Acquire restore lock"}
    Lock --> Pending
    Lock --> Running["Running"]
    Running --> Job["Launch restore job"]
    Job --> Pull["Pull snapshot"]
    Pull --> Apply["Restore to OpenBao"]
    Apply --> Completed["Completed"]
    Apply --> Retry{"Retry?"}
    Retry --> Job
    Retry --> Failed

    classDef read fill:transparent,stroke:#79c0ab,stroke-width:2px,color:#e6f4ef;
    classDef process fill:transparent,stroke:#fdd0a4,stroke-width:2px,color:#f8fafc;
    classDef write fill:transparent,stroke:#87d6be,stroke-width:2px,color:#e6f4ef;
    classDef critical fill:transparent,stroke:#dc2626,stroke-width:2px,color:#f8fafc;

    class Start read;
    class Pending,Validate,Lock,Running,Job,Pull,Retry process;
    class Completed write;
    class Failed,Apply critical;`}
/>

<DecisionTable
  kind="reference"
  title="Restore phases"
  columns={['Phase', 'Manager intent']}
  rows={[
    {
      cells: ['Pending / Validating', 'Reject invalid target clusters, inaccessible snapshots, missing auth, and unsafe conflicting operations before anything destructive starts.'],
      emphasis: 'recommended',
    },
    {
      cells: ['Running', 'Launch the restore job after the restore lock is owned and the request is known-good.'],
    },
    {
      cells: ['Completed', 'Release the lock and preserve the restore record as the audit trail of what happened.'],
    },
    {
      cells: ['Failed', 'Expose terminal failure while continuing lock cleanup on later reconciles until the cluster is no longer marked as restore-owned.'],
    },
  ]}
/>

## Safety Boundaries

<DecisionTable
  kind="reference"
  title="Safety boundaries"
  columns={['Concern', 'Manager behavior']}
  rows={[
    {
      cells: ['Conflicting operations', 'Backups and upgrades are blocked by the restore operation lock while restore is active.'],
      emphasis: 'recommended',
    },
    {
      cells: ['Emergency override', 'Override requires explicit force semantics rather than silently ignoring a stuck or conflicting lock.'],
    },
    {
      cells: ['Execution surface', 'The controller delegates the destructive work to a job instead of embedding restore logic in normal reconcile loops.'],
    },
    {
      cells: ['After restore', 'The manager may leave the cluster requiring unseal or follow-up recovery work; completion only means the restore workflow finished.'],
    },
  ]}
/>

<Callout type="warning" title="Restore is not routine reconciliation">

Restore is intentionally modeled outside the normal `OpenBaoCluster` lifecycle. The operator treats it as a destructive recovery operation with its own request object, its own controller path, and its own lock semantics.

</Callout>

<NextActions
  title="Related deep dives"
  items={[
    {
      label: 'Backup manager',
      description: 'Restore depends on the snapshot and object-storage path owned by the backup manager.',
      docId: 'architecture/backup-manager',
    },
    {
      label: 'Operation lifecycle coordination',
      description: 'See how restore shares lock and retry primitives with other disruptive operations.',
      docId: 'architecture/operation-lifecycle',
    },
    {
      label: 'Restore guide',
      description: 'Compare the internal restore controller model with the user-facing restore and recovery procedures.',
      docId: 'user-guide/openbaorestore/restore',
    },
  ]}
/>
