---
title: Day 2 Operations
hide_title: true
pageType: concept
journey: architecture
description: Operational lifecycle for upgrades, maintenance controls, and long-running admin operations after the cluster is live.
---

<PageHeader
  title="Hand off from cluster creation into upgrades, maintenance, and long-running operational work."
  lede="Day 2 starts once the cluster is initialized and the workload path is steady. From that point on, long-running operations such as upgrades and backups move through the admin operations path, while maintenance controls gate how much automation is allowed to continue during manual intervention."
/>



<JourneyRail
  current={3}
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
        'an initialized cluster with steady-state workload reconciliation',
        'version drift, backup schedules, or explicit maintenance requests',
        'operation lifecycle coordination available for lock and retry management',
      ],
    },
    {
      label: 'Primary owners',
      items: [
        'adminops controller path',
        'internal/service/upgrade',
        'internal/service/backup and internal/service/opslifecycle',
      ],
    },
    {
      label: 'Writes',
      items: [
        '`status.upgrade`, `status.blueGreen`, and operation-lock state',
        'upgrade and backup executor Jobs plus green revision resources when needed',
        'maintenance annotations or pause-driven no-op behavior depending on user intent',
      ],
    },
    {
      label: 'Hands off to',
      items: [
        'backup and restore flows once a cluster needs ongoing durability',
        'troubleshooting and recovery guides when automation must pause',
        'steady-state workload reconciliation after an operation completes',
      ],
    },
  ]}
/>

## Architectural Placement

Day 2 work is intentionally separated from the high-churn workload loop:

1. Workload reconciliation continues to own the steady-state pod, Service, and config contract.
2. Admin operations orchestration takes over when a change requires long-running coordination such as upgrade or backup.
3. `internal/service/opslifecycle` keeps disruptive operations consistent around lock ownership, retry timing, and audit fields.

That separation prevents upgrades, backups, and other long-running workflows from blocking normal workload repair.

<DiagramFrame
  title="Day 2 control-plane handoff"
  caption="Once the cluster is live, disruptive operations route through the admin operations path instead of staying inside the high-churn workload controller."
  code={`graph TD
    Drift["Version drift or operation request"] --> AdminOps["AdminOps orchestration"]
    AdminOps --> Upgrade["Upgrade manager"]
    AdminOps --> Backup["Backup manager"]
    Upgrade --> Lifecycle["Operation lifecycle"]
    Backup --> Lifecycle
    Lifecycle --> Lock["status.operationLock"]
    Upgrade --> Status["Upgrade and blue-green status"]
    Backup --> BackupStatus["status.backup"]
    Workload["Workload reconcile loop"] --> Ready["Steady-state pod repair"]

    classDef read fill:transparent,stroke:#79c0ab,stroke-width:2px,color:#e6f4ef;
    classDef process fill:transparent,stroke:#fdd0a4,stroke-width:2px,color:#f8fafc;
    classDef write fill:transparent,stroke:#87d6be,stroke-width:2px,color:#e6f4ef;

    class Drift,Workload read;
    class AdminOps,Upgrade,Backup,Lifecycle process;
    class Lock,Status,BackupStatus,Ready write;`}
/>

<DecisionTable
  kind="reference"
  title="Day 2 operation families"
  columns={['Operation family', 'Primary owner', 'Lifecycle role']}
  rows={[
    {
      cells: ['Routine workload repair', 'Workload reconcile path.', 'Keeps StatefulSets, Services, ConfigMaps, and Secrets converged without entering the long-running adminops model.'],
      emphasis: 'recommended',
    },
    {
      cells: ['Upgrade orchestration', 'Upgrade manager via adminops.', 'Handles version drift, strategy-specific state, and Raft-aware cutover logic.'],
    },
    {
      cells: ['Backup scheduling', 'Backup manager via adminops.', 'Runs snapshot Jobs and updates backup status without moving data through the controller.'],
    },
    {
      cells: ['Manual intervention gates', 'User-driven pause and maintenance settings.', 'Limit or reshape automation when an operator needs to intervene directly.'],
    },
  ]}
/>

<Tabs groupId="day2-upgrade-paths">
  <TabItem value="rolling" label="Rolling upgrades">

<Checklist
  title="Rolling path"
  items={[
    'Version drift triggers pre-upgrade validation around semver, health, and optional snapshot prerequisites.',
    'The upgrade manager uses StatefulSet partitioning and leader step-down to replace one pod at a time in reverse ordinal order.',
    'Progress is preserved in status so a failed step can stop cleanly and later resume from an explicit retry request.',
    'Completion updates currentVersion and clears the transient rolling-upgrade state once the workload fully converges.',
  ]}
/>

  </TabItem>
  <TabItem value="bluegreen" label="Blue-green upgrades">

<Checklist
  title="Blue-green path"
  items={[
    'A parallel green revision is created and joined as non-voters before any traffic cutover happens.',
    'Promotion, demotion, cleanup, and rollback all move through explicit phases stored in status.',
    'The Service selector changes only during cleanup, after a green leader is confirmed and blue peers are ready to leave.',
    'If rollback safety breaks down late, the manager enters break-glass instead of continuing risky automation blindly.',
  ]}
/>

  </TabItem>
</Tabs>

<DecisionTable
  kind="reference"
  title="Operational control surfaces"
  columns={['Control', 'What it does', 'When to use it']}
  rows={[
    {
      cells: ['`spec.paused=true`', 'Short-circuits reconcilers so the operator stops mutating managed resources for the cluster.', 'Use when you need manual intervention and want automation to stop entirely.'],
      emphasis: 'recommended',
    },
    {
      cells: ['`spec.maintenance.enabled=true`', 'Keeps reconciliation running, but marks resources for controlled disruptive changes allowed by policy.', 'Use when the operator should continue known-safe automation during maintenance work.'],
    },
    {
      cells: ['`spec.breakGlassAck`', 'Acknowledges an issued nonce before risky late-stage recovery automation can continue.', 'Use only after an operator has reviewed a break-glass condition and accepts the next step explicitly.'],
    },
  ]}
/>

<NextActions
  title="Continue the lifecycle"
  items={[
    {
      label: 'Backups and restore',
      description: 'Move into the durability path that protects live clusters with snapshots and explicit restore requests.',
      docId: 'architecture/lifecycle/dayN-backups',
    },
    {
      label: 'Upgrade manager',
      description: 'Open the deep dive for the exact rolling and blue-green orchestration contract.',
      docId: 'architecture/upgrade-manager',
    },
    {
      label: 'Operate docs',
      description: 'Compare the internal Day 2 control-plane model with the operator-facing upgrade, maintenance, and troubleshooting guides.',
      docId: 'user-guide/openbaocluster/operations/index',
    },
  ]}
/>
