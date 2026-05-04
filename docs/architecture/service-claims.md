---
title: Service claims
hide_title: true
pageType: concept
journey: architecture
description: Internal architecture for OpenBaoClusterClaim, including contract stages, same-cluster materialization, ownership boundaries, and current scope limits.
---

<PageHeader
  title="Service-claim architecture"
  lede="OpenBaoClusterClaim adds a catalog-driven provisioning and day-2 request path on top of the existing OpenBaoCluster runtime. The claim controller binds a tenant-facing request to immutable catalog revisions, renders an execution contract, materializes the same-cluster workload, and summarizes bounded claim-native workflows while preserving the direct OpenBaoCluster boundary as the runtime contract."
/>

<DecisionTable
  title="Why the claim layer exists"
  columns={['Need', 'Architectural answer', 'What it does not do']}
  rows={[
    {
      cells: [
        'Bound tenant-facing service requests',
        'OpenBaoClusterClaim exposes a small request surface and resolves through platform-owned catalog objects.',
        'It does not replace the direct OpenBaoCluster workload API as the runtime contract.',
      ],
      emphasis: 'recommended',
    },
    {
      cells: [
        'Stable aliases over immutable service revisions',
        'OpenBaoServiceOffering points new claims at the current immutable OpenBaoServiceProfile revision.',
        'Existing claims do not live-follow later offering moves.',
      ],
    },
    {
      cells: [
        'Same-cluster workload execution',
        'The claim pipeline still materializes into OpenBaoCluster for the supported path today.',
        'It is not a hidden second workload engine with its own runtime semantics.',
      ],
    },
  ]}
/>

<DiagramFrame
  title="Claim-to-runtime pipeline"
  caption="The claim path binds through the catalog, produces internal contract stages, materializes the same-cluster workload, and rolls direct-runtime and request-workflow state back into the claim."
  code={`graph LR
    Claim["OpenBaoClusterClaim"] --> Approved["Approved service contract"]
    Approved --> Rendered["Rendered execution contract"]
    Rendered --> Local["Same-cluster OpenBaoCluster"]
    Local --> Publish["Connection publication"]
    Day2["Claim workflow requests"] --> Status["Claim status summary"]
    Local --> Status
    Publish --> Status

    Catalog["Catalog objects"] --> Approved
    Rendered --> Deps["Projected bootstrap and edge dependencies"]

    classDef actor fill:transparent,stroke:#fdd0a4,stroke-width:2px,color:#e6f4ef;
    classDef control fill:transparent,stroke:#87d6be,stroke-width:2px,color:#e6f4ef;
    classDef data fill:transparent,stroke:#79c0ab,stroke-width:2px,color:#e6f4ef;

    class Claim,Catalog,Day2 actor;
    class Approved,Rendered,Local control;
    class Publish,Deps,Status data;`}
/>

## Keep the direct runtime seam

The claim layer exists to shape tenant intent and platform policy. It does not remove the need for an honest direct workload seam.

For the supported same-cluster path today:

1. the claim resolves catalog intent
2. the controller produces approved and rendered internal contracts
3. the system materializes a local `OpenBaoCluster`
4. workload managers still own bootstrap, networking, identity, initialization, and StatefulSet behavior behind that materialized cluster
5. claim request controllers serialize supported backup, restore, and in-place upgrade intent while core managers execute concrete lifecycle work

That design keeps the current architecture honest:

- new service shapes need a real same-cluster `OpenBaoCluster` seam before the claim path should support them
- unsupported claim workflows fail closed instead of inventing hidden controller behavior

## Ownership and custody boundaries

The claim path introduces additional operator-managed outputs in the tenant namespace:

- the materialized local `OpenBaoCluster`
- the claim connection Secret
- projected bootstrap dependency artifacts

The implementation keeps those boundaries explicit:

- claim-managed local clusters are protected from direct mutation and deletion
- connection and projected bootstrap artifacts use custody checks instead of name-only takeover
- service-offering alias binding is recorded as an applied immutable revision set in claim status

## Current scope boundary

The implemented public claim surface is intentionally narrower than the long-term design space.

Supported now:

- same-cluster provisioning
- stable offering alias binding
- internal, ingress, and gateway exposure through the cataloged path
- secret-backed self-init bootstrap projection
- claim-native backup, restore, and in-place upgrade request workflows
- claim status summaries, active maintenance projection, and claim workflow metrics

Deferred intentionally:

- adoption
- migration
- replacement-class rollout and migration workflows
- arbitrary post-materialization spec mutation as a workflow shortcut
- non-`SelfInit` bootstrap modes
- full multi-cluster claim convergence as the primary public story

<NextActions
  title="Related routes"
  items={[
    {
      label: 'Follow the contract pipeline',
      description: 'Trace catalog resolution, approved contracts, rendered contracts, materialization, and connection publication.',
      docId: 'architecture/service-claims-contract-pipeline',
    },
    {
      label: 'Review claim boundaries',
      description: 'Use the maintainer boundary rules before extending catalog, materialization, or day-2 workflow behavior.',
      docId: 'architecture/service-claims-boundaries',
    },
    {
      label: 'Extend service claims',
      description: 'Use the maintainer checklist for catalog fields, tenant parameters, runtime projections, and request workflows.',
      docId: 'architecture/service-claims-extension-guide',
    },
    {
      label: 'Open component design',
      description: 'Return to the broader controller, app, and manager boundaries for the rest of the operator.',
      docId: 'architecture/components',
    },
    {
      label: 'Open service claims',
      description: 'Use the user-guide section when you need the task flow and supported operational scope instead of the internal model.',
      docId: 'user-guide/service-claims/index',
    },
    {
      label: 'Review tenant isolation',
      description: 'Connect the claim custody and ownership model back to the shared-operator security posture.',
      docId: 'security/multi-tenancy/tenant-isolation',
    },
  ]}
/>
