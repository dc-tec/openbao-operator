---
title: Backup Manager
hide_title: true
pageType: concept
journey: architecture
description: Schedule snapshot jobs, enforce retention, and update backup status while keeping snapshot data transport out of the controller.
---

<PageHeader
  title="Backup manager workflow"
  lede="The backup manager owns scheduled and manual snapshot orchestration for `OpenBaoCluster`. It validates cluster readiness, acquires the operation lock, creates executor Jobs, and records backup state so backups stay auditable and resumable while snapshot transport stays outside the controller."
/>



<ManagerAtAGlance
  sections={[
    {
      label: 'Control path',
      items: [
        'adminops reconciler',
        'internal/app/openbaocluster/adminops',
        'internal/service/backup',
      ],
    },
    {
      label: 'Owns',
      items: [
        'backup trigger detection for schedules and manual requests',
        'preflight validation and operation-lock ownership for backup',
        'retention evaluation after successful uploads',
      ],
    },
    {
      label: 'Writes',
      items: [
        'backup executor Jobs and job annotations',
        'status.backup timing, success, and failure counters',
        'operation lock state while backup is in progress',
      ],
    },
    {
      label: 'Depends on',
      items: [
        'cluster health and absence of conflicting upgrade or restore work',
        'spec.backup target, authentication, and executor image configuration',
        'object-storage reachability and trust configuration for the selected provider',
      ],
    },
  ]}
/>

## Architectural Placement

Backup orchestration belongs to the AdminOps path:

1. `internal/controller/openbaocluster` receives an adminops reconcile event.
2. The controller delegates into `internal/app/openbaocluster/adminops`.
3. AdminOps orchestration invokes `internal/service/backup` to validate, launch, and observe backup execution.

That keeps the controller focused on reconcile plumbing while the backup manager owns timing, job launch, and retention decisions.

<DecisionTable
  kind="reference"
  title="Owned surfaces"
  columns={['Surface', 'What the manager decides', 'Why it matters']}
  rows={[
    {
      cells: ['Backup trigger window', 'Whether a cron window, manual trigger, or pre-upgrade request should launch a new Job.', 'Backups need at-most-once behavior per scheduled window and predictable manual overrides.'],
      emphasis: 'recommended',
    },
    {
      cells: ['Executor Job', 'Job name, annotations, auth wiring, and provider-specific environment for the backup binary.', 'The controller should schedule work, not stream snapshot data itself.'],
    },
    {
      cells: ['status.backup', 'Attempt timing, next schedule, last success, and consecutive failure state.', 'Operators need backup visibility without inspecting transient Jobs.'],
    },
    {
      cells: ['Retention policy', 'Which completed backups can be deleted after a successful upload.', 'Retention belongs to the control plane so cleanup stays consistent across providers.'],
    },
  ]}
/>

## Backup Flow

<DiagramFrame
  title="Validate, launch, then record"
  caption="The backup manager validates cluster state first, launches a stateless Job second, and only updates backup status after the Job reaches a terminal result."
  code={`graph TD
    Trigger["Cron or manual trigger"] --> Validate{"Preflight checks"}
    Validate --> Retry["Requeue without launch"]
    Validate --> Lock["Acquire backup operation lock"]
    Lock --> Job["Create backup Job"]
    Job --> Auth["Authenticate to OpenBao"]
    Auth --> Snapshot["Stream Raft snapshot"]
    Snapshot --> Upload["Upload to object storage"]
    Upload --> Result{"Job result"}
    Result --> Success["Patch lastSuccessfulBackup and next schedule"]
    Result --> Failure["Increment consecutiveFailures and record attempt"]
    Success --> Retention["Apply retention policy"]

    classDef read fill:transparent,stroke:#79c0ab,stroke-width:2px,color:#e6f4ef;
    classDef process fill:transparent,stroke:#fdd0a4,stroke-width:2px,color:#f8fafc;
    classDef write fill:transparent,stroke:#87d6be,stroke-width:2px,color:#e6f4ef;
    classDef critical fill:transparent,stroke:#dc2626,stroke-width:2px,color:#f8fafc;

    class Trigger read;
    class Validate,Lock,Job,Auth,Snapshot,Upload,Retention process;
    class Success write;
    class Failure critical;`}
/>

<DecisionTable
  kind="reference"
  title="Preflight and status model"
  columns={['Check', 'Manager behavior']}
  rows={[
    {
      cells: ['Cluster readiness', 'Backup launches only when the cluster is in a stable running phase and the workload is not already mid-transition.'],
      emphasis: 'recommended',
    },
    {
      cells: ['Conflicting operations', 'Restore and active upgrade state block backup launch; only one long-running operation may own the cluster lock at a time.'],
    },
    {
      cells: ['At-most-once scheduling', 'status.backup.lastAttemptScheduledTime and nextScheduledBackup prevent duplicate launches in the same cron window.'],
    },
    {
      cells: ['Failure accounting', 'Consecutive failures increase only when a terminal Job fails, not on every reconcile that notices the same failed Job.'],
    },
  ]}
/>

## Provider And Retention Surfaces

<DecisionTable
  kind="reference"
  title="Provider integration surfaces"
  columns={['Provider family', 'Auth patterns the manager supports', 'What stays the same']}
  rows={[
    {
      cells: ['S3-compatible', 'Static access keys, explicit web identity, ambient workload identity, or ServiceAccount annotation-driven identity.', 'The manager still creates one executor Job and records status the same way after upload completes.'],
      emphasis: 'recommended',
    },
    {
      cells: ['GCS', 'Service account key, Application Default Credentials, or Workload Identity metadata on the generated pod identity.', 'Upload and retention stay job-driven; only the credential wiring changes.'],
    },
    {
      cells: ['Azure Blob Storage', 'Account key, connection string, or managed identity/workload identity defaults.', 'Retention and backup naming stay provider-agnostic at the manager boundary.'],
    },
  ]}
/>

Backups are stored under a stable object prefix so restore workflows can locate artifacts without reverse-engineering Job names:

```text
<pathPrefix>/<namespace>/<cluster>/<timestamp>-<short-uuid>.snap
```

<DecisionTable
  kind="reference"
  title="Safety boundaries"
  columns={['Concern', 'Manager behavior']}
  rows={[
    {
      cells: ['No data-plane coupling', 'The controller never handles snapshot bytes directly; the executor Job performs authentication, snapshot, and upload work.'],
      emphasis: 'recommended',
    },
    {
      cells: ['Retention timing', 'Retention runs only after a successful upload so cleanup never removes older recovery points before a new one exists.'],
    },
    {
      cells: ['Upgrade coordination', 'Pre-upgrade snapshots reuse backup job machinery rather than creating a second snapshot implementation in the upgrade manager.'],
    },
    {
      cells: ['Local buffering risk', 'The backup path is designed around streaming to object storage rather than writing large transient snapshot files inside the controller.'],
    },
  ]}
/>

<NextActions
  title="Related deep dives"
  items={[
    {
      label: 'Restore manager',
      description: 'Restore consumes the snapshot contract that backup writes and protects with lock ownership.',
      docId: 'architecture/restore-manager',
    },
    {
      label: 'Upgrade manager',
      description: 'Pre-upgrade snapshots depend on the same backup execution surface instead of a separate snapshot implementation.',
      docId: 'architecture/upgrade-manager',
    },
    {
      label: 'Backups guide',
      description: 'Compare the internal backup orchestration model with the user-facing schedule, provider, and restore instructions.',
      docId: 'user-guide/openbaocluster/operations/backups',
    },
  ]}
/>
