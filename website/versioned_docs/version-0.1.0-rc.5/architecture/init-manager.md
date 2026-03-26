---
title: Init Manager
hide_title: true
pageType: concept
journey: architecture
description: Bootstrap a new cluster safely, handle self-init or operator init flows, and configure autopilot before scaling out.
---

<PageHeader
  title="Initialize one node first, then scale only after the cluster is safe to join."
  lede="The init manager owns the first-boot contract for a new `OpenBaoCluster`. It keeps bootstrap on a single node, handles operator-driven or self-init flows, stores or suppresses root material appropriately, and configures Raft autopilot before the workload expands to full replica count."
/>

<ManagerAtAGlance
  sections={[
    {
      label: 'Control path',
      items: [
        'workload reconciler',
        'internal/controller/openbaocluster',
        'internal/service/init',
      ],
    },
    {
      label: 'Owns',
      items: [
        'bootstrap detection and first init call behavior',
        'root token handling or self-init completion detection',
        'initial autopilot configuration immediately after cluster initialization',
      ],
    },
    {
      label: 'Writes',
      items: [
        'status.initialized and status.selfInitialized',
        'root token Secret when self-init is disabled',
        'autopilot configuration through the OpenBao API once initialization completes',
      ],
    },
    {
      label: 'Depends on',
      items: [
        'single-replica bootstrap from the infrastructure path',
        'pod readiness and TLS Secret availability before init proceeds',
        'self-init requests and auth bootstrap configuration when self-init is enabled',
      ],
    },
  ]}
/>

## Architectural Placement

Initialization stays on the workload-side controller path while the cluster is not yet ready for normal steady-state reconciliation:

1. `internal/controller/openbaocluster` keeps the cluster on the uninitialized path.
2. The controller calls `internal/service/init` once the first pod and TLS prerequisites are ready.
3. The init manager marks initialization state, configures autopilot, and only then allows the infrastructure path to scale to the requested replica count.

That separation prevents first-boot logic from leaking into every steady-state reconcile.

## Bootstrap Flow

<DiagramFrame
  title="Initialize, then scale"
  caption="The infrastructure path holds the workload at one replica until the init manager confirms the cluster is initialized. Only then does the cluster expand to the requested replica count."
  code={`sequenceDiagram
    participant Ctrl as OpenBaoCluster controller
    participant Infra as Infrastructure manager
    participant Pod0 as Pod-0
    participant Init as Init manager
    participant Bao as OpenBao API
    participant Status as Cluster status

    Ctrl->>Infra: Render StatefulSet at replicas=1
    Infra->>Pod0: Start first pod
    Pod0-->>Init: Pod ready + TLS available
    Init->>Bao: Detect initialized or call /sys/init
    Bao-->>Init: Init response or initialized health
    Init->>Status: Set initialized / selfInitialized
    Init->>Bao: Configure autopilot defaults
    Status-->>Infra: Initialization confirmed
    Infra->>Infra: Scale StatefulSet to spec.replicas
  `}
/>

## Initialization Phases

<Tabs groupId="init-manager-phases-versioned">
  <TabItem value="bootstrap" label="Bootstrap one node">

<Checklist
  title="Bootstrap contract"
  items={[
    'A new cluster starts at one replica even when spec.replicas is greater than one.',
    'The infrastructure manager keeps the StatefulSet capped until status.initialized becomes true.',
    'This avoids race conditions where multiple uninitialized pods could compete to become the first Raft leader.',
  ]}
/>

  </TabItem>
  <TabItem value="initialize" label="Initialize safely">

<Checklist
  title="Initialization contract"
  items={[
    'The manager first checks for an already initialized cluster and skips the init call when status or health proves bootstrap already happened.',
    'When self-init is disabled, it performs the init call, captures the root material once, and stores the root token in a Secret without logging the response.',
    'When self-init is enabled, it treats pod readiness and initialization signals as the completion boundary and does not create a root-token Secret.',
  ]}
/>

  </TabItem>
  <TabItem value="scale" label="Scale after success">

<Checklist
  title="Scale-out contract"
  items={[
    'The manager sets status.initialized after the cluster is known-good and, when relevant, also sets status.selfInitialized.',
    'Autopilot defaults are configured immediately after initialization so day-2 health policy exists before the cluster grows.',
    'Only after that handoff does the workload path expand the StatefulSet and let additional pods join through retry_join.',
  ]}
/>

  </TabItem>
</Tabs>

## Autopilot Defaults

<DecisionTable
  kind="reference"
  title="Autopilot configuration defaults"
  columns={['Setting', 'Default behavior', 'Why the init manager sets it early']}
  rows={[
    {
      cells: ['cleanup_dead_servers', 'Enabled by default, but forced off when minQuorum < 3 and the user did not explicitly override it.', 'The rendered policy must remain valid for small clusters before steady-state operations begin.'],
      emphasis: 'recommended',
    },
    {
      cells: ['dead_server_last_contact_threshold', '5m', 'Dead-peer cleanup should wait long enough to avoid reacting to short network turbulence.'],
    },
    {
      cells: ['last_contact_threshold', '10s', 'Autopilot needs a consistent heartbeat tolerance before higher replica counts join.'],
    },
    {
      cells: ['server_stabilization_time', '10s', 'New members should remain stable briefly before being treated as healthy participants.'],
    },
    {
      cells: ['max_trailing_logs', '1000', 'Replication lag needs a default budget before dead-server or readiness logic starts treating peers as unhealthy.'],
    },
    {
      cells: ['min_quorum', 'Hardened profile defaults to 3, or replicas when replicas > 3; other profiles use max(1, replicas).', 'The cleanup policy and quorum safety model must be aligned from the first initialized reconcile.'],
    },
  ]}
/>

<Callout type="warning" title="Already initialized is recovery, not import">

If the manager detects that a cluster is already initialized, it takes the initialized-cluster path as recovery for an operator-managed cluster. It is not a generic import path for arbitrary unmanaged OpenBao clusters.

</Callout>

<DecisionTable
  kind="reference"
  title="Safety boundaries"
  columns={['Concern', 'Manager behavior']}
  rows={[
    {
      cells: ['Split-brain at bootstrap', 'Single-pod bootstrap stays in force until initialization is confirmed, so the first Raft leader forms in a controlled way.'],
      emphasis: 'recommended',
    },
    {
      cells: ['Root material handling', 'The init response is used in-memory only for the current request and is not logged; self-init intentionally avoids creating a root-token Secret.'],
    },
    {
      cells: ['TLS readiness', 'Initialization waits for the TLS server Secret when TLS is managed by the operator so the API path is not used before the workload is ready.'],
    },
    {
      cells: ['Invalid autopilot cleanup', 'The manager forces cleanupDeadServers off for small-cluster configurations that OpenBao would reject.'],
    },
  ]}
/>

<NextActions
  title="Related deep dives"
  items={[
    {
      label: 'Infrastructure manager',
      description: 'See how the workload path holds the StatefulSet at one replica and then scales out after init succeeds.',
      docId: 'architecture/infra-manager',
    },
    {
      label: 'Self-init guide',
      description: 'Compare the internal self-init behavior with the user-facing bootstrap requests and auth setup.',
      docId: 'user-guide/openbaocluster/configuration/self-init',
    },
    {
      label: 'Day 1 lifecycle flow',
      description: 'Follow where initialization fits in the broader cluster-creation sequence.',
      docId: 'architecture/lifecycle/day1-creation',
    },
  ]}
/>
