---
title: Component Design
hide_title: true
pageType: concept
journey: architecture
description: Split-controller architecture for OpenBaoCluster, OpenBaoRestore, and OpenBaoTenant, including controller boundaries, app-layer orchestration, and service-layer coordination.
---

<PageHeader
  title="Split-controller control plane"
  lede="Focused controllers, app-layer orchestration, narrow domain managers, and shared platform contracts keep workload churn, long-running operations, and status writes separated."
/>

## Controller split

<DiagramFrame
  title="Controller split"
  caption="Workload, admin operations, and status are separated so high-churn reconciliation, long-running workflows, and API status writes do not block each other."
  code={`graph TD
    Manager["Manager process"] --> WorkloadCtrl["Workload controller"]
    Manager --> Admin["AdminOps controller"]
    Manager --> Status["Status controller"]

    subgraph Roles["Responsibilities"]
      WorkloadCtrl --> Cert["Cert manager"]
      WorkloadCtrl --> Bootstrap["Bootstrap manager"]
      WorkloadCtrl --> Networking["Networking manager"]
      WorkloadCtrl --> Identity["Identity manager"]
      WorkloadCtrl --> Init["Init manager"]
      WorkloadCtrl --> WorkloadMgr["Workload manager"]

      Admin --> Upgrade["Upgrade manager"]
      Admin --> Backup["Backup manager"]

      Status --> Conditions["Status conditions"]
    end

    classDef process fill:transparent,stroke:#fdd0a4,stroke-width:2px,color:#f8fafc;
    classDef write fill:transparent,stroke:#87d6be,stroke-width:2px,color:#e6f4ef;
    classDef read fill:transparent,stroke:#79c0ab,stroke-width:2px,color:#e6f4ef;

    class Manager process;
    class WorkloadCtrl,Admin,Status write;
    class Cert,Bootstrap,Networking,Identity,Init,WorkloadMgr,Upgrade,Backup,Conditions read;`}
/>

<DecisionTable
  kind="reference"
  title="Controller responsibilities"
  columns={['Controller', 'Primary role', 'Why it stays separate']}
  rows={[
    {
      cells: ['Workload', 'Reconciles workload-side certificate, bootstrap, networking, identity, initialization, and StatefulSet resources.', 'It handles high-churn pod and platform state and needs to react quickly.'],
      emphasis: 'recommended',
    },
    {
      cells: ['AdminOps', 'Runs upgrades and backups.', 'Long-running workflows should not block pod recovery or normal reconciliation.'],
    },
    {
      cells: ['Status', 'Aggregates state and writes status updates.', 'Serializing status writes avoids ResourceVersion conflicts and keeps conditions stable.'],
    },
    {
      cells: ['OpenBaoRestore', 'Reconciles destructive restore workflows.', 'Restore needs its own lock-aware control surface instead of riding on normal cluster reconcile loops.'],
    },
    {
      cells: ['Provisioner', 'Reconciles OpenBaoTenant onboarding and namespace scaffolding.', 'Tenant guardrails belong to Day 0 provisioning, not to workload reconciliation.'],
    },
  ]}
/>

<Callout type="note" title="Restore controller">

Restores are reconciled through the separate `OpenBaoRestore` controller, which orchestrates restore Jobs and acquires the cluster operation lock before destructive work starts.

</Callout>

## Watch and requeue strategy

The controller registration strategy follows the tenancy RBAC boundary. Single-tenant mode can watch owned child
resources inside the operator namespace. Multi-tenant mode does not register those child watches because
controller-runtime would require list and watch access across tenant namespaces.

<DecisionTable
  kind="reference"
  title="Reconcile triggers by tenancy mode"
  columns={['Mode', 'Primary triggers', 'Tradeoff']}
  rows={[
    {
      cells: [
        'Single-tenant',
        '`OpenBaoCluster` changes plus owned StatefulSet, Service, ConfigMap, Secret, Job, and ServiceAccount events; AdminOps also watches owned Jobs.',
        'Child-resource changes enqueue reconciliation quickly because namespace-scoped list and watch permissions stay inside one trust boundary.',
      ],
      emphasis: 'recommended',
    },
    {
      cells: [
        'Multi-tenant',
        'Qualifying `OpenBaoCluster` changes plus explicit progress and retry requeues; the status controller also performs a steady-state refresh.',
        'Status freshness stays bounded, but arbitrary steady-state child mutation does not enqueue the workload controller. Repair can wait for its next parent event or another explicit workload requeue. This preserves namespace-scoped tenant RBAC and avoids cluster-wide child-resource discovery.',
      ],
    },
  ]}
/>

Both modes use rate-limited retries. The status controller has a steady-state refresh and safety requeues, while
workload and AdminOps requeues follow active reconcile progress. The difference is event immediacy, not ownership:
the same managers still read and reconcile the same named resources when a cluster is processed.

<Callout type="warning" title="Multi-tenant child watches change the trust model">

Adding `Owns` registration for tenant child resources in multi-tenant mode requires a deliberate RBAC and
architecture change. Update the tenancy model, permissions, tests, and this page together.

</Callout>

## App orchestration and managers

