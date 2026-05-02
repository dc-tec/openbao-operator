---
title: Provisioner Manager
hide_title: true
pageType: concept
journey: architecture
description: Provision tenant namespaces with scoped RBAC, Secret allowlists, Pod Security labels, and quota defaults for OpenBaoTenant.
---

<PageHeader
  title="Provision tenant namespaces with scoped RBAC, Secret allowlists, and policy defaults."
  lede="The provisioner manager owns the tenant-onboarding contract for `OpenBaoTenant`. It creates namespace-scoped permissions for the operator, applies Pod Security and quota defaults, and keeps Secret access narrowed to explicit allowlists derived from managed clusters."
/>



<ManagerAtAGlance
  sections={[
    {
      label: 'Control path',
      items: [
        'dedicated OpenBaoTenant controller',
        'internal/app/provisioner',
        'internal/service/provisioner',
      ],
    },
    {
      label: 'Owns',
      items: [
        'tenant-scoped operator Role and RoleBinding resources',
        'reader and writer Secret allowlists derived from tenant clusters',
        'Pod Security labels plus optional ResourceQuota and LimitRange defaults',
      ],
    },
    {
      label: 'Writes',
      items: [
        'tenant Role / RoleBinding and Secret RBAC resources',
        'namespace labels that enforce Pod Security defaults',
        'ResourceQuota and LimitRange resources from OpenBaoTenant.spec',
      ],
    },
    {
      label: 'Depends on',
      items: [
        'OpenBaoTenant namespace targeting rules and resource policies',
        'admission dependencies before tenant Secret RBAC sync can proceed',
        'current OpenBaoCluster objects that still exist in the tenant namespace',
      ],
    },
  ]}
/>

## Architectural placement

Provisioning follows a dedicated tenant-controller path:

1. `internal/controller/provisioner` receives the reconcile event for `OpenBaoTenant`.
2. The controller delegates orchestration to `internal/app/provisioner`.
3. The app layer invokes `internal/service/provisioner` to apply RBAC, labels, allowlists, and cleanup behavior.

That keeps tenant onboarding separate from `OpenBaoCluster` steady-state reconciliation while still using the same design system and policy language.

The tenant `RoleBinding` is also the explicit handoff marker into the cluster controllers. Until that object exists, `OpenBaoCluster` workload, admin-operations, and status reconciliation pause and requeue instead of trying to mutate resources in a namespace that is not yet provisioned.

<DecisionTable
  kind="reference"
  title="Owned surfaces"
  columns={['Surface', 'What the manager decides', 'Why it matters']}
  rows={[
    {
      cells: ['Tenant RBAC', 'The operator ServiceAccount permissions granted in the tenant namespace.', 'The operator needs enough access to manage clusters there, but not broad namespace-level privileges by default.'],
      emphasis: 'recommended',
    },
    {
      cells: ['Secret allowlists', 'Which Secrets the operator may read or write for managed clusters in the tenant namespace.', 'Multi-tenant safety depends on explicit Secret access instead of wildcard RBAC.'],
    },
    {
      cells: ['Controller handoff', 'When the tenant namespace is actually ready for `OpenBaoCluster` reconciliation.', 'GitOps paths can submit tenant and cluster objects together only if the controller has a deterministic, namespace-scoped readiness marker.'],
    },
    {
      cells: ['Pod Security labels', 'The baseline namespace policy applied to tenant workloads and operator-managed pods.', 'Tenants need a secure default even before any cluster objects are created.'],
    },
    {
      cells: ['Quota defaults', 'Optional ResourceQuota and LimitRange resources derived from OpenBaoTenant.spec.', 'Tenants need resource guardrails that travel with the namespace onboarding contract.'],
    },
  ]}
/>

## Provisioning flow

<DiagramFrame
  title="Tenant onboarding flow"
  caption="Provisioning applies namespace-scoped guardrails first, then keeps Secret allowlists synchronized as managed clusters appear or disappear in the tenant namespace."
  code={`graph TD
    Tenant["OpenBaoTenant"] --> Ctrl["Provisioner controller"]
    Ctrl --> App["internal/app/provisioner"]
    App --> Manager["Provisioner manager"]
    Manager --> RBAC["Tenant Role / RoleBinding"]
    Manager --> Secrets["Reader / writer Secret allowlists"]
    Manager --> Labels["Pod Security labels"]
    Manager --> Quotas["ResourceQuota / LimitRange"]
    Quotas --> Ready["Tenant namespace ready for OpenBaoCluster"]

    classDef read fill:transparent,stroke:#79c0ab,stroke-width:2px,color:#e6f4ef;
    classDef process fill:transparent,stroke:#fdd0a4,stroke-width:2px,color:#f8fafc;
    classDef write fill:transparent,stroke:#87d6be,stroke-width:2px,color:#e6f4ef;

    class Tenant read;
    class Ctrl,App,Manager process;
    class RBAC,Secrets,Labels,Quotas,Ready write;`}
/>

<DecisionTable
  kind="reference"
  title="Security guardrails"
  columns={['Concern', 'Manager behavior']}
  rows={[
    {
      cells: ['Namespace targeting', 'Self-service tenants may target only their own namespace; cross-namespace provisioning is reserved for trusted operator-managed cases.'],
      emphasis: 'recommended',
    },
    {
      cells: ['Admission dependencies', 'Tenant Secret allowlists wait for admission-policy dependencies so Secret access is not widened before enforcement is ready.'],
    },
    {
      cells: ['Cluster-controller handoff', 'The tenant `RoleBinding` is the readiness marker that allows `OpenBaoCluster` reconciliation to proceed in multi-tenant mode.'],
    },
    {
      cells: ['Shared RBAC lifecycle', 'Provisioned tenant RBAC avoids OwnerReferences that would let a single cluster deletion garbage-collect shared namespace permissions.'],
    },
    {
      cells: ['Secret scope', 'Reader and writer Secret Roles are derived from explicit cluster references instead of wildcard list or get access to every Secret in the namespace.'],
    },
  ]}
/>

<DecisionTable
  kind="reference"
  title="Deletion lifecycle"
  columns={['Phase', 'Manager intent']}
  rows={[
    {
      cells: ['Tenant marked for deletion', 'Keep provisioned RBAC in place while managed clusters still exist in the namespace.'],
      emphasis: 'recommended',
    },
    {
      cells: ['Namespace emptied of managed clusters', 'Remove provisioned Role, RoleBinding, Secret allowlist roles, and default quota resources.'],
    },
    {
      cells: ['Finalizer removal', 'Only remove the tenant finalizer after namespace-scoped provisioning artifacts are cleaned up.'],
    },
  ]}
/>

<NextActions
  title="Related deep dives"
  items={[
    {
      label: 'Tenant onboarding guide',
      description: 'Compare the internal provisioning contract with the user-facing onboarding sequence for OpenBaoTenant.',
      docId: 'user-guide/openbaotenant/onboarding',
    },
    {
      label: 'Admission policies',
      description: 'See why tenant Secret allowlists wait for infrastructure policy dependencies before broadening access.',
      docId: 'security/infrastructure/admission-policies',
    },
    {
      label: 'Day 0 lifecycle flow',
      description: 'Follow where tenant provisioning sits before cluster creation begins.',
      docId: 'architecture/lifecycle/day0-provisioning',
    },
  ]}
/>
