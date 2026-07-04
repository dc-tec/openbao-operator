---
title: Security Model
hide_title: true
pageType: landing
journey: security
description: Core security fundamentals for OpenBao Operator including threat model, security profiles, and secrets management practices.
---

<PageHero
  variant="landing"
  eyebrow="Security / Security Model"
  title="Security model and trust assumptions"
  lede="Threat model, security profiles, and trust material such as root tokens, unseal keys, and bootstrap identities."
  actions={[
    {label: 'Read the threat model', docId: 'security/fundamentals/threat-model', variant: 'primary'},
    {label: 'Review production posture', docId: 'security/fundamentals/profiles', variant: 'secondary'},
  ]}
>
  <Checklist
    title="Covers"
    items={[
      'understand the security assumptions behind the operator architecture',
      'understand how Development and Hardened profiles change the supported security posture',
      'review how bootstrap and unseal trust material is handled',
      'anchor security review conversations before diving into RBAC or network policy details',
    ]}
  />
</PageHero>

<RouteList
  title="Security model routes"
  items={[
    {
      eyebrow: '01',
      title: 'Threat model',
      description: 'Trust boundaries, attacker assumptions, and design mitigations.',
      docId: 'security/fundamentals/threat-model',
    },
    {
      eyebrow: '02',
      title: 'Production posture',
      description: 'How Development and Hardened differ, and the supported production contract.',
      docId: 'security/fundamentals/profiles',
    },
    {
      eyebrow: '03',
      title: 'Secrets and trust material',
      description: 'Review how root tokens, unseal keys, and bootstrap credentials are created, stored, or intentionally avoided.',
      docId: 'security/fundamentals/secrets-management',
    },
  ]}
/>

<NextActions
  items={[
    {
      label: 'Open platform controls',
      description: 'Kubernetes controls that enforce the trust model.',
      docId: 'security/infrastructure/index',
    },
    {
      label: 'Configure security profiles',
      description: 'Task page for setting `spec.profile` on a cluster.',
      docId: 'user-guide/openbaocluster/configuration/security-profiles',
    },
  ]}
/>
