---
title: Service-claim boundaries
description: Maintainer architecture rules for service-claim catalog ownership, package boundaries, day-2 request APIs, and fail-closed unsupported shapes.
hide_title: true
pageType: concept
journey: architecture
---

<PageHeader
  title="Service-claim boundaries"
  lede="Service claims are a vertical module around the direct OpenBaoCluster runtime. The claim layer owns service-request policy and materialization intent; the core lifecycle remains the workload substrate."
/>

<Callout type="note" title="Core remains the substrate">

`OpenBaoCluster` lifecycle managers own the concrete workload. Service claims
consume that seam through CRDs, status, and narrow services instead of becoming
a second hidden lifecycle engine.

</Callout>

## Module ownership

<DecisionTable
  kind="reference"
  title="Claim module boundaries"
  columns={['Surface', 'Owner', 'Boundary rule']}
  rows={[
    {
      cells: ['Claim API and request APIs', 'Service-claims module', 'Tenant-facing service intent and day-2 request intent stay in claim-owned CRDs.'],
      emphasis: 'recommended',
    },
    {
      cells: ['Service catalog APIs', 'Service-claims module', 'Catalog objects are platform-owned policy. Tenant claims reference them but do not own them.'],
    },
    {
      cells: ['Direct OpenBaoCluster runtime', 'Core lifecycle module', 'Claims materialize supported same-cluster workloads through `OpenBaoCluster` and do not bypass workload managers.'],
    },
    {
      cells: ['Tenant onboarding and namespace guardrails', 'Tenant/provisioner module', 'Claims wait for tenant handoff instead of provisioning tenant access themselves.'],
    },
    {
      cells: ['Backup, restore, upgrade execution', 'Core lifecycle managers plus claim request controllers', 'Claim request controllers create or steer bounded intent; core managers execute concrete lifecycle work.'],
    },
    {
      cells: ['Shared labels, annotations, status apply, errors, admission checks', 'Shared platform kernel', 'Keep only cross-module protocol in shared packages. Move module-specific reasons close to the owning module.'],
    },
  ]}
/>

## Package structure

The service-claims implementation is grouped under the `openbaoclusterclaim`
module path while keeping each primary controller resource separate:

```text
internal/app/openbaoclusterclaim
internal/app/openbaoclusterclaim/backuprequest
internal/app/openbaoclusterclaim/restorerequest
internal/app/openbaoclusterclaim/upgraderequest

internal/controller/openbaoclusterclaim
internal/controller/openbaoclusterclaim/claimwatch
internal/controller/openbaoclusterclaim/requestwatch
internal/controller/openbaoclusterclaim/backuprequest
internal/controller/openbaoclusterclaim/restorerequest
internal/controller/openbaoclusterclaim/upgraderequest
internal/controller/openbaoclusterclaim/watchutil
```

The root claim app owns catalog binding, materialization, connection
publication, and claim status roll-up. The request subpackages own durable
backup, restore, and upgrade request reconciliation for their own CRDs.
Controller helpers under `claimwatch`, `requestwatch`, and `watchutil` are
module-local plumbing: they map related resource events back to claim or
claim-request keys and share metric-sync behavior without turning the root
controller into a mixed-concern file.

## Import and dependency rules

Keep the dependency direction explicit:

- core lifecycle code must not import claim, catalog, multi-cluster, federation,
  or other optional platform modules
- claim code may depend on core API contracts and narrow services, but not on
  controller internals from other modules
- controllers should not import other resource reconcilers or controller
  implementation packages; module-local watch helper packages are allowed when
  they do not own reconciliation themselves
- app-layer orchestration should not import adapters directly
- optional modules should communicate through approved CRDs, status, and service
  contracts rather than package reach-through

Architecture-boundary checks should enforce these rules in CI. If a new package
needs an exception, update the architecture policy and this page in the same
change set.

## Catalog ownership rules

Catalog objects are platform-owned. The tenant claim can select a service
offering and provide bounded service parameters only when the selected catalog
object permits them.

