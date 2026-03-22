---
title: Prepare for Day 2
slug: /get-started/next-steps
hide_title: true
description: Move from a first healthy OpenBaoCluster into production hardening, access, backups, upgrades, and tenant onboarding.
---

<JourneyHero
  eyebrow="Step 4"
  title="Turn a working cluster into an operable service."
  lede="A first cluster is only the start. The next job is to harden the environment, expose it safely, wire backups before you need them, and choose which documentation path owns the rest of the rollout."
  actions={[
    {label: 'Open the production checklist', docId: 'user-guide/openbaocluster/operations/production-checklist', variant: 'primary'},
    {label: 'Browse validated deployments', docId: 'user-guide/validated-deployments/index', variant: 'secondary'},
  ]}
>
  <Checklist
    title="Good day 2 follow-through usually means"
    items={[
      'security profile, TLS mode, and access path are aligned',
      'backup and restore work is started before the first risky change',
      'upgrade strategy is understood before the first release bump',
      'tenant onboarding and observability are treated as part of the service, not extras',
    ]}
  />
</JourneyHero>

<JourneySteps
  title="The first journey ends when operations can continue without guesswork"
  current={4}
  items={[
    {
      label: 'Choose a deployment path',
      description: 'Decide tenancy mode, security profile, TLS posture, and install method.',
      docId: 'user-guide/operator/deployment-decision-guide',
    },
    {
      label: 'Install the operator',
      description: 'Use Helm or manifests with the right namespace, identity, and admission model.',
      docId: 'user-guide/operator/installation',
    },
    {
      label: 'Create your first cluster',
      description: 'Apply a starting profile that matches local evaluation or hardened production.',
      docId: 'user-guide/openbaocluster/getting-started',
    },
    {
      label: 'Prepare for day 2',
      description: 'Move into production checklist items, backups, exposure, and observability.',
      docId: 'user-guide/openbaocluster/next-steps',
    },
  ]}
/>

## Pick the next domain on purpose

<CardGrid>
  <LinkCard eyebrow="Hardening" title="Finish the production checklist" docId="user-guide/openbaocluster/operations/production-checklist">
    Use the checklist to close the gap between a successful bootstrap and an environment you can responsibly expose.
  </LinkCard>
  <LinkCard eyebrow="Access" title="Expose the cluster safely" docId="user-guide/openbaocluster/configuration/external-access">
    Choose Gateway API, Ingress, or service exposure with the TLS posture your cluster profile actually supports.
  </LinkCard>
  <LinkCard eyebrow="Recovery" title="Wire backups before upgrades" docId="user-guide/openbaocluster/operations/backups">
    Configure snapshots and storage early so restore is practiced before an incident or a failed rollout.
  </LinkCard>
  <LinkCard eyebrow="Lifecycle" title="Plan upgrades" docId="user-guide/openbaocluster/operations/upgrades">
    Decide whether RollingUpdate is enough or whether you need blue/green validation and cutover control.
  </LinkCard>
  <LinkCard eyebrow="Platform model" title="Onboard tenant namespaces" docId="user-guide/openbaotenant/overview">
    Use OpenBaoTenant when the platform team owns the operator and teams consume OpenBao as a service.
  </LinkCard>
  <LinkCard eyebrow="Security" title="Review profile and access assumptions" docId="user-guide/openbaocluster/configuration/security-profiles">
    Re-check profile, auth, and policy assumptions before you standardize on this cluster shape.
  </LinkCard>
</CardGrid>

<OutcomePanel
  title="From here, the docs split into focused operating paths."
  tone="success"
  actions={[
    {label: 'Open Operate', docId: 'user-guide/openbaocluster/operations/upgrades'},
    {label: 'Open Security', docId: 'security/index'},
    {label: 'Open Architecture', docId: 'architecture/index'},
  ]}
>
  <p>Use the rest of the site based on the question you are actually answering:</p>

  - go to <strong>Configure</strong> when you are shaping the cluster spec and exposure model
  - go to <strong>Operate</strong> when you are planning upgrades, backups, maintenance, or troubleshooting
  - go to <strong>Recover</strong> when you are handling a failed rollout, sealed cluster, or restore event
  - go to <strong>Security</strong> when you are validating the trust boundaries and guardrails behind the platform
</OutcomePanel>
