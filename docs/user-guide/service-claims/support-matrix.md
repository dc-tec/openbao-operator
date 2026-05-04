---
title: Service catalog support matrix
description: Map direct OpenBaoCluster configuration areas to service-catalog support, intentional exclusions, and future profile candidates for OpenBaoClusterClaim.
slug: /service-claims/support-matrix
hide_title: true
pageType: reference
journey: get-started
---

<PageHeader
  title="Service catalog support matrix"
  lede="Use this matrix to decide whether a direct OpenBaoCluster shape can be offered through OpenBaoClusterClaim, belongs in a platform profile, or should stay on the direct-cluster path."
/>

<Callout type="note" title="Claims are not a raw passthrough">

The service catalog intentionally represents curated service shapes. When a
direct `OpenBaoCluster` field is low-level, disruptive, or too environment
specific, the claim path either models it through a platform-owned profile or
keeps it unsupported.

</Callout>

<DecisionTable
  kind="reference"
  title="Supported through the catalog"
  columns={['Direct cluster area', 'Claim/catalog surface', 'Notes']}
  rows={[
    {
      cells: ['Version, voter count, read-replica count, security profile', '`OpenBaoServiceProfile.spec.cluster`', 'Only compatible in-place version changes are supported through claim upgrade requests. Other service-shape changes are blocked in this release.'],
      emphasis: 'recommended',
    },
    {
      cells: ['Primary and read-replica capacity', '`OpenBaoServiceProfile.spec.storage`', 'Capacity is part of the service profile. Storage implementation stays in `OpenBaoStorageProfile`.'],
    },
    {
      cells: ['Primary and read-replica storage class', '`OpenBaoStorageProfile`', 'Platform-owned storage classes and read-replica inheritance.'],
    },
    {
      cells: ['ACME shared cache storage', '`OpenBaoStorageProfile.spec.acmeCache`', 'Native OpenBao ACME needs RWX cache storage. Keep this platform-owned.'],
    },
    {
      cells: ['Self-init bootstrap, lifecycle JWT auth, auth methods, policies, audit bootstrap', '`OpenBaoBootstrapProfile`', 'The current claim runtime supports `SelfInit` only. Secret and ConfigMap-backed bootstrap dependencies are projected.'],
    },
    {
      cells: ['Internal, ingress, and gateway exposure', '`OpenBaoExposureClass`, `OpenBaoEntrypoint`, `OpenBaoIngressPolicy`', 'Endpoint publication waits for the selected edge integration to be ready.'],
    },
    {
      cells: ['Operator-managed, external, and native ACME listener TLS', '`OpenBaoExposureClass.spec.tlsPolicy`', 'ACME directory, email, and domain policy remain platform-owned.'],
    },
    {
      cells: ['Bounded tenant hostnames', '`OpenBaoExposureClass.spec.hostnamePolicy.claim` plus `OpenBaoClusterClaim.spec.serviceParameters.exposure.hostname`', 'Allowed only when the exposure class enables claim hostnames and suffix validation passes.'],
    },
    {
      cells: ['Transit, cloud KMS, OCI KMS, KMIP, and PKCS#11 unseal posture', '`OpenBaoUnsealProfile`', 'API and projection support exist. Provider-specific operational test depth may vary by environment.'],
    },
    {
      cells: ['ServiceAccount, pod metadata, image pulls, image verification, helper images, hardening, security context', '`OpenBaoRuntimeProfile`', 'Platform-owned runtime integration for cloud identity, private registries, OpenShift/SCC, and helper executors.'],
    },
    {
      cells: ['Metrics, ServiceMonitor, telemetry', '`OpenBaoObservabilityProfile`', 'Use profiles rather than tenant-authored raw telemetry settings.'],
    },
    {
      cells: ['API server, DNS, ingress, and egress network dependencies', '`OpenBaoNetworkProfile` plus backup backend required egress', 'Raw tenant network policy is intentionally not claim-facing.'],
    },
    {
      cells: ['Scheduled backups, retention, backup target wiring', '`OpenBaoBackupProfile` and lower backup implementation objects', 'Backup target and auth posture remain platform-owned. Tenant parameters stay bounded.'],
    },
    {
      cells: ['Rolling and blue/green upgrade defaults', '`OpenBaoServiceProfile.spec.lifecycle` and `OpenBaoUpgradePolicy`', 'One-shot upgrade execution is request based.'],
    },
  ]}