<DiagramFrame
  title="App-layer orchestration"
  caption="Controllers hand off to narrow app-layer facades first, then into focused managers and shared lifecycle services. This keeps import surfaces small and responsibilities explicit."
  code={`graph TD
    OBC["OpenBaoCluster controllers"] --> OBCApp["internal/app/openbaocluster"]
    OBR["OpenBaoRestore controller"] --> OBRApp["internal/app/openbaorestore"]
    Prov["Provisioner controller"] --> ProvApp["internal/app/provisioner"]

    OBCApp --> WorkloadOps["Workload orchestration"]
    OBCApp --> AdminOps["AdminOps orchestration"]
    OBCApp --> StatusOps["Status and deletion orchestration"]

    WorkloadOps --> Cert["Cert manager"]
    WorkloadOps --> Bootstrap["Bootstrap manager"]
    WorkloadOps --> Networking["Networking manager"]
    WorkloadOps --> Identity["Identity manager"]
    WorkloadOps --> Init["Init manager"]
    WorkloadOps --> WorkloadMgr["Workload manager"]
    AdminOps --> Upgrade["Upgrade manager"]
    AdminOps --> Backup["Backup manager"]

    OBRApp --> Restore["Restore manager"]
    ProvApp --> Provisioner["Provisioner manager"]

    classDef process fill:transparent,stroke:#fdd0a4,stroke-width:2px,color:#f8fafc;
    classDef write fill:transparent,stroke:#87d6be,stroke-width:2px,color:#e6f4ef;
    classDef read fill:transparent,stroke:#79c0ab,stroke-width:2px,color:#e6f4ef;

    class OBC,OBR,Prov write;
    class OBCApp,OBRApp,ProvApp,WorkloadOps,AdminOps,StatusOps process;
    class Cert,Bootstrap,Networking,Identity,Init,WorkloadMgr,Upgrade,Backup,Restore,Provisioner read;`}
/>

<DecisionTable
  kind="reference"
  title="Manager boundaries"
  columns={['Manager', 'Scope', 'Key reason for separation']}
  rows={[
    {
      cells: ['Bootstrap manager', 'Renders config and prepares bootstrap prerequisites.', 'Config rendering and seal or self-init prerequisites change for different reasons than networking or StatefulSet mutation.'],
      emphasis: 'recommended',
    },
    {
      cells: ['Networking manager', 'Owns Services, Gateway or Ingress resources, and workload network policy.', 'Reachability and network contract changes should not be coupled to config rendering or RBAC wiring.'],
    },
    {
      cells: ['Identity manager', 'Owns the workload ServiceAccount and tenant-scoped RBAC.', 'Workload identity and RBAC should evolve independently from networking and StatefulSet behavior.'],
    },
    {
      cells: ['Workload manager', 'Owns StatefulSet, PodDisruptionBudget, and rollout-safe workload mutation.', 'Replica intent, pod-template mutation, and rollout triggers belong close to the StatefulSet contract.'],
    },
    {
      cells: ['Cert manager', 'Handles operator-managed, ACME, and external TLS interactions.', 'TLS integration has its own dependency model and readiness surface.'],
    },
    {
      cells: ['Init manager', 'Coordinates initialization when self-init is disabled or needs confirmation.', 'Bootstrap logic is security-sensitive and distinct from normal steady-state reconcile work.'],
    },
    {
      cells: ['Upgrade / Backup / Restore managers', 'Run lock-aware disruptive operations.', 'These workflows share lifecycle helpers but own different risk profiles and side effects.'],
    },
    {
      cells: ['Provisioner manager', 'Onboards tenant namespaces and guardrails.', 'Tenant governance belongs to provisioning time, not to the cluster workload loop.'],
    },
  ]}
/>

## Shared contracts below managers

The controller and app layers coordinate managers, but some semantics stay below the manager boundary because they must stay uniform across multiple services.

<DecisionTable
  kind="reference"
  title="Shared contracts"
  columns={['Contract', 'Used by', 'Why it stays separate']}
  rows={[
    {
      cells: ['Configuration service', 'Bootstrap manager and blue-green upgrade startup.', '`config.hcl` semantics should stay in one place even though both workload bootstrap and upgrade orchestration need them.'],
      emphasis: 'recommended',
    },
    {
      cells: ['Resource identity', 'Bootstrap, networking, identity, and workload managers.', 'Names, labels, and selectors define the managed-resource contract and should not drift across services.'],
    },
    {
      cells: ['Owned apply', 'Bootstrap, networking, identity, and workload managers.', 'Generic owner-ref-aware SSA apply behavior is a platform concern; object-specific exceptions still stay in the owning service.'],
    },
    {
      cells: ['Architecture boundary policy', 'Controllers, app packages, services, and selected platform packages.', 'Explicit service and adapter allowlists keep layered architecture rules enforced in CI instead of implied by convention alone.'],
    },
  ]}
/>

<Callout type="note" title="Boundary contract">

Controller, app, service, and selected platform import surfaces are intentionally narrow and enforced by generated architecture-boundary rules from `.ast-grep/policy/architecture-boundaries.yml`.

</Callout>

<NextActions
  title="Deep dives"
  items={[
    {
      label: 'Workload managers',
      description: 'Bootstrap, networking, identity, and StatefulSet ownership on the workload reconcile path.',
      docId: 'architecture/workload-managers',
    },
    {
      label: 'Upgrade manager',
      description: 'RollingUpdate and BlueGreen state transitions.',
      docId: 'architecture/upgrade-manager',
    },
    {
      label: 'Restore manager',
      description: 'Destructive restore path and lock lifecycle behind OpenBaoRestore.',
      docId: 'architecture/restore-manager',
    },
    {
      label: 'Lifecycle architecture',
      description: 'Day-by-day lifecycle flows that use these components.',
      docId: 'architecture/lifecycle/index',
    },
  ]}
/>
