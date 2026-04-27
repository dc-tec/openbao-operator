---
title: Choose service claims
description: Decide when to use OpenBaoClusterClaim instead of managing OpenBaoCluster directly, and understand the roles and scope boundaries of the claim model.
slug: /service-claims/overview
hide_title: true
pageType: concept
journey: get-started
---

<PageHeader
  title="Choose the claim provisioning model"
  lede="Use service claims when platform teams want to publish bounded OpenBao service shapes and tenant users should request service through a stable catalog instead of composing an OpenBaoCluster directly."
/>

<Callout type="warning" title="Current scope">

The supported public scope today is same-cluster service claims with explicit request workflows for in-place upgrades, manual backups, and restore from the latest successful or selected completed claim backup request. Adoption, migration, non-`SelfInit` bootstrap modes, and broader multi-cluster claim convergence remain out of scope.

</Callout>

<DecisionTable
  title="Claims versus direct clusters"
  columns={['Question', 'Choose service claims', 'Choose direct OpenBaoCluster']}
  rows={[
    {
      cells: [
        'Who owns the workload shape?',
        'A platform team owns the catalog and wants tenant users to choose from bounded offerings.',
        'The team provisioning the service should own the workload contract directly.',
      ],
      emphasis: 'recommended',
    },
    {
      cells: [
        'How much configuration should tenant users see?',
        'Only a small claim surface such as tenant, offering, and bounded service parameters.',
        'The full cluster, bootstrap, exposure, and lifecycle surface.',
      ],
    },
    {
      cells: [
        'Do you need a stable alias over immutable revisions?',
        'Yes. Use OpenBaoServiceOffering so new claims bind through a friendly catalog alias.',
        'No. Direct clusters already point at the exact workload spec you apply.',
      ],
    },
    {
      cells: [
        'Do you need adoption or migration right now?',
        'No. These workflows are not part of the supported claim surface yet.',
        'Yes. Stay on the direct-cluster path until those workflows exist explicitly.',
      ],
    },
  ]}
/>

<DecisionTable
  title="Role split"
  columns={['Role', 'Primary responsibility', 'What it does not own']}
  rows={[
    {
      cells: [
        'Platform admin',
        'Install the operator with the claim surface enabled, publish the service catalog, and govern the exposure, bootstrap, and backup policy objects the catalog points at.',
        'Per-claim day-to-day tenant intent beyond the bounded claim surface.',
      ],
      emphasis: 'recommended',
    },
    {
      cells: [
        'Tenant user',
        'Create OpenBaoClusterClaim against an allowed tenant and service offering, then consume the published connection contract.',
        'Platform-owned catalog objects such as service profiles, exposure classes, ingress policies, or backup targets.',
      ],
    },
  ]}
/>

<DiagramFrame
  title="The user-facing claim flow"
  caption="Tenant onboarding stays the namespace introduction point. The claim selects a service offering, the controller binds that alias to an immutable profile revision, then materializes the same-cluster workload and publishes the connection contract."
  code={`graph LR
    Tenant["OpenBaoTenant"] --> Claim["OpenBaoClusterClaim"]
    Claim --> Offering["OpenBaoServiceOffering"]
    Offering --> Profile["OpenBaoServiceProfile"]
    Profile --> Materialize["Same-cluster OpenBaoCluster"]
    Materialize --> Connection["Connection Secret and endpoint"]

    classDef actor fill:transparent,stroke:#fdd0a4,stroke-width:2px,color:#e6f4ef;
    classDef control fill:transparent,stroke:#87d6be,stroke-width:2px,color:#e6f4ef;
    classDef data fill:transparent,stroke:#79c0ab,stroke-width:2px,color:#e6f4ef;

    class Tenant,Claim actor;
    class Offering,Profile,Materialize control;
    class Connection data;`}
/>

## Keep the supported path explicit

Use the current claim feature for these workflows:

1. same-cluster service provisioning
2. catalog-driven internal, ingress, or gateway exposure
3. self-init bootstrap through platform-owned bootstrap profiles
4. secret-backed bootstrap dependencies projected into the tenant namespace
5. explicit in-place upgrade requests inside the supported compatibility boundary
6. manual backup requests and restore requests against the latest successful or selected completed claim backup request

Do not use the claim surface yet for:

1. adopting an existing direct OpenBaoCluster into claim ownership
2. migrating between same-cluster and multi-cluster execution paths
3. treating free-form post-materialization claim edits as rollout automation
4. bootstrap modes other than `SelfInit`

## Follow the role-specific path

Platform admins publish catalog objects first. Tenant users then submit claims
against stable offerings. Operators use request APIs for day-2 actions after the
claim has reached `Ready`.

<DecisionTable
  title="Who does what"
  columns={['Role', 'First page', 'Main output']}
  rows={[
    {
      cells: [
        'Platform admin',
        'Publish a service catalog',
        'A stable `OpenBaoServiceOffering` backed by immutable service-profile and implementation-profile revisions.',
      ],
      emphasis: 'recommended',
    },
    {
      cells: [
        'Tenant user',
        'Apply the first claim',
        'An `OpenBaoClusterClaim` that selects an offering and receives a connection contract.',
      ],
    },
    {
      cells: [
        'Operator',
        'Run claim day-2 workflows',
        'Immutable request objects for upgrade, backup, and restore workflows.',
      ],
    },
  ]}
/>

<NextActions
  title="Continue the claim path"
  items={[
    {
      label: 'Publish a catalog',
      description: 'Create the platform-owned offering and profile revisions before tenants submit claims.',
      docId: 'user-guide/service-claims/publish-service-catalog',
    },
    {
      label: 'Apply the first claim',
      description: 'Use the same-cluster quickstart once the operator install, target namespace, and service catalog are ready.',
      docId: 'user-guide/service-claims/getting-started',
    },
    {
      label: 'Review the service catalog',
      description: 'See which objects remain platform-owned and how service offerings map claims to immutable service-profile revisions.',
      docId: 'user-guide/service-claims/service-catalog',
    },
    {
      label: 'Stay on direct clusters',
      description: 'Use the direct OpenBaoCluster path when you need the full workload surface or unsupported claim workflows.',
      docId: 'user-guide/openbaocluster/getting-started',
    },
  ]}
/>
