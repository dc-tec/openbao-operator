---
title: Workload Managers
hide_title: true
pageType: concept
journey: architecture
description: Bootstrap, networking, identity, and workload managers on the OpenBaoCluster workload reconcile path.
---

<PageHeader
  title="Workload managers"
  lede="The OpenBaoCluster workload path is coordinated through bootstrap, networking, identity, workload, cert, and init managers. Each manager owns a narrower write surface on the same reconcile path."
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
4. Each manager owns a narrower write surface on the same reconcile path.

This keeps change-coupling lower: config rendering, service exposure, ServiceAccount or RBAC wiring, and StatefulSet lifecycle do not need to move together by default.

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
      cells: ['Bootstrap manager', 'Rendered config, self-init surfaces, unseal prerequisites, and related validation.', 'ConfigMap surfaces, static unseal Secret when applicable, self-init ConfigMap, and shared-cache PVC setup.', 'Config and bootstrap prerequisites change for different reasons than networking or StatefulSet lifecycle.'],
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
  ]}
/>

## What each manager does

### Bootstrap manager

The bootstrap manager prepares everything the workload needs before the StatefulSet can be reconciled safely:

- render `config.hcl`
- reconcile the self-init ConfigMap when self-init is enabled
- generate the static unseal Secret when that seal mode is selected
- validate unseal prerequisites and related secret references
- prepare ACME shared-cache storage when that mode requires it

### Networking manager

The networking manager owns how traffic and policy reach the cluster:

- headless and external Services
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
- PodDisruptionBudget reconciliation
- rollout triggers from rendered config or certificate hash changes
- single-replica bootstrap and later scale-out after initialization
- revision-scoped workload resources used by blue/green and rollout-safe updates

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
  ]}
/>
