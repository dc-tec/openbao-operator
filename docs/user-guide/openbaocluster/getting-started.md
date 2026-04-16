---
title: Create Your First Cluster
slug: /get-started/first-cluster
hide_title: true
description: Start with the closest safe OpenBaoCluster baseline, verify the important readiness signals, and move cleanly into day 2 work.
pageType: task
journey: get-started
journeyStep: 4
---

<PageHeader
  title="Create the first cluster you can keep operating."
  lede="By the time you reach this step, the operator is installed and the target namespace is already onboarded when you are in the default multi-tenant mode. Start with the closest safe baseline, verify the cluster becomes healthy, and then move directly into the next operating concern."
/>

<Checklist
    title="Cluster manifest checklist"
    items={[
      'confirm the operator install is healthy in the namespace model you chose',
      'confirm the target namespace is already onboarded through OpenBaoTenant when you are in multi-tenant mode',
      'choose a StorageClass explicitly for production before the first reconcile',
      'decide whether this cluster is only for evaluation or intended to become production',
    ]}
  />


<Callout type="note" title="Choose the namespace handoff first">

- In the default multi-tenant mode, create the target namespace and finish [OpenBaoTenant onboarding](../openbaotenant/onboarding.md) before you apply `OpenBaoCluster`.
- In single-tenant mode, skip `OpenBaoTenant` and create the cluster only in the controller's watched namespace.

</Callout>

<JourneyRail
  title="The first five moves"
  current={4}
  items={[
    {
      label: 'Choose a deployment model',
      description: 'Lock down tenancy, security posture, install method, and the main exceptions before you install.',
      docId: 'user-guide/operator/deployment-decision-guide',
    },
    {
      label: 'Install the operator',
      description: 'Render the right namespace, identity, and admission posture.',
      docId: 'user-guide/operator/installation',
    },
    {
      label: 'Onboard the target namespace',
      description: 'In the default multi-tenant path, introduce the namespace through OpenBaoTenant before you create a cluster.',
      docId: 'user-guide/openbaotenant/onboarding',
    },
    {
      label: 'Create your first cluster',
      description: 'Start with the closest cluster baseline and verify the important readiness signals.',
      docId: 'user-guide/openbaocluster/getting-started',
    },
    {
      label: 'Prepare for day 2',
      description: 'Move immediately into backups, access, upgrades, and production hardening.',
      docId: 'user-guide/openbaocluster/next-steps',
    },
  ]}
/>

<DecisionTable
  title="Pick the first-cluster intent"
  columns={['Intent', 'Start with', 'Do not skip', 'Go deeper']}
  rows={[
    {
      cells: [
        'Local evaluation',
        'Development profile with operator-managed TLS and minimal storage choices.',
        'Treat it as disposable. Do not carry this profile into production.',
        'Security profiles',
      ],
      emphasis: 'recommended',
    },
    {
      cells: [
        'Hardened production baseline',
        'Hardened profile, self-init, External or ACME TLS, and explicit storage.',
        'User access bootstrap, unseal configuration, and backups before the first risky upgrade.',
        'Validated deployments',
      ],
    },
    {
      cells: [
        'Dedicated team namespace',
        'The hardened baseline plus the single-tenant operator install path.',
        'Namespace ownership, rendered controller identity, and `WATCH_NAMESPACE` alignment.',
        'Single-tenant mode',
      ],
    },
  ]}
/>

## Start with the closest manifest

<Tabs groupId="first-cluster-intent">

<TabItem value="evaluation" label="Local evaluation">

<CommandBlock
  language="yaml"
  label="configure"
  title="Start a development-profile cluster for local evaluation"
  code={`apiVersion: openbao.org/v1alpha1
kind: OpenBaoCluster
metadata:
  name: dev-cluster
  namespace: openbao-demo
spec:
  version: "2.5.0"
  replicas: 3
  profile: Development
  tls:
    enabled: true
    mode: OperatorManaged
    rotationPeriod: "720h"
  storage:
    size: "10Gi"`}
/>

<Callout type="note" title="Namespace choice still follows tenancy mode">

If you are on the default multi-tenant path, `openbao-demo` must already be onboarded through `OpenBaoTenant`.
If you are on the single-tenant path, replace `openbao-demo` with the namespace watched by the controller.