<DecisionTable
  kind="reference"
  title="Catalog decisions"
  columns={['Decision', 'Correct home', 'Do not put it on']}
  rows={[
    {
      cells: ['Service version, voter count, read replicas, security profile, capacity', '`OpenBaoServiceProfile`', 'Tenant claim parameters.'],
      emphasis: 'recommended',
    },
    {
      cells: ['Storage class and ACME cache storage', '`OpenBaoStorageProfile`', 'Tenant claim parameters or raw OpenBaoCluster passthrough.'],
    },
    {
      cells: ['Unseal provider, provider config, credential Secret reference', '`OpenBaoUnsealProfile`', 'Tenant claim parameters.'],
    },
    {
      cells: ['ServiceAccount, pod metadata, pull secrets, helper images, image verification, hardening', '`OpenBaoRuntimeProfile`', 'Tenant claim parameters.'],
    },
    {
      cells: ['API server, DNS, ingress, and egress dependencies', '`OpenBaoNetworkProfile` and lower implementation objects', 'Tenant-authored NetworkPolicy snippets.'],
    },
    {
      cells: ['Metrics, ServiceMonitor, telemetry posture', '`OpenBaoObservabilityProfile`', 'Raw tenant telemetry config.'],
    },
    {
      cells: ['Gateway, ingress, TLS, ACME, hostname policy', '`OpenBaoExposureClass`, entrypoint, and ingress policy objects', 'Tenant-authored edge resources.'],
    },
    {
      cells: ['Manual backup, restore, upgrade execution', 'Immutable request APIs', 'Post-materialization claim spec edits.'],
    },
  ]}
/>

## Day-2 workflow boundary

Day-2 operations that change runtime state use request APIs:

- `OpenBaoServiceOfferingRollout`
- `OpenBaoClusterClaimUpgradeRequest`
- `OpenBaoClusterClaimBackupRequest`
- `OpenBaoClusterClaimRestoreRequest`

These APIs are immutable and status-driven. `OpenBaoServiceOfferingRollout`
orchestrates platform-owned offering movement by creating per-claim upgrade
requests; the generated request objects still own classification and execution.
Add new disruptive operations as explicit workflow APIs for the same reason:
they need classification, lock awareness, observable state, and clear failure
semantics. The root claim app observes active request and restore-execution
state only to derive claim phase, summary, and workflow sub-status; it does not
execute the underlying backup, restore, or upgrade lifecycle itself.

Do not use claim spec mutation as a shortcut for rollout, migration, restore,
restart, or maintenance behavior.

## Fail-closed unsupported shapes

The same-cluster claim path can support only service shapes that project
honestly into the direct `OpenBaoCluster` runtime. When the selected catalog
requires behavior that the direct runtime cannot represent, the claim should
block with a clear status reason.

Examples that must remain fail-closed until explicit support exists:

- raw OpenBao configuration passthrough
- non-`SelfInit` bootstrap modes
- adoption of existing direct clusters
- migration between same-cluster and multi-cluster execution
- arbitrary restore-source selection
- hidden runtime changes through post-materialization claim spec edits

## Status ownership

Each module writes only its own status fields:

- claim controllers write claim and claim-request status
- core lifecycle controllers write `OpenBaoCluster` and `OpenBaoRestore` status
- provisioner writes tenant onboarding status

Claim status may summarize the referenced local cluster, restore, backup, or
upgrade request. It must not become a second source of truth for fields owned by
the core runtime.

<NextActions
  title="Related architecture"
  items={[
    {
      label: 'Follow the contract pipeline',
      description: 'See where catalog binding, rendering, materialization, and publication happen.',
      docId: 'architecture/service-claims-contract-pipeline',
    },
    {
      label: 'Review operator invariants',
      description: 'Connect these boundaries to the cross-cutting lifecycle and claim invariants.',
      docId: 'architecture/operator-invariants',
    },
    {
      label: 'Open component design',
      description: 'See how claim controllers fit into the broader controller/app/service layering.',
      docId: 'architecture/components',
    },
  ]}
/>
