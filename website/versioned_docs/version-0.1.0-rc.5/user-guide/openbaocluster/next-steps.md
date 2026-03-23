---
title: Prepare for Day 2
slug: /get-started/next-steps
hide_title: true
description: Choose the next operating concern after the first cluster is healthy so backups, access, and hardening are not left as future cleanup.
pageType: landing
journey: get-started
journeyStep: 5
---

<PageHero
  variant="landing"
  eyebrow="Step 5"
  title="Choose the next operating concern before you walk away."
  lede="A working cluster is not the end of setup. The next move should be deliberate: harden it, expose it, wire backups, or move into the operating guides that match the job in front of you."
  actions={[
    {label: 'Open the production checklist', docId: 'user-guide/openbaocluster/operations/production-checklist', variant: 'primary'},
    {label: 'Open Operate', docId: 'user-guide/openbaocluster/operations/index', variant: 'secondary'},
  ]}
>
  <Checklist
    title="Leave Get Started only when"
    items={[
      'the access path and TLS posture are chosen',
      'backup and restore work has an owner',
      'the first upgrade will not be improvised',
      'someone knows which docs section owns the next change',
    ]}
  />
</PageHero>

<JourneyRail
  title="The first five moves"
  current={5}
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
      description: 'Choose the next operating concern instead of leaving the cluster in a half-configured state.',
      docId: 'user-guide/openbaocluster/next-steps',
    },
  ]}
/>

<RouteList
  title="Pick the next operating concern"
  items={[
    {
      eyebrow: '01',
      title: 'Finish the production checklist',
      description: 'Close the gap between a working cluster and an environment you can responsibly expose and support.',
      docId: 'user-guide/openbaocluster/operations/production-checklist',
    },
    {
      eyebrow: '02',
      title: 'Expose the cluster safely',
      description: 'Choose Gateway API, Ingress, or service exposure with the TLS posture your profile actually supports.',
      docId: 'user-guide/openbaocluster/configuration/external-access',
    },
    {
      eyebrow: '03',
      title: 'Configure backups and restore readiness',
      description: 'Wire snapshots before the first risky change so restore is practiced before an incident or failed rollout.',
      docId: 'user-guide/openbaocluster/operations/backups',
    },
    {
      eyebrow: '04',
      title: 'Plan upgrades and routine operations',
      description: 'Choose the upgrade strategy, then move into maintenance, troubleshooting, and operating runbooks.',
      docId: 'user-guide/openbaocluster/operations/upgrades',
    },
    {
      eyebrow: '05',
      title: 'Onboard tenant namespaces',
      description: 'Use OpenBaoTenant when the platform team owns the operator and teams consume OpenBao as a service.',
      docId: 'user-guide/openbaotenant/overview',
    },
  ]}
/>

<NextActions
  title="When you need deeper context"
  items={[
    {
      label: 'Browse validated deployments',
      description: 'Use tested architectures, recipes, and runbooks when you want a known-good implementation path.',
      docId: 'user-guide/validated-deployments/index',
    },
    {
      label: 'Open Security',
      description: 'Review trust boundaries, guardrails, and profile assumptions before you standardize on this cluster shape.',
      docId: 'security/index',
    },
    {
      label: 'Open Architecture',
      description: 'Use the internal model when you need to understand why the operator behaves the way it does.',
      docId: 'architecture/index',
    },
  ]}
/>
