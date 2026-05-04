---
title: Extend service claims
description: Maintainer checklist for adding service-catalog fields, bounded tenant parameters, materialized projections, and claim day-2 workflows safely.
hide_title: true
pageType: concept
journey: architecture
---

<PageHeader
  title="Extend service claims"
  lede="Extend the claim module by choosing the right contract stage first. Catalog fields, tenant parameters, materialized runtime projections, and day-2 workflows each have different ownership and test requirements."
/>

<DecisionTable
  title="Choose the extension path"
  columns={['Change', 'Correct path', 'Do not use']}
  rows={[
    {
      cells: [
        'New platform-owned service decision',
        'Add or extend a catalog/profile field, then bind it into the approved contract.',
        'Tenant claim parameters.',
      ],
      emphasis: 'recommended',
    },
    {
      cells: [
        'New bounded tenant input',
        'Add a narrow `OpenBaoClusterClaim.spec.serviceParameters` field and validate it against the selected catalog object.',
        'Raw `OpenBaoCluster` passthrough.',
      ],
    },
    {
      cells: [
        'New workload behavior',
        'Add direct `OpenBaoCluster` support first, then render it from the claim execution contract.',
        'Hidden claim-only lifecycle logic.',
      ],
    },
    {
      cells: [
        'New disruptive operation',
        'Add an immutable claim request API with classification, status, serialization, and failure reasons.',
        'Post-materialization claim spec edits.',
      ],
    },
    {
      cells: [
        'New endpoint publication signal',
        'Add it to connection publication and claim status after the edge object has an honest readiness signal.',
        'Assuming external readiness from object creation alone.',
      ],
    },
  ]}
/>

## Add a catalog field

Use this path when the platform owns the decision.

Checklist:

1. Add the field to the owning catalog/profile API, not to the claim.
2. Resolve and record the catalog object identity in catalog binding when the field affects rendered behavior.
3. Add approved-contract validation so unsupported or incomplete profiles fail closed before runtime mutation.
4. Render the field into the execution contract only after direct `OpenBaoCluster` support exists.
5. Update the user-guide support matrix and API reference artifacts.
6. Add unit tests for catalog binding, rendered contract output, and unsupported-shape failure.

## Add a bounded tenant parameter

Use this path only when tenants need a narrow, policy-checked value.

Checklist:

1. Add the parameter under `OpenBaoClusterClaim.spec.serviceParameters`.
2. Add a catalog-side allow policy for the parameter.
3. Reject disallowed values during approved-contract binding.
4. Keep the rendered value deterministic and observable through claim/request status where useful.
5. Add admission, reconciliation, and troubleshooting docs for the accepted and rejected cases.

Current examples are bounded exposure hostnames and backup partitions.

## Add a day-2 workflow

Disruptive operations need request APIs because they need a durable audit trail,
status, serialization, and explicit failure semantics.

Checklist:

1. Add an immutable request CRD instead of making the claim spec editable.
2. Classify the request before mutating the claim or local runtime.
3. Serialize conflicting active requests for the same claim.
4. Surface the active request through claim status.
5. Use low-cardinality states and reasons that operators can alert on.
6. Add user-guide examples, status reference entries, metrics labels, and e2e coverage.

Current request controllers live below `internal/app/openbaoclusterclaim/<request>`
and `internal/controller/openbaoclusterclaim/<request>`. Keep request execution
logic in those subpackages. Use the root claim app only to observe the active
request and summarize it on `OpenBaoClusterClaim`.

## Add watch reactivity

Use module-local watch helpers when a related resource should requeue a claim or
claim request:

1. Put root-claim event mapping in `internal/controller/openbaoclusterclaim/claimwatch`.
2. Put request-controller event mapping in `internal/controller/openbaoclusterclaim/requestwatch`.
3. Put shared key construction or metric-sync helpers in `internal/controller/openbaoclusterclaim/watchutil`.
4. Do not import another resource reconciler to reuse watch code.
5. Keep watch fan-out narrow. Prefer claim-managed resources and explicit request
   references over broad Secret, ConfigMap, Gateway, or Ingress watches.

## Keep the runtime seam honest

The claim layer materializes same-cluster services through `OpenBaoCluster`.
When a requested catalog shape cannot be represented by the direct runtime,
block it with a clear status reason. Do not add claim-only special cases that
skip core lifecycle managers.

<NextActions
  title="Related architecture"
  items={[
    {
      label: 'Follow the contract pipeline',
      description: 'See where catalog resolution, approved contracts, rendered contracts, materialization, and publication happen.',
      docId: 'architecture/service-claims-contract-pipeline',
    },
    {
      label: 'Review claim boundaries',
      description: 'Use the module and ownership rules before changing package dependencies or status ownership.',
      docId: 'architecture/service-claims-boundaries',
    },
    {
      label: 'Check supported shapes',
      description: 'Keep user-facing catalog support aligned with implementation support.',
      docId: 'user-guide/service-claims/support-matrix',
    },
  ]}
/>