</Callout>

<Callout type="warning" title="Evaluation only">

The `Development` profile stores sensitive material in Kubernetes Secrets and relaxes production controls.
Use it for local testing and CI, not for real environments.

</Callout>

</TabItem>

<TabItem value="production" label="Hardened baseline">

<CommandBlock
  language="yaml"
  label="configure"
  title="Use a hardened baseline as the starting production shape"
  code={`apiVersion: openbao.org/v1alpha1
kind: OpenBaoCluster
metadata:
  name: prod-cluster
  namespace: openbao-prod
spec:
  version: "2.5.0"
  replicas: 3
  profile: Hardened
  tls:
    enabled: true
    mode: External
  storage:
    size: "50Gi"
    storageClassName: "fast-ssd"
  selfInit:
    enabled: true
    oidc:
      enabled: true
    requests:
      # add at least one human login path before first exposure
      # for example: userpass, JWT, or Kubernetes auth
  unseal:
    # configure cloud or transit auto-unseal before first reconcile`}
/>

<Callout type="warning" title="Do not apply the hardened example unchanged">

Complete these before the first production reconcile:

1. finish the full `selfInit` contract so it includes both `oidc.enabled: true` for operator lifecycle auth and at least one human login path in `selfInit.requests`, using [Self-Initialization](../openbaocluster/configuration/self-init.md) and [Operator Authentication](../operator/authn.md)
2. finish `unseal` with an external trust path such as cloud KMS, transit, KMIP, OCI KMS, or PKCS#11 in [Unseal Configuration](../openbaocluster/configuration/unseal.md)
3. finish the namespace handoff for your tenancy mode so `openbao-prod` is already onboarded in multi-tenant mode or is the watched namespace in single-tenant mode

</Callout>

<Callout type="danger" title="Do not treat human auth as a post-bootstrap step">

`spec.selfInit.oidc.enabled: true` gives the operator a JWT-based control path. It does not create a human login path.
If the cluster will self-initialize, include at least one human auth method in `selfInit.requests` before the first reconcile so the cluster is usable after the root token is revoked.

</Callout>

<Callout type="tip" title="Use a validated baseline when possible">

If you are going straight to production, prefer a tested architecture or recipe under [Validated Deployments](../validated-deployments/index.mdx) rather than inventing the entire first manifest from scratch.

</Callout>

</TabItem>

</Tabs>

## Apply and verify

<CommandBlock
  language="bash"
  label="apply"
  title="Apply the cluster manifest"
  code={`kubectl apply -f cluster.yaml`}
/>

<CommandBlock
  language="bash"
  label="inspect"
  title="Inspect cluster phase and readiness"
  code={`kubectl get openbaocluster <name> -n <namespace> -o wide`}
>
  Watch `status.phase`, `readyReplicas`, and whether the cluster reaches `Available=True`.
</CommandBlock>

<CommandBlock
  language="bash"
  label="verify"
  title="Watch the cluster pods stabilize"
  code={`kubectl get pods -l openbao.org/cluster=<name> -n <namespace> -w`}
>
  A healthy first cluster should converge without repeated crash loops or long-lived pending state.
</CommandBlock>

<Callout type="note" title="What to look for next">

Confirm the cluster is available, TLS and storage match the shape you intended, and hardened clusters can realistically progress toward `ProductionReady=True`.

</Callout>

<NextActions
  title="Once the first cluster is healthy"
  items={[
    {
      label: 'Prepare for day 2',
      description: 'Choose the next operating concern instead of leaving the cluster in a half-configured state.',
      docId: 'user-guide/openbaocluster/next-steps',
    },
    {
      label: 'Expose the cluster',
      description: 'Pick the access path and TLS posture that match the security profile you chose.',
      docId: 'user-guide/openbaocluster/configuration/external-access',
    },
    {
      label: 'Configure backups',
      description: 'Wire snapshots before the first risky change so restore is not first attempted during an incident.',
      docId: 'user-guide/openbaocluster/operations/backups',
    },
  ]}
/>

## Official OpenBao background

- [OpenBao self-initialization](https://openbao.org/docs/configuration/self-init/)
