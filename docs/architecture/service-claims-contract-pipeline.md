---
title: Service-claim contract pipeline
description: Maintainer architecture for catalog resolution, approved contracts, rendered contracts, same-cluster materialization, and connection publication.
hide_title: true
pageType: concept
journey: architecture
---

<PageHeader
  title="Service-claim contract pipeline"
  lede="Claim reconciliation moves through explicit contract stages before it touches the direct OpenBaoCluster runtime. Each stage narrows the input surface and records enough identity to detect drift."
/>

<DiagramFrame
  title="Pipeline stages"
  caption="The controller accepts a tenant claim, resolves catalog intent, binds an approved contract, renders execution inputs, materializes the same-cluster runtime, then publishes the tenant-facing connection contract."
  code={`graph LR
    Accept["Accept claim"] --> Catalog["Resolve catalog"]
    Catalog --> Approved["Bind approved contract"]
    Approved --> Rendered["Render execution contract"]
    Rendered --> Materialize["Materialize OpenBaoCluster"]
    Materialize --> Publish["Publish connection"]
    Publish --> Status["Summarize status"]

    classDef input fill:transparent,stroke:#fdd0a4,stroke-width:2px,color:#e6f4ef;
    classDef stage fill:transparent,stroke:#87d6be,stroke-width:2px,color:#e6f4ef;
    classDef output fill:transparent,stroke:#79c0ab,stroke-width:2px,color:#e6f4ef;

    class Accept,Catalog input;
    class Approved,Rendered,Materialize stage;
    class Publish,Status output;`}
/>

## Accept the claim

Acceptance checks only the request boundary:

- the claim controller is enabled
- the referenced `OpenBaoTenant` exists and has completed namespace handoff
- the claim selects either a stable offering alias or an explicit service
  profile revision according to the supported API rules
- post-materialization edits do not try to change locked service identity

Do not add workload reconciliation behavior to this stage. Acceptance should
decide whether the claim can enter catalog binding, not construct runtime
objects.

## Resolve the catalog

Catalog resolution loads the stable alias and immutable profile revisions:

- `OpenBaoServiceOffering`
- `OpenBaoServiceProfile`
- `OpenBaoExposureClass`
- `OpenBaoBootstrapProfile`
- `OpenBaoBackupProfile`
- optional storage, unseal, runtime, observability, network, and upgrade
  profiles
- lower backup and exposure implementation objects referenced by those profiles

Existing claims do not live-follow offering changes. The controller records the
bound revision identity in status so later reconciliation can distinguish normal
requeue from catalog drift.

## Bind the approved contract

The approved contract is the platform policy decision. It combines:

- the immutable service profile and auxiliary profile revisions
- the bounded tenant service parameters the catalog allows
- compatibility checks that reject unsupported same-cluster shapes
- provenance for every catalog object that affects the materialized service

This stage is where the implementation must fail closed when the requested
service cannot be represented honestly by the direct same-cluster runtime.

<DecisionTable
  kind="reference"
  title="Approved contract responsibilities"
  columns={['Responsibility', 'Reason']}
  rows={[
    {
      cells: ['Bind immutable catalog identity', 'Later reconciles need to know exactly which profile revisions were applied.'],
      emphasis: 'recommended',
    },
    {
      cells: ['Validate bounded tenant parameters', 'Hostname and backup parameters must be allowed by the selected catalog object before rendering.'],
    },
    {
      cells: ['Enforce production posture', 'Hardened services still require a non-static unseal path, trusted TLS posture, and self-init-compatible lifecycle auth.'],
    },
    {
      cells: ['Block unsupported shapes', 'The claim path must not invent runtime behavior that the direct `OpenBaoCluster` API cannot carry.'],
    },
  ]}
/>

## Render the execution contract

The rendered contract turns approved policy into concrete same-cluster execution
inputs:

- materialized cluster name, labels, annotations, and ownership markers
- storage, backup, restore, and upgrade helper image choices
- exposure, TLS, hostname, and edge publication inputs
- bootstrap source projections
- network dependencies and endpoint publication expectations
- unseal configuration and same-cluster transit defaults where applicable

Rendered output remains internal. Tenant users see claim status and the
connection Secret, not the rendered contract.

## Materialize the same-cluster runtime

Same-cluster materialization creates or patches a claim-managed
`OpenBaoCluster`. That object remains the concrete workload runtime seam.

Materialization must preserve these rules:

- claim-managed clusters carry ownership labels that identify the claim
- non-controller identities cannot create, mutate, or delete claim-managed local
  clusters as normal workflow
- workload managers still own bootstrap, networking, identity, StatefulSet,
  backup, restore, and upgrade behavior after the local cluster exists
- unsupported catalog shapes stop before runtime mutation

## Publish the connection contract

Connection publication turns runtime readiness into tenant-facing output:

- internal endpoints can publish once the local service is ready
- ingress and gateway endpoints wait for edge integration readiness
- connection Secrets are custody-checked operator-managed outputs
- claim status summarizes whether the service is ready, pending, degraded, or
  blocked

Do not shortcut endpoint publication by assuming an edge object exists. External
endpoint readiness is part of the claim contract.

## Maintain the stage boundary

<DecisionTable
  kind="reference"
  title="Where changes belong"
  columns={['Change', 'Stage']}
  rows={[
    {
      cells: ['New catalog object or profile reference', 'Catalog resolution and approved contract binding.'],
      emphasis: 'recommended',
    },
    {
      cells: ['New bounded tenant parameter', 'Approved contract validation, rendered contract projection, and API/status docs.'],
    },
    {
      cells: ['New same-cluster workload projection', 'Rendered contract and materialization, backed by direct `OpenBaoCluster` support first.'],
    },
    {
      cells: ['New tenant-facing endpoint signal', 'Connection publication and claim status.'],
    },
    {
      cells: ['New disruptive day-2 action', 'Dedicated immutable request API, not post-materialization claim spec edits.'],
    },
  ]}
/>

<NextActions
  title="Related architecture"
  items={[
    {
      label: 'Review claim boundaries',
      description: 'See which code and API surfaces own catalog, materialization, and day-2 behavior.',
      docId: 'architecture/service-claims-boundaries',
    },
    {
      label: 'Read service-claim architecture',
      description: 'Return to the high-level claim architecture page.',
      docId: 'architecture/service-claims',
    },
    {
      label: 'Open component design',
      description: 'Place the claim pipeline in the broader controller and app-layer model.',
      docId: 'architecture/components',
    },
  ]}
/>
