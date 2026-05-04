---
title: Component Design
hide_title: true
pageType: concept
journey: architecture
description: Split-controller architecture for OpenBaoCluster, OpenBaoClusterClaim, OpenBaoRestore, and OpenBaoTenant, including controller boundaries, app-layer orchestration, and service-layer coordination.
---

<PageHeader
  title="Split-controller control plane"
  lede="Focused controllers, app-layer orchestration, narrow domain managers, and shared platform contracts keep workload churn, long-running operations, and status writes separated."
/>

## Cluster lifecycle controllers

<DiagramFrame
  title="Cluster lifecycle controllers"
  caption="Workload, admin operations, status, and destructive restore stay separated so high-churn reconciliation, long-running workflows, and status writes do not block each other."
  code={`graph TD
    Manager["Manager process"] --> WorkloadCtrl["Workload controller"]
    Manager --> Admin["AdminOps controller"]
    Manager --> Status["Status controller"]
    Manager --> RestoreCtrl["OpenBaoRestore controller"]

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
      RestoreCtrl --> Restore["Restore manager"]
    end

    classDef process fill:transparent,stroke:#fdd0a4,stroke-width:2px,color:#f8fafc;
    classDef write fill:transparent,stroke:#87d6be,stroke-width:2px,color:#e6f4ef;
    classDef read fill:transparent,stroke:#79c0ab,stroke-width:2px,color:#e6f4ef;

    class Manager process;
    class WorkloadCtrl,Admin,Status,RestoreCtrl write;
    class Cert,Bootstrap,Networking,Identity,Init,WorkloadMgr,Upgrade,Backup,Conditions,Restore read;`}
/>

## Tenant and service-request controllers

<DiagramFrame
  title="Tenant and claim controllers"
  caption="Provisioner and claim controllers stay separate from the direct workload path. Provisioner introduces namespace access and guardrails. Claim reconciliation binds catalog intent, plans materialization, and publishes the connection contract without becoming a second workload engine."
  code={`graph TD
    Manager["Manager process"] --> Prov["Provisioner controller"]
    Manager --> Claim["OpenBaoClusterClaim controller"]

    subgraph Responsibilities["Responsibilities"]
      Prov --> Tenant["OpenBaoTenant onboarding"]
      Prov --> Guardrails["Tenant RBAC, Secret allowlists, namespace guardrails"]

      Claim --> Catalog["Catalog resolution and continuity"]
      Claim --> Placement["Placement and materialization planning"]
      Claim --> Materialize["Same-cluster OpenBaoCluster materialization"]
      Claim --> Connect["Connection publication"]
    end

    classDef process fill:transparent,stroke:#fdd0a4,stroke-width:2px,color:#f8fafc;
    classDef write fill:transparent,stroke:#87d6be,stroke-width:2px,color:#e6f4ef;
    classDef read fill:transparent,stroke:#79c0ab,stroke-width:2px,color:#e6f4ef;

    class Manager process;
    class Prov,Claim write;
    class Tenant,Guardrails,Catalog,Placement,Materialize,Connect read;`}
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
      cells: ['OpenBaoClusterClaim', 'Resolves tenant-facing claims through the service catalog, materializes supported same-cluster workloads, and publishes the claim connection contract.', 'Claim binding, continuity, and connection publication are a different control surface from direct workload reconciliation.'],
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

## Cluster runtime orchestration

<DiagramFrame
  title="Cluster runtime orchestration"
  caption="The direct cluster runtime stays split between workload, admin-operations, restore, and provisioning paths. Each controller hands off to a narrow app facade before reaching its managers."
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

## Claim orchestration

<DiagramFrame
  title="Claim orchestration"
  caption="The claim path binds a tenant-facing request through the catalog, renders a bounded execution contract, then materializes the supported same-cluster runtime and publishes the connection contract."
  code={`graph TD
    ClaimCtrl["OpenBaoClusterClaim controller"] --> ClaimApp["internal/app/openbaoclusterclaim"]
    BackupReqCtrl["Backup request controller"] --> BackupReqApp["openbaoclusterclaim/backuprequest"]
    RestoreReqCtrl["Restore request controller"] --> RestoreReqApp["openbaoclusterclaim/restorerequest"]
    UpgradeReqCtrl["Upgrade request controller"] --> UpgradeReqApp["openbaoclusterclaim/upgraderequest"]

    ClaimApp --> Catalog["claimcontract catalog binding"]
    ClaimApp --> Approved["Approved service contract"]
    ClaimApp --> Rendered["Rendered execution contract"]
    ClaimApp --> Placement["placement and materialization state"]
    ClaimApp --> Local["Same-cluster OpenBaoCluster materialization"]
    ClaimApp --> Connection["connectionpublishing"]
    ClaimApp --> RequestSummary["request workflow summary"]

    BackupReqApp --> RequestSummary
    RestoreReqApp --> RequestSummary
    UpgradeReqApp --> RequestSummary

    Local --> Runtime["OpenBaoCluster runtime path"]

    classDef process fill:transparent,stroke:#fdd0a4,stroke-width:2px,color:#f8fafc;
    classDef write fill:transparent,stroke:#87d6be,stroke-width:2px,color:#e6f4ef;
    classDef read fill:transparent,stroke:#79c0ab,stroke-width:2px,color:#e6f4ef;

    class ClaimCtrl,ClaimApp,BackupReqCtrl,RestoreReqCtrl,UpgradeReqCtrl,BackupReqApp,RestoreReqApp,UpgradeReqApp process;
    class Catalog,Approved,Rendered,Placement,Connection read;
    class RequestSummary,Local,Runtime write;`}
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

<DecisionTable
  kind="reference"
  title="Claim-specific orchestration surfaces"
  columns={['Surface', 'Scope', 'Why it stays separate']}
  rows={[
    {
      cells: ['Claim contract pipeline', 'Catalog binding, continuity checks, approved-contract identity, rendered execution contract, and same-cluster projection planning.', 'Tenant-facing service policy should stay separate from direct workload managers so unsupported shapes can fail closed before the runtime seam is touched.'],
      emphasis: 'recommended',
    },
    {
      cells: ['Connection publication', 'Tenant-facing connection Secret and endpoint publication for internal, ingress, and gateway claim shapes.', 'Claim output custody and external endpoint timing are different concerns from direct workload networking reconciliation.'],
    },
    {
      cells: ['Placement and materialization state', 'Current same-cluster materialization plus future explicit remote placement state.', 'The claim path needs a clear service-request state machine without turning placement into hidden behavior inside workload managers.'],
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
      label: 'Service claims',
      description: 'Claim-to-contract-to-materialization flow and the current scope limits behind the bounded claim model.',
      docId: 'architecture/service-claims',
    },
    {
      label: 'Lifecycle architecture',
      description: 'Day-by-day lifecycle flows that use these components.',
      docId: 'architecture/lifecycle/index',
    },
  ]}
/>
