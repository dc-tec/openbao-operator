---
title: Day 0 Provisioning
hide_title: true
pageType: concept
journey: architecture
description: Tenant provisioning flow from OpenBaoTenant creation through namespace-scoped RBAC, policy defaults, and the handoff to cluster creation.
---

<PageHeader
  title="Provision the tenant boundary before any cluster exists."
  lede="Day 0 is the namespace and tenancy setup phase. The provisioner controller takes an `OpenBaoTenant` request, applies tenant-scoped RBAC and policy defaults, and leaves behind a namespace that is safe for later `OpenBaoCluster` creation."
/>

<JourneyRail
  current={1}
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
        '`OpenBaoTenant` creation and target namespace selection',
        'operator ServiceAccount identity and tenant policy defaults',
        'admission dependencies that must exist before Secret access is widened',
      ],
    },
    {
      label: 'Primary owners',
      items: [
        'internal/controller/provisioner',
        'internal/app/provisioner',
        'internal/service/provisioner',
      ],
    },
    {
      label: 'Writes',
      items: [
        'tenant Role and RoleBinding resources',
        'Secret reader and writer allowlist roles',
        'Pod Security labels plus optional ResourceQuota and LimitRange defaults',
      ],
    },
    {
      label: 'Hands off to',
      items: [
        'a namespace that is ready for `OpenBaoCluster` creation',
        'tenant operators who can now move into Day 1 cluster creation',
        'later Secret allowlist sync as clusters appear or disappear',
      ],
    },
  ]}
/>

## Architectural Placement

Day 0 provisioning uses the dedicated tenant-controller path:

1. A cluster admin or namespace owner creates `OpenBaoTenant`.
2. `internal/controller/provisioner` receives the reconcile event and delegates into `internal/app/provisioner`.
3. `internal/service/provisioner` applies namespace-scoped RBAC, policy labels, allowlists, and quota defaults.

This keeps tenant onboarding separate from steady-state cluster reconciliation and makes the tenant boundary explicit before any workload resources exist.

<DiagramFrame
  title="Day 0 provisioning flow"
  caption="Provisioning establishes the namespace boundary first, then keeps Secret access aligned with the managed clusters that later appear in that namespace."
  code={`sequenceDiagram
    autonumber
    participant Admin as Admin or namespace owner
    participant K8s as Kubernetes API
    participant Ctrl as Provisioner controller
    participant App as App orchestration
    participant Manager as Provisioner manager
    participant Namespace as Tenant namespace

    Admin->>K8s: Create OpenBaoTenant
    K8s-->>Ctrl: Watch OpenBaoTenant
    Ctrl->>App: Reconcile tenant request
    App->>Manager: Apply tenant provisioning contract
    Manager->>Namespace: Create tenant Role / RoleBinding
    Manager->>Namespace: Apply Pod Security labels
    Manager->>Namespace: Apply optional quota defaults
    Manager->>Namespace: Sync Secret allowlist roles
    Namespace-->>Admin: Namespace ready for OpenBaoCluster
  `}
/>

<DecisionTable
  kind="reference"
  title="Day 0 responsibilities"
  columns={['Stage', 'Primary owner', 'Why it matters']}
  rows={[
    {
      cells: ['Namespace targeting', 'Provisioner controller and app layer.', 'Tenant provisioning must decide which namespace is allowed before any RBAC or policy defaults are applied.'],
      emphasis: 'recommended',
    },
    {
      cells: ['Operator tenant RBAC', 'Provisioner manager.', 'The operator needs enough namespace-scoped access to manage OpenBao resources there without defaulting to broad cluster-wide permissions.'],
    },
    {
      cells: ['Secret allowlists', 'Provisioner manager plus tenant Secret RBAC sync.', 'Multi-tenant safety depends on explicit Secret access derived from actual managed cluster references.'],
    },
    {
      cells: ['Policy defaults', 'Provisioner manager.', 'Pod Security labels and optional quotas give the namespace a safe baseline before a cluster is created.'],
    },
  ]}
/>

<DecisionTable
  kind="reference"
  title="Safety boundaries"
  columns={['Concern', 'Provisioning behavior']}
  rows={[
    {
      cells: ['Self-service namespace scope', 'Self-service tenants may target only their own namespace; cross-namespace provisioning is reserved for trusted centrally managed cases.'],
      emphasis: 'recommended',
    },
    {
      cells: ['Admission readiness', 'Tenant Secret allowlists wait for admission-policy dependencies so Secret access is not widened before enforcement is available.'],
    },
    {
      cells: ['Shared RBAC lifecycle', 'Tenant RBAC avoids OwnerReferences that would let a single cluster deletion garbage-collect shared namespace permissions.'],
    },
    {
      cells: ['Cleanup timing', 'Provisioned RBAC remains in place until managed clusters are gone, then the tenant finalizer can be released safely.'],
    },
  ]}
/>

<NextActions
  title="Continue the lifecycle"
  items={[
    {
      label: 'Day 1 creation',
      description: 'Follow how a provisioned namespace hands off into first-cluster bootstrap and initialization.',
      docId: 'architecture/lifecycle/day1-creation',
    },
    {
      label: 'Provisioner manager',
      description: 'Open the deep dive for the exact namespace-scoped RBAC, allowlist, and cleanup contract.',
      docId: 'architecture/provisioner-manager',
    },
    {
      label: 'Tenant onboarding guide',
      description: 'Compare the internal provisioning flow with the operator-facing onboarding steps.',
      docId: 'user-guide/openbaotenant/onboarding',
    },
  ]}
/>
