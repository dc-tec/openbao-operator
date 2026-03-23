---
title: Tenancy & Governance
description: Understand how OpenBaoTenant introduces namespaces, default guardrails, and the shared-operator boundary in the multi-tenant model.
slug: /tenant-onboarding
hide_title: true
pageType: concept
journey: get-started
---

<PageHero
  variant="compact"
  eyebrow="Tenant Onboarding"
  title="Introduce namespaces deliberately instead of letting the operator discover them."
  lede="`OpenBaoTenant` is the namespace-introduction contract in the default multi-tenant model. It tells the operator which namespace should become an authorized tenant and lets the control plane create the RBAC and guardrails that make shared operation safe."
  actions={[
    {label: 'Onboard the target namespace', docId: 'user-guide/openbaotenant/onboarding', variant: 'primary'},
    {label: 'Review multi-tenant security', docId: 'user-guide/openbaotenant/multi-tenancy', variant: 'secondary'},
  ]}
>
  <Checklist
    title="Use this page when you need to"
    items={[
      'understand why the default operator model does not discover namespaces implicitly',
      'explain what OpenBaoTenant introduces before the first cluster appears',
      'choose between self-service and centrally managed onboarding',
      'reason about tenant guardrails separately from cluster configuration',
    ]}
  />
</PageHero>

<DiagramFrame
  title="OpenBaoTenant is the namespace introduction point"
  caption="The Provisioner reacts to OpenBaoTenant, introduces the namespace boundary, and only then can the rest of the operator safely manage cluster resources there."
  code={`graph LR
    Tenant["Target namespace owner or platform admin"] --> Request["OpenBaoTenant"]
    Request --> Provisioner["Provisioner controller"]
    Provisioner --> Guardrails["RBAC, quota, limit range, guardrail labels"]
    Guardrails --> Cluster["OpenBaoCluster lifecycle in the tenant namespace"]

    classDef actor fill:transparent,stroke:#fdd0a4,stroke-width:2px,color:#e6f4ef;
    classDef control fill:transparent,stroke:#87d6be,stroke-width:2px,color:#e6f4ef;
    classDef data fill:transparent,stroke:#79c0ab,stroke-width:2px,color:#e6f4ef;

    class Tenant actor;
    class Request,Provisioner control;
    class Guardrails,Cluster data;`}
/>

<DecisionTable
  title="What OpenBaoTenant owns"
  columns={['Surface', 'Why it exists', 'What it is not']}
  rows={[
    {
      cells: [
        'Namespace introduction',
        'The operator only acts on namespaces that were introduced explicitly.',
        'It is not broad namespace discovery or a cluster-wide wildcard.',
      ],
      emphasis: 'recommended',
    },
    {
      cells: [
        'Tenant RBAC',
        'The Provisioner creates the namespace-scoped RBAC the operator needs to manage OpenBao resources there.',
        'It is not a request to grant arbitrary Secret access to tenant users.',
      ],
    },
    {
      cells: [
        'Default guardrails',
        'Tenant quotas, limit ranges, and namespace guardrail labels can be introduced as part of onboarding.',
        'It is not per-cluster tuning for the OpenBao workload itself.',
      ],
    },
  ]}
/>

<DecisionTable
  title="Choose the governance model"
  columns={['Model', 'Who creates the request', 'Best fit', 'Tradeoff']}
  rows={[
    {
      cells: [
        'Self-service',
        'Namespace owners create OpenBaoTenant in their own namespace.',
        'High-trust platform environments where teams already own namespace boundaries.',
        'The request can only target the same namespace and uses default guardrails.',
      ],
      emphasis: 'recommended',
    },
    {
      cells: [
        'Centrally managed',
        'Platform admins create OpenBaoTenant from the operator namespace.',
        'Stricter environments that want review, auditability, or custom tenant guardrails.',
        'The platform team owns more of the namespace introduction workflow.',
      ],
    },
  ]}
/>

<Callout type="note" title="API contract">

`spec.targetNamespace` is immutable after creation.
If the target namespace changes, delete and recreate the `OpenBaoTenant` instead of trying to mutate it in place.

</Callout>

<NextActions
  title="Continue from the concept into the task"
  items={[
    {
      label: 'Onboard the target namespace',
      description: 'Use the step-by-step task page when you are ready to create the OpenBaoTenant request.',
      docId: 'user-guide/openbaotenant/onboarding',
    },
    {
      label: 'Create your first cluster',
      description: 'Return to the main path once the namespace introduction is complete.',
      docId: 'user-guide/openbaocluster/getting-started',
    },
    {
      label: 'Open multi-tenant security',
      description: 'Go deeper on the isolation model when you need the security reasoning behind the shared-operator path.',
      docId: 'user-guide/openbaotenant/multi-tenancy',
    },
  ]}
/>
