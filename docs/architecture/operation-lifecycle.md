---
title: Operation Lifecycle Coordination
hide_title: true
pageType: concept
journey: architecture
description: Shared lock, retry, and phase-audit primitives used by backup, restore, and upgrade managers.
---

<PageHeader
  title="Operation lifecycle coordination"
  lede="`internal/service/opslifecycle` is the shared service-layer contract behind backup, restore, and upgrade orchestration. It does not own a controller or CRD of its own. Instead, it keeps operation lock identity, retry timing, and phase audit logging consistent whenever a manager needs to take disruptive action against a cluster."
/>



<ManagerAtAGlance
  sections={[
    {
      label: 'Used by',
      items: [
        'internal/service/backup',
        'internal/service/restore',
        'internal/service/upgrade',
      ],
    },
    {
      label: 'Owns',
      items: [
        'operation-lock identity helpers for disruptive work',
        'retry intent classes and default requeue mapping',
        'phase-transition audit field normalization',
      ],
    },
    {
      label: 'Writes through',
      items: [
        'internal/service/opslifecycle for status.operationLock updates',
        'audit event fields for phase transitions',
        'shared retry delays consumed by controller requeues',
      ],
    },
    {
      label: 'Depends on',
      items: [
        'OpenBaoCluster.status.operationLock as the persisted mutex surface',
        'controller requeue behavior for long-running progress polling',
        'manager-specific phase names and audit metadata',
      ],
    },
  ]}
/>

## Architectural placement

Operation lifecycle coordination sits below the concrete managers and owns the shared lock write path:

1. A manager such as backup, restore, or upgrade decides it needs to start or resume work.
2. It uses `internal/service/opslifecycle` to acquire or release the expected lock identity, classify retry intent, and log phase changes.
3. `opslifecycle` applies `status.operationLock` directly through the shared SSA lock plane.

That keeps the shared safety model in one place instead of scattering lock and retry semantics across several managers.

<DecisionTable
  kind="reference"
  title="OpenBaoCluster status ownership planes"
  columns={['Plane', 'Field manager', 'Owned status fields']}
  rows={[
    {
      cells: ['Observed status', '`openbao-status-controller`', '`status.observedGeneration`, `status.phase`, `status.activeLeader`, `status.readyReplicas`, `status.readReplicas`, `status.currentVersion`, `status.conditions`'],
      emphasis: 'recommended',
    },
    {
      cells: ['Workload status', '`openbao-workload-controller`', '`status.initialized`, `status.selfInitialized`, `status.workload`'],
    },
    {
      cells: ['AdminOps status', '`openbao-adminops-controller`', '`status.acceptedUpgradeStrategy`, `status.upgrade`, `status.upgradeRequests`, `status.backup`, `status.blueGreen`, `status.breakGlass`, `status.adminOps`'],
    },
    {
      cells: ['Operation lock status', '`openbao-operationlock-controller`', '`status.operationLock`'],
    },
  ]}
/>

## Server-side apply status contract

Each status plane has one server-side apply field manager. The split prevents an observed-status write from
claiming workload or AdminOps fields, but it does not make fragments within one plane independent. A writer that
shares a field manager must read and apply that manager's complete plane.

<DecisionTable
  kind="reference"
  title="Status write rules"
  columns={['Concern', 'Required behavior', 'Why']}
  rows={[
    {
      cells: [
        'Ownership plane',
        'Use the field manager assigned in the status ownership table and apply only that manager\'s fields.',
        'Separate managers preserve sibling planes and make ownership conflicts diagnosable.',
      ],
      emphasis: 'recommended',
    },
    {
      cells: [
        'Shared AdminOps plane',
        'Read the latest object, mutate one concern, and apply the full AdminOps plane.',
        'Omitting sibling fields from a later apply by the same manager can clear them. Fresh-read and concurrency tests protect upgrade, backup, blue-green, break-glass, and AdminOps state from each other.',
      ],
    },
    {
      cells: [
        'Immediate readback',
        'Use a fresh API read after apply when the same reconcile decision depends on the committed value.',
        'The controller-runtime cache can lag the API server after a status write.',
      ],
    },
    {
      cells: [
        'Operation lock clear',
        'Clear through the dedicated operation-lock manager and use explicit ownership takeover only when legacy or external ownership conflicts.',
        'Lock removal must not silently steal unrelated status ownership; force is a conflict recovery path, not the normal write mode.',
      ],
    },
  ]}
/>

The preservation guarantee has two parts: separate field managers protect sibling status planes, while a
fresh-read, full-plane mutation protects sibling fields that intentionally share the AdminOps manager.

