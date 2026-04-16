---
title: Upgrade Manager
hide_title: true
pageType: concept
journey: architecture
description: Orchestrate rolling and blue-green upgrades, status-backed resumability, rollback safety, and Raft-aware rollout behavior.
---

<PageHeader
  title="Upgrade orchestration and safety model"
  lede="The upgrade manager owns disruptive version changes. It keeps upgrade orchestration out of the workload loop, persists state in status so upgrades survive controller restarts, and prioritizes cluster availability throughout the rollout."
/>



<ManagerAtAGlance
  sections={[
    {
      label: 'Control path',
      items: [
        'adminops reconciler',
        'internal/app/openbaocluster/adminops',
        'internal/service/upgrade/rolling and internal/service/upgrade/bluegreen',
        'shared seams in internal/service/upgrade/core, snapshot, and raftops',
      ],
    },
    {
      label: 'Owns',
      items: [
        'strategy-specific rolling and blue-green phase orchestration',
        'shared lock, status, metrics, and root-lifecycle mechanics',
        'upgrade executor jobs, snapshot prerequisites, and Raft coordination',
      ],
    },
    {
      label: 'Writes',
      items: [
        'status.upgrade, status.blueGreen, and status.breakGlass through shared status helpers',
        'partition changes, green revision resources, and executor jobs',
        'break-glass and failure state when rollback safety is compromised',
      ],
    },
    {
      label: 'Depends on',
      items: [
        'target version policy and image alignment',
        'backup readiness and network egress for snapshot prerequisites',
        'operation lifecycle coordination for lock, retry, and phase timing',
      ],
    },
  ]}
/>

## Architectural Placement

Upgrade execution belongs to the AdminOps orchestration path:

1. `internal/controller/openbaocluster` receives an adminops reconcile event.
2. The controller delegates to `internal/app/openbaocluster`.
3. AdminOps orchestration invokes either the rolling or blue-green upgrade manager flow.
4. Strategy packages delegate shared mechanics to `internal/service/upgrade/core`,
   `internal/service/upgrade/snapshot`, `internal/service/upgrade/raftops`, and
   `internal/platform/statusapply`.

That keeps upgrade state machines out of the workload loop and lets long-running transitions own their own retry model.

## Package Shape

The upgrade subsystem is split so strategy packages keep workflow ownership while
shared mechanics live behind narrower seams:

- `internal/service/upgrade` keeps root helpers that are shared by both
  strategies but are not strategy-specific or executor-specific, such as
  request parsing, version and image policy, shared metrics types, pod client
  helpers, and root lifecycle helpers.
- `internal/service/upgrade/rolling` owns the rolling state machine: partition
  progression, leader step-down sequencing, per-pod rollout, convergence, and
  rolling-specific retry/failure handling.
- `internal/service/upgrade/bluegreen` owns the blue-green phase machine:
  green deployment, sync/promotion/cutover, rollback, and break-glass handling.
- `internal/service/upgrade/core` owns shared lifecycle mechanics used by
  strategy code, including upgrade locks, common status mutators, metrics
  session bookkeeping, and blue-green status/state helpers that are not tied to
  a single phase.
- `internal/service/upgrade/snapshot` owns shared pre-upgrade snapshot
  preparation: prerequisite validation, runtime bootstrap, Job state modeling,
  and common existing-Job result handling.
- `internal/service/upgrade/raftops` owns executor-side Raft and OpenBao
  coordination such as leader discovery, leader transfer, peer
  join/promote/demote/remove, and autopilot capability fallback.
- `internal/platform/statusapply` owns the shared AdminOps status apply and
  mutate+apply gateways so upgrade, backup, and adminops flows use the same
  status-subresource ownership rules and field-manager boundaries.

<DecisionTable
  kind="decision"
  title="Strategy selection"
  columns={['Strategy', 'Best fit', 'Primary tradeoff']}
  rows={[
    {
      cells: ['Rolling update', 'Default upgrades with minimal extra infrastructure.', 'Lower resource cost, but each pod replacement must preserve Raft health and leader safety.'],
      emphasis: 'recommended',
    },
    {
      cells: ['Blue-green', 'High-control cutovers with explicit promotion and rollback phases.', 'More orchestration and roughly double storage during the transition.'],
      emphasis: 'caution',
    },
  ]}
/>

<Tabs groupId="upgrade-manager-strategies">
  <TabItem value="rolling" label="Rolling update">

<DiagramFrame
  title="Rolling update flow"
  caption="Rolling upgrades use StatefulSet partitioning and leader step-down so each pod can be replaced while Raft remains healthy."
  code={`graph TD
    Trigger["Version change"] --> Partition["Set partition to replicas"]
    Partition --> Loop{"Partition > 0"}
    Loop --> Identify["Identify leader"]
    Identify --> StepDown["Step down if target pod is leader"]
    StepDown --> Update["Decrement partition"]
    Update --> Ready["Wait for pod ready"]
    Ready --> Health["Wait for OpenBao health"]
    Health --> Loop
    Loop --> Converge["Verify StatefulSet and pod convergence"]
    Converge --> Finalize["Atomically finalize status"]

    classDef process fill:transparent,stroke:#fdd0a4,stroke-width:2px,color:#f8fafc;
    classDef write fill:transparent,stroke:#87d6be,stroke-width:2px,color:#e6f4ef;
    classDef read fill:transparent,stroke:#79c0ab,stroke-width:2px,color:#e6f4ef;

    class Trigger,Identify,Ready,Health read;
    class Partition,StepDown,Update,Converge process;
    class Finalize write;`}
