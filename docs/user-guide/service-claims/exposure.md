---
title: Plan claim exposure
description: Exposure choices for OpenBaoClusterClaim, including cluster-internal, ingress, and gateway publication and the endpoint contract that tenants consume.
slug: /service-claims/exposure
hide_title: true
pageType: concept
journey: get-started
---

<PageHeader
  title="Plan claim exposure"
  lede="Claim users do not define ingress or gateway objects directly on the claim. Exposure stays platform-owned through the service catalog, and the claim reports a connection endpoint only after the chosen edge integration is actually ready."
/>

<DecisionTable
  title="Choose the cataloged exposure mode"
  columns={['Mode', 'Use it for', 'Claim behavior', 'Platform objects behind it']}
  rows={[
    {
      cells: [
        'ClusterInternal',
        'Tenant-local service consumption inside the cluster.',
        'The claim publishes an internal connection contract without waiting for edge-controller readiness.',
        'OpenBaoExposureClass only.',
      ],
      emphasis: 'recommended',
    },
    {
      cells: [
        'Ingress',
        'HTTP ingress-controller based publication through a platform-approved ingress shape.',
        'The claim endpoint is published only after ingress integration is ready and the ingress status shows a published address.',
        'OpenBaoExposureClass, OpenBaoEntrypoint, and OpenBaoIngressPolicy.',
      ],
    },
    {
      cells: [
        'Gateway',
        'Gateway API publication through a platform-managed Gateway and listener contract.',
        'The claim endpoint is published only after the referenced Gateway is programmed and the integration is ready.',
        'OpenBaoExposureClass and OpenBaoEntrypoint backed by a Gateway object.',
      ],
    },
  ]}
/>

<Callout type="note" title="Connection contract timing">

The claim connection Secret and `status.connection.endpoint` follow the selected exposure mode. For ingress and gateway publication, the endpoint is intentionally withheld until the controller can prove the external integration is actually ready.

</Callout>

## Keep edge policy in the catalog

Use the catalog when platform teams need to control:

- ingress-controller specific annotations and backend TLS publication
- gateway listener selection and hostname policy
- whether hostnames are generated, fixed, or claim-provided within an allowed suffix
- whether the workload listener uses operator-managed, external, or native OpenBao ACME TLS material

When native ACME is selected, keep the ACME directory, registration identity, domain policy, and shared RWX cache storage in platform-owned catalog objects. The ACME cache belongs with the storage profile because it is workload storage, not a tenant-facing certificate request.

Do not move these choices onto the tenant-facing claim just to reduce the number of catalog objects. That weakens the point of the claim model.

## Allow bounded tenant hostnames

Platform teams can allow a claim to request a hostname without giving tenants raw ingress or gateway control. Set `spec.hostnamePolicy.claim.enabled: true` on the exposure class and constrain the request with `allowedSuffixes`. The claim may then set `spec.serviceParameters.exposure.hostname`.

The controller fails closed when the requested hostname is not a valid DNS subdomain, the exposure class does not allow claim hostnames, or the hostname is outside the configured suffixes.

<DecisionTable
  kind="reference"
  title="What tenant users should watch"
  columns={['Signal', 'What it tells you', 'Check next']}
  rows={[
    {
      cells: [
        '`status.connection.endpoint` is empty',
        'The service is not ready for tenant consumption yet.',
        'Claim phase, materialized local cluster status, and ingress or gateway readiness.',
      ],
      emphasis: 'recommended',
    },
    {
      cells: [
        'Claim is Ready and the endpoint is internal',
        'The selected exposure class is internal-only or the external edge path is not part of the chosen offering.',
        'The applied exposure-class revision in claim status and the catalog object behind it.',
      ],
    },
    {
      cells: [
        'External endpoint exists but traffic still fails',
        'The edge controller may be ready enough to publish the route, but the workload or backend TLS path is still wrong.',
        'The materialized local OpenBaoCluster, ingress or gateway object status, and the claim connection Secret.',
      ],
    },
  ]}
/>

<NextActions
  title="Continue the claim path"
  items={[
    {
      label: 'Review bootstrap dependencies',
      description: 'Understand how secret-backed bootstrap inputs are projected into the tenant namespace for same-cluster execution.',
      docId: 'user-guide/service-claims/bootstrap-dependencies',
    },
    {
      label: 'Troubleshoot a claim',
      description: 'Route pending edge publication and readiness issues to the right surface quickly.',
      docId: 'user-guide/service-claims/troubleshooting',
    },
    {
      label: 'Configure direct-cluster external access',
      description: 'Use the direct OpenBaoCluster exposure docs when you are not using the claim path.',
      docId: 'user-guide/openbaocluster/configuration/external-access',
    },
  ]}
/>
