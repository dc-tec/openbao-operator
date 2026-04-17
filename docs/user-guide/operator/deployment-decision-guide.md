---
title: Deployment Decision Guide
description: Choose the tenancy model, security posture, bootstrap flow, and install path for a new deployment.
slug: /get-started/deployment-decision-guide
hide_title: true
pageType: task
journey: get-started
journeyStep: 1
---

<PageHeader
  title="Choose the deployment path"
  lede="Choose the tenancy model, security posture, bootstrap flow, and install path for a new deployment. The default path is multi-tenant, Hardened, self-init, and the standard install flow."
/>

<Checklist
    title="Default starting point"
    items={[
      'multi-tenant mode',
      'Hardened profile',
      'self-init enabled',
      'External or ACME TLS',
      'admission policies enabled',
      'RollingUpdate until you need blue-green cutover control',
    ]}
    tone="success"
  />


<JourneyRail
  title="The first five moves"
  current={1}
  items={[
    {
      label: 'Choose a deployment model',
      description: 'Set tenancy, security posture, install method, and any planned exceptions.',
      docId: 'user-guide/operator/deployment-decision-guide',
    },
    {
      label: 'Install the operator',
      description: 'Render the right namespace, identity, and admission posture.',
      docId: 'user-guide/operator/installation',
    },
    {
      label: 'Onboard the target namespace',
      description: 'In the default multi-tenant path, use OpenBaoTenant, then create the first cluster.',
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
  title="Default deployment decisions and exceptions"
  columns={['Decision area', 'Default', 'Use an alternative when', 'Go deeper']}
  rows={[
    {
      cells: [
        'Tenancy model',
        'Multi-tenant',
        'One team directly owns one namespace and does not need the default tenant-onboarding model.',
        'Single-tenant mode',
      ],
      emphasis: 'recommended',
    },
    {
      cells: [
        'Security profile',
        'Hardened',
        'This environment is strictly local development, CI, or short-lived evaluation.',
        'Security profiles',
      ],
    },
    {
      cells: [
        'Bootstrap flow',
        'Self-init',
        'You are intentionally carrying a compatibility or controlled manual-bootstrap workflow.',
        'Self-initialization',
      ],
    },
    {
      cells: [
        'TLS mode',
        'External or ACME',
        'You are in a non-Hardened environment and temporary operator-managed TLS convenience matters more than production trust requirements.',
        'TLS and identity',
      ],
    },
    {
      cells: [
        'Installation path',
        'Helm',
        'You need raw-manifest overlays, source-based rendering, or install-time identity customization.',
        'Operator installation',
      ],
    },
    {
      cells: [
        'Upgrade strategy',
        'RollingUpdate',
        'You need parallel validation, manual promotion, or stronger cutover control than rolling upgrades provide.',
        'Cluster upgrades',
      ],
    },
  ]}
/>

<Callout type="warning" title="Operator auth is not human auth">

`spec.selfInit.oidc.enabled: true` bootstraps operator authentication only.
Decide which human login path will be created as part of `spec.selfInit.requests` during bootstrap, then finalize the cluster design.

</Callout>

<RouteList
  title="Exceptions that change the default path"
  items={[
    {
      eyebrow: 'A',
      title: 'Single-tenant mode',
      description: 'Use this when one team owns one namespace and wants direct namespace-scoped operator control.',
      docId: 'user-guide/operator/single-tenant-mode',
      actionLabel: 'Review',
    },
    {
      eyebrow: 'B',
      title: 'Operator identity and access',
      description: 'Use this when you customize names, namespaces, JWT audience, or raw-manifest identity wiring.',
      docId: 'user-guide/operator/identity-and-access',
      actionLabel: 'Review',
    },
    {
      eyebrow: 'C',
      title: 'Validated deployments',
      description: 'Use a tested architecture or recipe when you want a validated starting point instead of building the path from scratch.',
      docId: 'user-guide/validated-deployments/index',
      actionLabel: 'Open',
    },
  ]}
/>

<Checklist
  title="Check these decisions"
  items={[
    'Am I running multi-tenant or single-tenant mode?',
    'Is this environment Hardened or Development?',
    'If I stay multi-tenant, who creates the first OpenBaoTenant and in which namespace?',
    'How will humans authenticate after the first cluster comes up?',
    'Does Helm or raw manifests own the rendered operator identity?',
    'What is my backup plan before the first production upgrade?',
  ]}
/>

<NextActions
  title="Continue the guided path"
  items={[
    {
      label: 'Install the operator',
      description: 'Use the supported install flow and verify the rendered controller identity.',
      docId: 'user-guide/operator/installation',
    },
    {
      label: 'Onboard the target namespace',
      description: 'In the default multi-tenant path, introduce the namespace through OpenBaoTenant, then create the first cluster.',
      docId: 'user-guide/openbaotenant/onboarding',
    },
  ]}
/>
