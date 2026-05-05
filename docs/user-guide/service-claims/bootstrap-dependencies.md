---
title: Understand bootstrap dependencies
description: Self-init bootstrap behavior for service claims, including secret-backed bootstrap sources and their projection into the tenant namespace.
slug: /service-claims/bootstrap-dependencies
hide_title: true
pageType: concept
journey: get-started
---

<PageHeader
  title="Understand claim bootstrap dependencies"
  lede="The supported claim bootstrap path today is self-init. Platform-owned bootstrap profiles can reference Secret and ConfigMap sources, and the controller projects those dependencies into the tenant namespace. Auth and policy sources feed self-init requests; audit sinks are resolved into declarative `OpenBaoCluster.spec.audit` configuration."
/>

<Callout type="warning" title="Supported bootstrap mode">

The supported claim path today is `SelfInit`. Managed initialization and broader bootstrap-migration workflows are not part of the supported claim surface yet.

</Callout>

<DecisionTable
  title="Bootstrap dependency model"
  columns={['Surface', 'Owner', 'Why it exists', 'What tenant users see']}
  rows={[
    {
      cells: [
        'OpenBaoBootstrapProfile',
        'Platform admin',
        'Defines the bootstrap contract behind the service profile, including lifecycle auth, auth-method requests, policies, and audit-device configuration.',
        'The claim applies the service offering. It does not embed the bootstrap contract directly.',
      ],
      emphasis: 'recommended',
    },
    {
      cells: [
        'Secret and ConfigMap source objects',
        'Platform admin or another approved producer in the tenant namespace',
        'Carry sensitive or large bootstrap data such as auth-method config, policy content, or audit sink settings.',
        'The claim can remain small; projected references and rendered declarative audit configuration stay operator-managed.',
      ],
    },
    {
      cells: [
        'Projected bootstrap artifacts',
        'Controller-managed',
        'Create the concrete same-cluster execution boundary the materialized OpenBaoCluster can consume safely.',
        'These are operator-managed artifacts. Do not mutate them directly.',
      ],
    },
  ]}
/>

## What gets projected

The current same-cluster claim path supports secret-backed or configmap-backed sources for:

- auth-method config
- policy content
- audit sink configuration, rendered onto the materialized cluster as `spec.audit`

The controller projects the needed artifacts into the tenant namespace, records the applied projection identity in claim status, and prunes those projected artifacts on claim deletion.

<CommandBlock
  language="yaml"
  label="configure"
  title="Example bootstrap source Secret"
  code={`apiVersion: v1
kind: Secret
metadata:
  name: authcfg-team-a
  namespace: team-a-prod
type: Opaque
stringData:
  kubernetes_host: https://kubernetes.default.svc
  issuer: https://kubernetes.default.svc.cluster.local`}
>
  Platform-owned bootstrap profiles can reference Secret or ConfigMap objects like this instead of requiring the bootstrap content inline on the materialized OpenBaoCluster.
</CommandBlock>

<CommandBlock
  language="yaml"
  label="configure"
  title="Example audit sink source Secret"
  code={`apiVersion: v1
kind: Secret
metadata:
  name: audit-file-default
  namespace: team-a-prod
type: Opaque
stringData:
  sink.json: |
    {
      "path": "stdout",
      "description": "stdout audit",
      "fileOptions": {
        "filePath": "stdout"
      }
    }`}
>
  Claim materialization reads this sink shape and renders a declarative audit device on the materialized `OpenBaoCluster`; it does not create a `sys/audit/*` self-init request.
</CommandBlock>

<Callout type="note" title="Custody boundary">

The projected bootstrap artifacts and the claim connection Secret are operator-managed objects with explicit custody checks. Treat them as outputs of the claim runtime, not as general-purpose tenant inventory.

</Callout>

<NextActions
  title="Continue the claim path"
  items={[
    {
      label: 'Understand catalog objects',
      description: 'See how bootstrap profiles fit into the wider service catalog behind the claim.',
      docId: 'user-guide/service-claims/service-catalog',
    },
    {
      label: 'Troubleshoot claim services',
      description: 'Use the troubleshooting page when bootstrap dependencies are missing or remain pending.',
      docId: 'user-guide/service-claims/troubleshooting',
    },
    {
      label: 'Read self-init for direct clusters',
      description: 'Use the direct-cluster self-init page when you are not provisioning through the claim model.',
      docId: 'user-guide/openbaocluster/configuration/self-init',
    },
  ]}
/>