<DiagramFrame
  title="Coordination model"
  caption="Backup, restore, and upgrade do not each implement their own lock and retry policy. They share one coordination service that owns the lock write path and keeps audit fields consistent."
  code={`graph TD
    Backup["Backup manager"] --> Ops["Operation lifecycle"]
    Restore["Restore manager"] --> Ops
    Upgrade["Upgrade manager"] --> Ops
    Ops --> Lock["Operation lock status writer"]
    Ops --> Retry["Retry classes"]
    Ops --> Audit["Phase audit logging"]
    Lock --> Status["OpenBaoCluster.status.operationLock"]

    classDef read fill:transparent,stroke:#79c0ab,stroke-width:2px,color:#e6f4ef;
    classDef process fill:transparent,stroke:#fdd0a4,stroke-width:2px,color:#f8fafc;
    classDef write fill:transparent,stroke:#87d6be,stroke-width:2px,color:#e6f4ef;

    class Backup,Restore,Upgrade read;
    class Ops,Retry,Audit process;
    class Lock process;
    class Status write;`}
/>

<DecisionTable
  kind="reference"
  title="Shared primitives"
  columns={['Primitive', 'What it standardizes', 'Why it exists']}
  rows={[
    {
      cells: ['OperationLock', 'A stable holder + operation identity for a long-running action.', 'Managers need an exact lock identity so renew and release only succeed for the intended owner.'],
      emphasis: 'recommended',
    },
    {
      cells: ['Acquire / Release', 'Status-based lock ownership via the adapter with a fresh read-before-write gateway.', 'Controllers should not each patch status.operationLock differently, rely on stale cached objects, or invent different lock messages.'],
    },
    {
      cells: ['IsLockHeld / HeldError / AddHeldAuditFields', 'A shared way to classify contention and enrich audit events with who currently owns the lock.', 'Contention should produce consistent diagnostics instead of manager-specific strings.'],
    },
    {
      cells: ['LogPhaseTransition', 'Stable phase_from / phase_to audit fields for long-running operations.', 'Audit streams stay comparable across backup, restore, and upgrade.'],
    },
  ]}
/>

## Retry and lock model

<DecisionTable
  kind="reference"
  title="Retry classes"
  columns={['Retry class', 'Default delay', 'Typical use']}
  rows={[
    {
      cells: ['lock-contention', '`5s`', 'Another disruptive operation already owns the cluster lock, so the manager should requeue quickly and check again.'],
      emphasis: 'recommended',
    },
    {
      cells: ['progress-poll', '`5s`', 'A Job or long-running operation is still in progress and the manager is waiting for the next observable state change.'],
    },
    {
      cells: ['standard', '`1m` by default, overridable with `OPENBAO_REQUEUE_STANDARD`', 'Background retry work that does not need tight polling.'],
    },
  ]}
/>

<DecisionTable
  kind="reference"
  title="Lock contract"
  columns={['Concern', 'Shared behavior']}
  rows={[
    {
      cells: ['Acquire vs renew', 'If the exact holder and operation already own the lock, acquisition renews the same lock instead of treating it as contention.'],
      emphasis: 'recommended',
    },
    {
      cells: ['Exact-match release', 'Release succeeds only when holder and operation match the active lock, so one manager cannot accidentally clear another manager’s ownership.'],
    },
    {
      cells: ['Legacy takeover', 'The adapter only forces ownership when a clear or explicit override hits an SSA ownership conflict, so normal lock renewals stay non-destructive.'],
    },
    {
      cells: ['Force override', 'Force semantics exist for explicit override paths only; normal long-running operations should not silently steal the lock.'],
    },
    {
      cells: ['Contention diagnostics', 'HeldError exposes the current operation and holder so audit events and logs can explain why a manager requeued.'],
    },
  ]}
/>

<Callout type="note" title="This is coordination, not orchestration">

`opslifecycle` does not decide whether an upgrade should roll or blue-green, whether a restore request is valid, or whether a backup target is reachable. It only standardizes the lock, retry, and audit mechanics around those domain decisions.

</Callout>

<NextActions
  title="Related deep dives"
  items={[
    {
      label: 'Upgrade manager',
      description: 'See how rolling and blue-green orchestration use shared lock and retry primitives during long-running transitions.',
      docId: 'architecture/upgrade-manager',
    },
    {
      label: 'Backup manager',
      description: 'See how scheduled and manual snapshot flows reuse the same contention and requeue model.',
      docId: 'architecture/backup-manager',
    },
    {
      label: 'Restore manager',
      description: 'See how destructive restore requests rely on the same lock identity and audit mechanics.',
      docId: 'architecture/restore-manager',
    },
  ]}
/>
