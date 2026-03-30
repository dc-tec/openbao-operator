---
title: Component Design
hide_title: true
pageType: concept
journey: architecture
description: Split-controller architecture for OpenBaoCluster, OpenBaoRestore, and OpenBaoTenant, including controller boundaries, manager orchestration, and service-layer coordination.
---

<PageHeader
  title="Split the control plane so pod churn, long-running operations, and status writes stay separate."
  lede="OpenBao Operator avoids a single reconciliation loop. The control plane is divided into focused controllers, then delegated into app-layer orchestration and narrower domain managers so the system can react quickly without mixing unrelated responsibilities."
/>



## Controller Split

<DiagramFrame
  title="Controller split"
  caption="Workload, admin operations, and status are separated so high-churn reconciliation, long-running workflows, and API status writes do not block each other."
  code={`graph TD
    Manager["Manager process"] --> Workload["Workload controller"]
    Manager --> Admin["AdminOps controller"]
    Manager --> Status["Status controller"]

    subgraph Roles["Responsibilities"]
      Workload --> Infra["Infra Manager"]
      Workload --> Cert["Cert Manager"]
      Workload --> Init["Init Manager"]

      Admin --> Upgrade["Upgrade Manager"]
      Admin --> Backup["Backup Manager"]

      Status --> Conditions["Status conditions"]
    end

    classDef process fill:transparent,stroke:#fdd0a4,stroke-width:2px,color:#f8fafc;
    classDef write fill:transparent,stroke:#87d6be,stroke-width:2px,color:#e6f4ef;
    classDef read fill:transparent,stroke:#79c0ab,stroke-width:2px,color:#e6f4ef;

    class Manager process;
    class Workload,Admin,Status write;
    class Infra,Cert,Init,Upgrade,Backup,Conditions read;`}
/>

<DecisionTable
  kind="reference"
  title="Controller responsibilities"
  columns={['Controller', 'Primary role', 'Why it stays separate']}
  rows={[
    {
      cells: ['Workload', 'Reconciles StatefulSets, Services, ConfigMaps, and Secrets.', 'It handles high-churn pod and platform state and needs to react quickly.'],
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

## App Orchestration And Managers

<DiagramFrame
  title="App-layer orchestration"
  caption="Controllers hand off to narrow app-layer facades first, then into focused managers and shared lifecycle services. This keeps import surfaces small and responsibilities explicit."
  code={`graph TD
    OBC["OpenBaoCluster controllers"] --> OBCApp["internal/app/openbaocluster"]
    OBR["OpenBaoRestore controller"] --> OBRApp["internal/app/openbaorestore"]
    Prov["Provisioner controller"] --> ProvApp["internal/app/provisioner"]

    OBCApp --> Workload["Workload orchestration"]
    OBCApp --> AdminOps["AdminOps orchestration"]
    OBCApp --> StatusOps["Status and deletion orchestration"]

    Workload --> Infra["Infra Manager"]
    Workload --> Cert["Cert Manager"]
    Workload --> Init["Init Manager"]
    AdminOps --> Upgrade["Upgrade Manager"]
    AdminOps --> Backup["Backup Manager"]

    OBRApp --> Restore["Restore Manager"]
    ProvApp --> Provisioner["Provisioner Manager"]

    classDef process fill:transparent,stroke:#fdd0a4,stroke-width:2px,color:#f8fafc;
    classDef write fill:transparent,stroke:#87d6be,stroke-width:2px,color:#e6f4ef;
    classDef read fill:transparent,stroke:#79c0ab,stroke-width:2px,color:#e6f4ef;

    class OBC,OBR,Prov write;
    class OBCApp,OBRApp,ProvApp,Workload,AdminOps,StatusOps process;
    class Infra,Cert,Init,Upgrade,Backup,Restore,Provisioner read;`}
/>

<DecisionTable
  kind="reference"
  title="Manager boundaries"
  columns={['Manager', 'Scope', 'Key reason for separation']}
  rows={[
    {
      cells: ['Infrastructure Manager', 'Renders config and manages StatefulSet-facing infrastructure.', 'Workload state and rendered configuration change frequently and should stay close to the pod lifecycle.'],
      emphasis: 'recommended',
    },
    {
      cells: ['Cert Manager', 'Handles operator-managed, ACME, and external TLS interactions.', 'TLS integration has its own dependency model and readiness surface.'],
    },
    {
      cells: ['Init Manager', 'Coordinates initialization when self-init is disabled.', 'Bootstrap logic is security-sensitive and distinct from normal steady-state reconcile work.'],
    },
    {
      cells: ['Upgrade / Backup / Restore Managers', 'Run lock-aware disruptive operations.', 'These workflows share lifecycle helpers but own different risk profiles and side effects.'],
    },
    {
      cells: ['Provisioner Manager', 'Onboards tenant namespaces and guardrails.', 'Tenant governance belongs to provisioning time, not to the cluster workload loop.'],
    },
  ]}
/>

<Callout type="note" title="Boundary contract">

Controller import surfaces are intentionally narrow and enforced by generated architecture-boundary rules from `.ast-grep/policy/architecture-boundaries.yml`.

</Callout>

<NextActions
  title="Deep dives"
  items={[
    {
      label: 'Infrastructure manager',
      description: 'See how configuration rendering and StatefulSet ownership are coordinated.',
      docId: 'architecture/infra-manager',
    },
    {
      label: 'Upgrade manager',
      description: 'Review how RollingUpdate and BlueGreen state transitions are modeled.',
      docId: 'architecture/upgrade-manager',
    },
    {
      label: 'Restore manager',
      description: 'Understand the destructive restore path and lock lifecycle behind OpenBaoRestore.',
      docId: 'architecture/restore-manager',
    },
    {
      label: 'Lifecycle architecture',
      description: 'Move from component boundaries into the day-by-day lifecycle flows that use them.',
      docId: 'architecture/lifecycle/index',
    },
  ]}
/>
