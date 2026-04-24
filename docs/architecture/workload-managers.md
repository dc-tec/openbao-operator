---
title: Workload Managers
hide_title: true
pageType: concept
journey: architecture
description: Bootstrap, networking, identity, and workload managers plus shared contracts on the OpenBaoCluster workload reconcile path.
---

<PageHeader
  title="Workload managers"
  lede="The OpenBaoCluster workload path is coordinated through bootstrap, networking, identity, workload, cert, and init managers. Shared configuration, resource identity, and owned-apply contracts keep the cross-service behavior aligned."
/>

<ManagerAtAGlance
  sections={[
    {
      label: 'Control path',
      items: [
        'workload reconciler',
        'internal/controller/openbaocluster',
        'internal/app/openbaocluster workload orchestration',
      ],
    },
    {
      label: 'Also reached by',
      items: [
        'same-cluster OpenBaoClusterClaim materialization',
        'the claim controller after catalog binding and local projection succeed',
      ],
    },
    {
      label: 'Workload-side managers',
      items: [
        'internal/service/bootstrap',
        'internal/service/networking',
        'internal/service/identity',
        'internal/service/workload',
      ],
    },
    {
      label: 'Coordinates with',
      items: [
        'internal/service/certs',
        'internal/service/init',
      ],
    },
    {
      label: 'Shared contracts',
      items: [
        'internal/service/configuration',
        'internal/platform/resourceidentity',
        'internal/platform/resourceapply',
      ],
    },
    {
      label: 'Writes',
      items: [
        'rendered config and bootstrap prerequisites',
        'Services, Ingress or Gateway resources, and network policies',
        'ServiceAccount and tenant-scoped RBAC for the main workload',
        'StatefulSet, PodDisruptionBudget, and rollout-triggering pod template changes',
      ],
    },
  ]}
/>

## Architectural placement

The workload reconcile path is now sequenced explicitly:

1. `internal/controller/openbaocluster` receives a workload-side reconcile event.
2. The controller delegates into `internal/app/openbaocluster`.
3. The app layer coordinates cert, bootstrap, networking, identity, workload, and init behavior in the correct order.
4. Shared contracts keep `config.hcl`, managed-resource identity, and generic owned-apply behavior consistent across those managers.
5. Each manager owns a narrower write surface on the same reconcile path.

This keeps change-coupling lower: config rendering, service exposure, ServiceAccount or RBAC wiring, and StatefulSet lifecycle do not need to move together by default.

The same-cluster claim path does not introduce a second workload manager stack. It resolves tenant intent first, then hands off into the same `OpenBaoCluster` runtime seam described here.

<DiagramFrame
  title="Same-cluster claim handoff"
  caption="Claims stop at the local materialization boundary. After that point, the same workload managers own the runtime just as they do for a directly created OpenBaoCluster."
  code={`graph LR
    Claim["OpenBaoClusterClaim"] --> ClaimApp["Claim orchestration"]
    ClaimApp --> Local["Local OpenBaoCluster"]
    Local --> Event["Workload-side reconcile event"]
    Event --> App["internal/app/openbaocluster"]

    classDef process fill:transparent,stroke:#fdd0a4,stroke-width:2px,color:#f8fafc;
    classDef write fill:transparent,stroke:#87d6be,stroke-width:2px,color:#e6f4ef;
    classDef read fill:transparent,stroke:#79c0ab,stroke-width:2px,color:#e6f4ef;

    class Claim,ClaimApp process;
    class Local,App write;
    class Event read;`}
/>

<DiagramFrame
  title="Workload managers on the reconcile path"
  caption="The app layer sequences narrow managers so each one owns a specific part of the workload-side contract."
  code={`graph TD
    Event["Workload-side reconcile event"] --> App["internal/app/openbaocluster"]

    App --> Cert["Cert manager"]
    App --> Bootstrap["Bootstrap manager"]
    App --> Networking["Networking manager"]
    App --> Identity["Identity manager"]
    App --> Workload["Workload manager"]
    App --> Init["Init manager when uninitialized"]
    Bootstrap --> ConfigSvc["Configuration service"]
    Bootstrap --> IdentityContract["Resource identity"]
    Networking --> IdentityContract
    Identity --> IdentityContract
    Workload --> IdentityContract
    Bootstrap --> ApplyContract["Owned apply"]
    Networking --> ApplyContract
    Identity --> ApplyContract
    Workload --> ApplyContract

    Cert --> TLS["TLS Secrets and reload signals"]
    Bootstrap --> Config["config.hcl, self-init config, seal prerequisites"]
    Networking --> Network["Services, Ingress or Gateway, NetworkPolicy"]
    Identity --> RBAC["ServiceAccount, Role, RoleBinding"]
    Workload --> Stateful["StatefulSet, PDB, rollout triggers"]
    Init --> InitState["status.initialized, autopilot handoff"]

    classDef process fill:transparent,stroke:#fdd0a4,stroke-width:2px,color:#f8fafc;
    classDef write fill:transparent,stroke:#87d6be,stroke-width:2px,color:#e6f4ef;
    classDef read fill:transparent,stroke:#79c0ab,stroke-width:2px,color:#e6f4ef;

    class App process;
    class Event,TLS,Config,Network,RBAC,Stateful,InitState read;
    class Cert,Bootstrap,Networking,Identity,Workload,Init write;`}
/>

