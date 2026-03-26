---
title: Day N Backups And Restore
hide_title: true
pageType: concept
journey: architecture
description: Backup and restore lifecycle for live clusters, including snapshot scheduling, status surfaces, retention, and explicit restore requests.
---

<PageHeader
  title="Protect live clusters with scheduled snapshots and explicit restore requests."
  lede="Once the cluster is running in production, durability becomes its own lifecycle. The backup manager schedules and records snapshot Jobs, the restore manager handles destructive restore requests through a separate CRD path, and both rely on shared operation-lock coordination so they do not collide with upgrades."
/>

<JourneyRail
  current={4}
  title="Lifecycle phases"
  items={[
    {
      label: 'Day 0 provisioning',
      description: 'Prepare a namespace boundary and tenant-scoped policy before any cluster exists.',
      docId: 'architecture/lifecycle/day0-provisioning',
    },
    {
      label: 'Day 1 creation',
      description: 'Bootstrap the first node, initialize safely, and only then scale out.',
      docId: 'architecture/lifecycle/day1-creation',
    },
    {
      label: 'Day 2 operations',
      description: 'Hand off into upgrades, maintenance, and long-running operational workflows.',
      docId: 'architecture/lifecycle/day2-operations',
    },
    {
      label: 'Backups and restore',
      description: 'Protect data durability with scheduled snapshots and explicit restore requests.',
      docId: 'architecture/lifecycle/dayN-backups',
    },
  ]}
/>

<ManagerAtAGlance
  sections={[
    {
      label: 'Starts with',
      items: [
        'a live initialized cluster and an optional `spec.backup` schedule',
        'object-storage configuration plus backup authentication',
        'explicit `OpenBaoRestore` requests when destructive recovery is needed',
      ],
    },
    {
      label: 'Primary owners',
      items: [
        'internal/service/backup',
        'internal/service/restore',
        'internal/service/opslifecycle',
      ],
    },
    {
      label: 'Writes',
      items: [
        'backup executor Jobs and `status.backup` timing and failure state',
        '`OpenBaoRestore` phase progression and cluster operation-lock ownership',
        'retention cleanup after successful uploads and restore job launch/cleanup state',
      ],
    },
    {
      label: 'Hands off to',
      items: [
        'normal steady-state operation when backups succeed',
        'post-restore follow-up when a restore request completes',
        'operator-facing backup, restore, and recovery procedures',
      ],
    },
  ]}
/>

## Architectural Placement

Durability work is shared across two explicit operation surfaces:

1. The backup manager lives on the adminops path and handles scheduled, manual, and pre-upgrade snapshot jobs.
2. The restore manager runs through the dedicated `OpenBaoRestore` controller path so destructive recovery stays explicit and auditable.
3. `internal/service/opslifecycle` supplies shared lock and retry behavior so backups, restores, and upgrades coordinate instead of colliding.

That model keeps backup routine and restore exceptional, even though both exist in the same durability phase of the lifecycle.

<DiagramFrame
  title="Day N durability loop"
  caption="Backups produce durable recovery points during live operation; restore consumes one of those recovery points through a separate request path when the cluster needs destructive recovery."
  code={`graph TD
    Schedule["Backup schedule or manual trigger"] --> Backup["Backup manager"]
    Backup --> Job["Snapshot executor Job"]
    Job --> Storage["Object storage"]
    Job --> BackupStatus["status.backup"]
    Storage --> RestoreReq["OpenBaoRestore request"]
    RestoreReq --> Restore["Restore manager"]
    Restore --> Lock["status.operationLock"]
    Restore --> RestoreJob["Restore Job"]
    RestoreJob --> Cluster["Cluster restored or follow-up required"]

    classDef read fill:transparent,stroke:#79c0ab,stroke-width:2px,color:#e6f4ef;
    classDef process fill:transparent,stroke:#fdd0a4,stroke-width:2px,color:#f8fafc;
    classDef write fill:transparent,stroke:#87d6be,stroke-width:2px,color:#e6f4ef;
    classDef critical fill:transparent,stroke:#dc2626,stroke-width:2px,color:#f8fafc;

    class Schedule,RestoreReq read;
    class Backup,Job,Restore,RestoreJob process;
    class Storage,BackupStatus,Lock write;
    class Cluster critical;`}
/>

<DecisionTable
  kind="reference"
  title="Durability surfaces"
  columns={['Surface', 'Primary owner', 'Purpose']}
  rows={[
    {
      cells: ['`spec.backup`', 'Backup manager consumes it.', 'Declares schedule, provider target, auth wiring, and retention policy for routine snapshots.'],
      emphasis: 'recommended',
    },
    {
      cells: ['`status.backup`', 'Backup manager writes it.', 'Records last attempt, next schedule, last success, and consecutive failures so durability is visible without inspecting Jobs.'],
    },
    {
      cells: ['`OpenBaoRestore`', 'Restore manager consumes and updates it.', 'Keeps restore explicit, immutable, and auditable instead of hiding destructive recovery inside cluster status.'],
    },
    {
      cells: ['`status.operationLock`', 'Shared via opslifecycle.', 'Blocks conflicting upgrade, backup, or restore work while one disruptive operation is in flight.'],
    },
  ]}
/>

<DecisionTable
  kind="reference"
  title="Safety boundaries"
  columns={['Concern', 'Durability behavior']}
  rows={[
    {
      cells: ['Backup during disruptive work', 'Scheduled backups do not start while upgrades or restore are already active on the same cluster.'],
      emphasis: 'recommended',
    },
    {
      cells: ['Authentication surface', 'Backup and restore use dedicated auth wiring such as JWT roles or explicit token references; root tokens are not the durability mechanism.'],
    },
    {
      cells: ['Restore visibility', 'Restore is modeled as a separate CRD-backed request so destructive recovery has its own audit trail and phase status.'],
    },
    {
      cells: ['Retention timing', 'Retention cleanup runs only after a successful backup so older recovery points are not removed before a new one exists.'],
    },
  ]}
/>

<NextActions
  title="Related durability pages"
  items={[
    {
      label: 'Backup manager',
      description: 'Open the deep dive for scheduled backup execution, status, and retention details.',
      docId: 'architecture/backup-manager',
    },
    {
      label: 'Restore manager',
      description: 'Open the deep dive for explicit restore requests, lock ownership, and destructive workflow handling.',
      docId: 'architecture/restore-manager',
    },
    {
      label: 'Restore guide',
      description: 'Compare the internal durability model with the operator-facing restore and recovery procedures.',
      docId: 'user-guide/openbaorestore/restore',
    },
  ]}
/>