/>

<Checklist
  title="Rolling safety controls"
  items={[
    'StatefulSet partitioning pauses Kubernetes-driven rollout until the manager explicitly advances each ordinal.',
    'Reverse ordinal updates and forced leader step-down protect Raft availability during pod replacement.',
    'Finalization only happens after the StatefulSet revision and observed workload health fully converge.',
  ]}
/>

  </TabItem>
  <TabItem value="bluegreen" label="Blue-green">

<Callout type="warning" title="Resource usage">

Blue-green creates a second revision and needs roughly double storage capacity for the duration of the transition.

</Callout>

<DiagramFrame
  title="Blue-green flow"
  caption="Blue-green creates a parallel revision, promotes it through explicit phases, then switches traffic only after leadership and voter transitions are safe."
  code={`graph TD
    Deploy["Deploy green revision"] --> Join["Join green as non-voters"]
    Join --> Sync["Wait for sync and optional hold"]
    Sync --> Promote["Promote green to voters"]
    Promote --> Demote["Demote blue and step down"]
    Demote --> Leader{"Green leader observed?"}
    Leader --> Switch["Cleanup: switch service to green"]
    Switch --> Remove["Remove blue peers"]
    Remove --> Delete["Delete blue StatefulSet"]
    Promote --> Rollback["Late failure triggers rollback"]
    Rollback --> Repair["Repair consensus"]
    Repair --> DeleteGreen["Delete green revision"]

    classDef process fill:transparent,stroke:#fdd0a4,stroke-width:2px,color:#f8fafc;
    classDef write fill:transparent,stroke:#87d6be,stroke-width:2px,color:#e6f4ef;
    classDef critical fill:transparent,stroke:#dc2626,stroke-width:2px,stroke-dasharray: 5 5,color:#f8fafc;

    class Deploy,Join,Sync,Promote,Demote,Remove,Delete,Repair,DeleteGreen process;
    class Switch,Rollback critical;
    class Leader write;`}
/>

<Checklist
  title="Blue-green safety controls"
  tone="warning"
  items={[
    'The service selector switches to green only in cleanup.',
    'Manual promotion, manual rollback, and validation-hook failures all route through explicit phase handling in status.',
    'If rollback consensus repair fails late, the manager enters break-glass and stops risky automation.',
  ]}
/>

  </TabItem>
</Tabs>

## State And Recovery Model

<DecisionTable
  kind="reference"
  title="Status-backed upgrade state"
  columns={['State surface', 'What it preserves']}
  rows={[
    {
      cells: ['status.upgrade', 'Rolling partition progress, completed pods, finalization gating, and the structured failure surface under `status.upgrade.failure` (with deprecated `lastError*` compatibility fields still mirrored for now).'],
      emphasis: 'recommended',
    },
    {
      cells: ['status.blueGreen.phase', 'The active blue-green phase and whether promotion, cleanup, or rollback is in progress.'],
    },
    {
      cells: ['status.upgradeRequests', 'Edge-trigger bookkeeping for retry, promote, and rollback requests so one-shot adminops intent is handled exactly once.'],
    },
    {
      cells: ['status.breakGlass', 'The nonce and diagnostic state when late rollback automation can no longer continue safely.'],
    },
  ]}
/>

<DecisionTable
  kind="reference"
  title="Safety boundaries"
  columns={['Concern', 'Manager behavior']}
  rows={[
    {
      cells: ['Availability over progress', 'Rolling pauses or retries when health is ambiguous; blue-green aborts early and rolls back later phases instead of forcing completion.'],
      emphasis: 'recommended',
    },
    {
      cells: ['Version policy and image alignment', 'Invalid semantic versions, downgrades, and conflicting image/version inputs are rejected before orchestration begins.'],
    },
    {
      cells: ['Backup prerequisites', 'Snapshot prerequisites and backup authentication must already be valid before upgrade safety checks pass.'],
    },
    {
      cells: ['Atomic completion', 'Rolling finalization updates upgrade state and currentVersion together so status does not split across two truths.'],
    },
  ]}
/>

<NextActions
  title="Related deep dives"
  items={[
    {
      label: 'Operation lifecycle coordination',
      description: 'See how lock, retry, and phase-transition helpers are shared with backup and restore.',
      docId: 'architecture/operation-lifecycle',
    },
    {
      label: 'Backup manager',
      description: 'Pre-upgrade snapshots and object-storage readiness are part of the upgrade safety model.',
      docId: 'architecture/backup-manager',
    },
    {
      label: 'Upgrades guide',
      description: 'Compare the internal state machine with the user-facing rolling and blue-green operating procedures.',
      docId: 'user-guide/openbaocluster/operations/upgrades',
    },
  ]}
/>
