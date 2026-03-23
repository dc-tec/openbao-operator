---
title: Day 1 Creation
hide_title: true
pageType: concept
journey: architecture
description: Cluster creation flow from OpenBaoCluster creation through TLS bootstrap, one-node initialization, autopilot configuration, and scale-out.
---

<PageHero
  variant="compact"
  eyebrow="Architecture / Lifecycle / Day 1"
  title="Bootstrap one node, initialize safely, then scale to the requested cluster shape."
  lede="Day 1 begins when `OpenBaoCluster` is created. The control plane bootstraps TLS and unseal prerequisites, renders the workload, keeps the StatefulSet at one replica for safe initialization, then hands off into steady-state operations only after the cluster is known-good."
  actions={[
    {label: 'Open init manager', docId: 'architecture/init-manager', variant: 'primary'},
    {label: 'Open self-init guide', docId: 'user-guide/openbaocluster/configuration/self-init', variant: 'secondary'},
  ]}
>
  <Checklist
    title="Use this page when you need to"
    items={[
      'compare self-init and standard initialization from the controller perspective',
      'see how cert, infra, and init managers cooperate during first boot',
      'understand why the cluster starts at one replica regardless of the requested size',
      'trace how initialization state becomes a safe handoff into day 2 operations',
    ]}
  />
</PageHero>

<JourneyRail
  current={2}
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
        '`OpenBaoCluster` creation and the requested replica count',
        'TLS mode, unseal mode, and optional self-init configuration',
        'a provisioned namespace boundary from Day 0 when multi-tenancy is in use',
      ],
    },
    {
      label: 'Primary owners',
      items: [
        'internal/service/certs',
        'internal/service/infra',
        'internal/service/init',
      ],
    },
    {
      label: 'Writes',
      items: [
        'TLS Secrets, trust-bundle surfaces, and rendered `config.hcl`',
        'single-replica StatefulSet followed by scale-out after initialization',
        '`status.initialized`, `status.selfInitialized`, and initial autopilot configuration',
      ],
    },
    {
      label: 'Hands off to',
      items: [
        'steady-state workload reconciliation once the cluster is initialized',
        'day 2 upgrade, maintenance, and backup workflows',
        'operator-facing configuration and first-cluster guides',
      ],
    },
  ]}
/>

## Architectural Placement

Day 1 creation crosses three workload-side services in sequence:

1. The cert manager ensures TLS material exists or is ready to be observed.
2. The infrastructure manager renders the workload contract and keeps the StatefulSet at one replica.
3. The init manager initializes the cluster, configures autopilot defaults, and only then permits scale-out.

That split keeps first-boot safety logic separate from routine steady-state reconciliation.

<DiagramFrame
  title="Day 1 creation flow"
  caption="First boot is a controlled handoff: TLS and rendered config first, one-node bootstrap second, initialization and autopilot third, then scale-out into the requested cluster size."
  code={`sequenceDiagram
    autonumber
    participant User as User
    participant K8s as Kubernetes API
    participant Certs as Cert manager
    participant Infra as Infrastructure manager
    participant Init as Init manager
    participant Pod0 as Pod-0

    User->>K8s: Create OpenBaoCluster
    K8s-->>Certs: Reconcile TLS state
    Certs->>K8s: Create or validate TLS material
    K8s-->>Infra: Reconcile workload state
    Infra->>K8s: Render StatefulSet at replicas=1
    Infra->>Pod0: Start first pod
    Pod0-->>Init: Pod ready and API reachable
    Init->>Pod0: Detect initialized or perform init
    Init->>K8s: Set initialized status
    Init->>Pod0: Configure autopilot defaults
    K8s-->>Infra: Initialization confirmed
    Infra->>K8s: Scale StatefulSet to spec.replicas
  `}
/>

<DecisionTable
  kind="decision"
  title="Initialization paths"
  columns={['Path', 'Best fit', 'What changes in the control plane']}
  rows={[
    {
      cells: ['Self-init', 'Recommended when OpenBao supports native self-initialization and you want no root-token Secret in the cluster.', 'The workload renders self-init requests for pod-0, initialization is detected rather than performed by the operator, and status.selfInitialized becomes true.'],
      emphasis: 'recommended',
    },
    {
      cells: ['Standard initialization', 'Use when self-init is not enabled or you need the operator to drive `/sys/init` directly.', 'The init manager performs the init call, stores the root token in a Secret, then marks status.initialized before scale-out.'],
    },
  ]}
/>

<Tabs groupId="day1-init-paths">
  <TabItem value="self-init" label="Self-init">

<Checklist
  title="Self-init flow"
  items={[
    'Cert and infra managers prepare TLS, seal, and rendered config as usual, but pod-0 receives self-init requests in the rendered startup configuration.',
    'OpenBao initializes itself on first start and auto-revokes the transient root token instead of returning it to the operator.',
    'The init manager detects successful initialization through service-registration labels or equivalent readiness signals and sets status.selfInitialized.',
    'Once initialized, the infrastructure manager scales to the requested replica count and later pods auto-unseal and join the Raft cluster.',
  ]}
/>

<Callout type="note" title="Self-init changes root-token handling">

Self-init requires an auto-unseal mechanism. In exchange, it avoids creating a root-token Secret and keeps bootstrap closer to OpenBao’s native startup path.

</Callout>

  </TabItem>
  <TabItem value="standard" label="Standard init">

<Checklist
  title="Standard init flow"
  items={[
    'The infrastructure manager still forces single-pod bootstrap and renders TLS, storage, retry_join, and seal configuration first.',
    'The init manager waits for pod-0 readiness, detects whether the cluster is already initialized, and calls `/v1/sys/init` only when needed.',
    'The init response is handled in-memory, the root token is stored in `<cluster>-root-token`, and initialization status is patched back to the cluster.',
    'After initialization and autopilot configuration complete, the infrastructure manager scales the StatefulSet to the requested replica count.',
  ]}
/>

<Callout type="warning" title="Static auto-unseal version requirement">

Static auto-unseal requires OpenBao v2.4.0 or later. Older versions need a supported external KMS seal instead of the built-in static seal path.

</Callout>

  </TabItem>
</Tabs>

<DecisionTable
  kind="reference"
  title="Safety boundaries"
  columns={['Concern', 'Creation behavior']}
  rows={[
    {
      cells: ['Split-brain at first boot', 'The cluster always starts with one replica so only one node can become the first Raft leader.'],
      emphasis: 'recommended',
    },
    {
      cells: ['Root material handling', 'Standard init stores the root token in a Secret without logging the init response; self-init avoids creating the Secret entirely.'],
    },
    {
      cells: ['Already initialized detection', 'The init manager skips the init call when service-registration labels or health prove the cluster is already initialized.'],
    },
    {
      cells: ['Autopilot baseline', 'The init manager applies valid autopilot defaults before steady-state operations and later upgrades begin.'],
    },
  ]}
/>

<NextActions
  title="Continue the lifecycle"
  items={[
    {
      label: 'Day 2 operations',
      description: 'Move from first boot into upgrades, maintenance, and the long-running operation model.',
      docId: 'architecture/lifecycle/day2-operations',
    },
    {
      label: 'Init manager',
      description: 'Open the deep dive for the exact initialization, root-token, and autopilot contract.',
      docId: 'architecture/init-manager',
    },
    {
      label: 'Create your first cluster',
      description: 'Compare the internal creation flow with the operator-facing first-cluster guide.',
      docId: 'user-guide/openbaocluster/getting-started',
    },
  ]}
/>