<DecisionTable
  kind="reference"
  title="Manager boundaries within the workload path"
  columns={['Manager', 'Owns', 'Primary writes', 'Why it stays separate']}
  rows={[
    {
      cells: ['Bootstrap manager', 'Rendered config, self-init surfaces, unseal prerequisites, and related validation.', 'ConfigMap surfaces, static unseal Secret when applicable, self-init ConfigMap, shared-cache PVC setup, and managed audit file storage PVC setup.', 'Config and bootstrap prerequisites change for different reasons than networking or StatefulSet lifecycle.'],
      emphasis: 'recommended',
    },
    {
      cells: ['Networking manager', 'Cluster reachability and policy surfaces.', 'Headless and external Services, Ingress or Gateway resources, gateway CA ConfigMap, and network policies.', 'Service exposure and network policy assumptions are a distinct domain from config rendering or pod-template mutation.'],
    },
    {
      cells: ['Identity manager', 'Main workload Kubernetes identity.', 'ServiceAccount, Role, and RoleBinding for the cluster workload.', 'Workload RBAC should change independently from config, networking, or rollout behavior.'],
    },
    {
      cells: ['Workload manager', 'StatefulSet lifecycle and rollout boundaries.', 'StatefulSet, PodDisruptionBudget, revision-scoped rendered config, and pod-template rollout triggers.', 'Replica intent, pod-template mutation, and rollout safety belong close to the StatefulSet contract.'],
    },
    {
      cells: ['Claim handoff boundary', 'Entry point from same-cluster claims into the direct runtime seam.', 'A local OpenBaoCluster created by claim materialization, then the same workload-path managers as a direct cluster.', 'Claim policy and catalog resolution should stay above the runtime path instead of leaking into the workload managers.'],
    },
  ]}
/>

## What each manager does

### Bootstrap manager

The bootstrap manager prepares everything the workload needs before the StatefulSet can be reconciled safely:

- reconcile the main ConfigMap using `config.hcl` rendered by the shared configuration service
- reconcile the self-init ConfigMap when self-init is enabled
- generate the static unseal Secret when that seal mode is selected
- validate unseal prerequisites and related secret references
- prepare ACME shared-cache storage when that mode requires it
- prepare managed audit file storage PVCs when file audit handoff storage is configured

### Networking manager

The networking manager owns how traffic and policy reach the cluster:

- headless and external Services, including the shared client Service and optional dedicated read Service
- Ingress and Gateway API resources
- gateway CA export and backend TLS policy resources
- workload and job network policies
- API server network discovery and related preflight checks

### Identity manager

The identity manager owns the steady-state Kubernetes identity for the cluster workload:

- ServiceAccount creation or reuse
- Role and RoleBinding for pod-level runtime actions
- resource naming and ownership metadata for those identity objects

### Workload manager

The workload manager owns the StatefulSet-facing contract:

- StatefulSet render and apply
- steady-state read-replica StatefulSet lifecycle and safe drain or delete behavior
- PodDisruptionBudget reconciliation
- rollout triggers from rendered config or certificate hash changes
- audit file storage volume and mount wiring for voter and read-replica StatefulSets
- single-replica bootstrap and later scale-out after initialization
- revision-scoped workload resources used by blue/green and rollout-safe updates
- read-first or restore-safe operational ordering delegated by the app layer for rolling upgrades, blue-green, and restore

<Callout type="note" title="The shared client endpoint is intentional">

The networking manager keeps the main client Service attached to the shared client-serving selector, which can include both voters and steady read replicas. The optional dedicated read Service remains a separate networking surface for explicit consumers, but the primary endpoint stays stable so OpenBao can handle standby reads and request forwarding itself.

</Callout>

<Callout type="note" title="Claims reuse the same workload runtime">

Same-cluster claims do not get a special runtime-only manager path. Once the claim materializes a local `OpenBaoCluster`, workload behavior, readiness, bootstrap, networking, identity, and StatefulSet rollout all follow the same runtime contracts described on this page.

</Callout>

## Shared contracts on the workload path

The workload-side managers share three narrower contracts instead of reimplementing the same semantics in each package:

<DecisionTable
  kind="reference"
  title="Shared contracts below the managers"
  columns={['Contract', 'Used by', 'Owns']}
  rows={[
    {
      cells: ['Configuration service', 'Bootstrap manager and blue-green upgrade flow.', 'Shared `config.hcl` rendering semantics, including revision-aware green startup config.'],
      emphasis: 'recommended',
    },
    {
      cells: ['Resource identity', 'Bootstrap, networking, identity, and workload managers.', 'Managed-resource names, labels, and selectors that must stay aligned across services.'],
    },
    {
      cells: ['Owned apply', 'Bootstrap, networking, identity, and workload managers.', 'Generic owner-ref-aware apply flow and shared SSA error handling for resources that do not need object-specific special cases.'],
    },
  ]}
/>

<Callout type="note" title="Cert and init stay adjacent to the workload path">

TLS lifecycle and first-boot initialization stay as separate managers on the same workload-side reconcile path. They continue to coordinate with the workload managers without becoming part of the bootstrap, networking, identity, or workload services.

</Callout>

<NextActions
  title="Related deep dives"
  items={[
    {
      label: 'Component design',
      description: 'See where the workload-side managers sit relative to controllers, admin operations, and status writes.',
      docId: 'architecture/components',
    },
    {
      label: 'Init manager',
      description: 'Follow the first-boot contract and the handoff into scale-out after initialization succeeds.',
      docId: 'architecture/init-manager',
    },
    {
      label: 'Cert manager',
      description: 'See how TLS lifecycle fits into the same workload-side path without becoming part of the bootstrap manager.',
      docId: 'architecture/cert-manager',
    },
    {
      label: 'Day 1 creation',
      description: 'Follow how these managers interact during cluster bootstrap and the first safe scale-out.',
      docId: 'architecture/lifecycle/day1-creation',
    },
    {
      label: 'Service claims',
      description: 'See the claim path that feeds into this same-cluster runtime instead of creating a second workload engine.',
      docId: 'architecture/service-claims',
    },
  ]}
/>