/>

<DecisionTable
  kind="reference"
  title="Request APIs instead of durable claim spec"
  columns={['Workflow', 'Use', 'Reason']}
  rows={[
    {
      cells: ['Upgrade', '`OpenBaoClusterClaimUpgradeRequest`', 'Upgrade classification and execution are rollout workflows, not free-form claim edits.'],
      emphasis: 'recommended',
    },
    {
      cells: ['Manual backup', '`OpenBaoClusterClaimBackupRequest`', 'Immediate snapshots are explicit operations against a ready same-cluster claim.'],
    },
    {
      cells: ['Restore', '`OpenBaoClusterClaimRestoreRequest`', 'Restore is destructive and bounded to the latest successful backup or a completed claim backup request for the same claim and local cluster.'],
    },
    {
      cells: ['Future restart or maintenance actions', 'Future request APIs', 'Operational actions should be explicit, serialized, and observable rather than durable service-profile fields.'],
    },
  ]}
/>

<DecisionTable
  kind="reference"
  title="Intentionally unsupported or deferred"
  columns={['Direct cluster area', 'Current claim posture', 'Use instead']}
  rows={[
    {
      cells: ['Raw OpenBao configuration', 'Unsupported as a passthrough.', 'Create curated profiles only when a product-level setting is worth offering. Use direct clusters for expert raw configuration.'],
      emphasis: 'recommended',
    },
    {
      cells: ['Listener, Raft, cache, plugin, and other low-level config tuning', 'Unsupported or future curated profiles only.', 'Keep on direct clusters until there is a named catalog product requirement.'],
    },
    {
      cells: ['Pause, break-glass acknowledgement, direct maintenance flags', 'Unsupported as tenant claim inputs.', 'Use direct admin workflows and narrow maintenance access.'],
    },
    {
      cells: ['Token-secret backup or upgrade auth', 'Unsupported for claim-managed services.', 'Use lifecycle JWT auth from bootstrap profiles.'],
    },
    {
      cells: ['Arbitrary restore source or raw snapshot key selection', 'Unsupported in claim restore requests.', 'Use completed claim backup requests or the direct `OpenBaoRestore` path for broader restore-source control.'],
    },
    {
      cells: ['Adoption of existing direct clusters', 'Deferred to a dedicated adoption workflow.', 'Keep existing workloads direct-managed until adoption preflight and ownership transfer exist.'],
    },
    {
      cells: ['Migration between same-cluster and multi-cluster execution', 'Deferred.', 'Treat the current public surface as same-cluster provisioning only.'],
    },
    {
      cells: ['Non-SelfInit bootstrap modes', 'Unsupported in the current claim runtime.', 'Use direct clusters for non-`SelfInit` workflows.'],
    },
  ]}
/>

## Future adoption planning

There is no public adoption workflow in the current claim release. Existing
direct `OpenBaoCluster` workloads remain direct-managed.

A future adoption workflow would need to compare the direct `OpenBaoCluster` to
the selected service profile and auxiliary profiles before ownership changes.
It should fail closed when a direct cluster depends on unsupported raw
configuration, unmodeled storage or unseal posture, custom image behavior, ACME
cache storage that is not represented, or lifecycle settings that the catalog
cannot express.

<NextActions
  title="Use the matrix"
  items={[
    {
      label: 'Publish a minimum catalog',
      description: 'Create the first internal offering inside the supported matrix.',
      docId: 'user-guide/service-claims/publish-service-catalog',
    },
    {
      label: 'Publish a production catalog',
      description: 'Add implementation profiles for hardened storage, unseal, runtime, network, observability, backup, and upgrade policy.',
      docId: 'user-guide/service-claims/publish-production-catalog',
    },
    {
      label: 'Read unsupported workflows',
      description: 'Check workflow boundaries that are intentionally outside the current claim release.',
      docId: 'user-guide/service-claims/unsupported-workflows',
    },
    {
      label: 'Open service-claim architecture',
      description: 'Review the fail-closed contract pipeline behind these support decisions.',
      docId: 'architecture/service-claims',
    },
  ]}
/>
