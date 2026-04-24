---
title: Understand the service catalog
description: Platform-owned catalog objects behind OpenBaoClusterClaim, including stable offerings, immutable profiles, exposure policy, bootstrap policy, and backup policy.
slug: /service-claims/catalog
hide_title: true
pageType: concept
journey: get-started
---

<PageHeader
  title="Understand the service catalog"
  lede="The service catalog is the platform-owned policy surface behind OpenBaoClusterClaim. Tenant users bind through a stable offering alias, while platform admins manage the immutable service revisions and the lower policy objects they reference."
/>

<DecisionTable
  title="Catalog objects at a glance"
  columns={['Object', 'Primary owner', 'Why it exists', 'Mutability']}
  rows={[
    {
      cells: [
        'OpenBaoServiceOffering',
        'Platform admin',
        'Stable friendly alias that points new claims at the current immutable service-profile revision.',
        'Mutable. Existing claims do not live-follow later offering changes.',
      ],
      emphasis: 'recommended',
    },
    {
      cells: [
        'OpenBaoServiceProfile',
        'Platform admin',
        'Immutable service baseline for cluster shape, bootstrap mode, exposure class, implementation profile refs, lifecycle defaults, and backup profile.',
        'Immutable by design once published.',
      ],
    },
    {
      cells: [
        'OpenBaoStorageProfile, OpenBaoUnsealProfile, OpenBaoRuntimeProfile, OpenBaoNetworkProfile, OpenBaoUpgradePolicy, and OpenBaoObservabilityProfile',
        'Platform admin',
        'Carry storage classes, ACME cache storage, unseal provider posture, workload identity metadata, helper images, read-replica runtime settings, network dependencies, upgrade defaults, metrics, and telemetry.',
        'Immutable by design once published.',
      ],
    },
    {
      cells: [
        'OpenBaoExposureClass and OpenBaoIngressPolicy',
        'Platform admin',
        'Bound the exposure surface and edge-controller integration choices tenant users are allowed to consume.',
        'Immutable by design once published.',
      ],
    },
    {
      cells: [
        'OpenBaoBootstrapProfile, OpenBaoBackupProfile, OpenBaoBackupTarget, and related lower policy objects',
        'Platform admin',
        'Carry bootstrap, backup, and lower execution-policy details that do not belong on the claim surface.',
        'Treat as platform-owned policy objects. Publish a new revision instead of mutating in place when the contract changes materially.',
      ],
    },
  ]}
/>

<DiagramFrame
  title="Catalog binding model"
  caption="A claim should normally bind through a stable offering alias. The controller resolves that alias to the current immutable service-profile revision, then records the applied revision identity in claim status."
  code={`graph LR
    Claim["OpenBaoClusterClaim"] --> Offering["OpenBaoServiceOffering"]
    Offering --> Profile["OpenBaoServiceProfile"]
    Profile --> Exposure["OpenBaoExposureClass"]
    Profile --> Storage["OpenBaoStorageProfile"]
    Profile --> Unseal["OpenBaoUnsealProfile"]
    Profile --> Runtime["OpenBaoRuntimeProfile"]
    Profile --> Network["OpenBaoNetworkProfile"]
    Profile --> Upgrade["OpenBaoUpgradePolicy"]
    Profile --> Observability["OpenBaoObservabilityProfile"]
    Profile --> Bootstrap["OpenBaoBootstrapProfile"]
    Profile --> Backup["OpenBaoBackupProfile"]

    classDef actor fill:transparent,stroke:#fdd0a4,stroke-width:2px,color:#e6f4ef;
    classDef control fill:transparent,stroke:#87d6be,stroke-width:2px,color:#e6f4ef;
    classDef data fill:transparent,stroke:#79c0ab,stroke-width:2px,color:#e6f4ef;

    class Claim actor;
    class Offering,Profile control;
    class Exposure,Storage,Unseal,Runtime,Network,Upgrade,Observability,Bootstrap,Backup data;`}
/>

## Use the offering alias as the tenant entry point

Use `spec.serviceOfferingRef` for tenant-facing claim submission. That gives the platform team room to publish a friendly stable name such as `dev-internal` or `hardened-edge`, while still keeping the actual service profile immutable.

<CommandBlock
  language="yaml"
  label="configure"
  title="Publish a stable service offering alias"
  code={`apiVersion: openbao.org/v1alpha1
kind: OpenBaoServiceOffering
metadata:
  name: dev-internal
spec:
  currentRevisionRef:
    name: dev-internal-v1`}
>
  New claims bind through `dev-internal`. Existing claims keep the immutable revision they already applied, even if the offering later advances to another revision.
</CommandBlock>

## Keep tenant and platform surfaces separate

Use the claim model when the platform wants to keep these decisions centrally shaped:

- bootstrap mode and bootstrap dependencies
- exposure mode and edge-controller integration
- storage classes and ACME shared-cache storage
- unseal provider selection and credential posture
- workload identity, pod metadata, image pulls, hardening, and security context
- helper image and read-replica runtime posture
- network dependencies and approved ingress/egress peers
- metrics, ServiceMonitor, and OpenBao telemetry posture
- backup baseline and lower target wiring
- lifecycle and blue/green upgrade defaults that should not vary per tenant request

Keep only bounded tenant choices on the claim surface, such as:

- tenant identity
- selected service offering
- narrow service parameters the profile explicitly permits, such as bounded backup
  parameters or an exposure hostname allowed by the selected exposure class

<Callout type="warning" title="Do not treat the catalog as a tenant-editable control plane">

Catalog objects are part of the platform policy surface. Tenant users should not edit service profiles, exposure classes, ingress policies, or backup targets directly just because they can read them.

</Callout>

<NextActions
  title="Continue catalog-driven provisioning"
  items={[
    {
      label: 'Apply the first claim',
      description: 'Use the same-cluster quickstart once the offering and its backing catalog objects exist.',
      docId: 'user-guide/service-claims/getting-started',
    },
    {
      label: 'Plan exposure',
      description: 'See how the cataloged exposure class changes endpoint publication for internal, ingress, and gateway service shapes.',
      docId: 'user-guide/service-claims/exposure',
    },
    {
      label: 'Open the API reference',
      description: 'Use the reference when you need exact schema details for claim or catalog objects.',
      docId: 'reference/api',
    },
  ]}
/>
