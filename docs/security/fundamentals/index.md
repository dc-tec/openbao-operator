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
  lede="This section covers the threat model, security profiles, and handling of trust material such as root tokens, unseal keys, and bootstrap identities. It is the starting point for understanding the operator's security assumptions."
  actions={[
    {label: 'Read the threat model', docId: 'security/fundamentals/threat-model', variant: 'primary'},
    {label: 'Review production posture', docId: 'security/fundamentals/profiles', variant: 'secondary'},
  ]}
>
  <Checklist
    title="Use this section when you need to"
    items={[
      'understand the security assumptions behind the operator architecture',
      'decide what Hardened actually means before you deploy it',
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
      description: 'Read the trust boundaries, attacker assumptions, and design mitigations behind the operator.',
      docId: 'security/fundamentals/threat-model',
    },
    {
      eyebrow: '02',
      title: 'Production posture',
      description: 'Understand what Development and Hardened actually mean, and why Hardened is the supported production contract.',
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
      description: 'Move from the trust model into the Kubernetes controls that enforce it.',
      docId: 'security/infrastructure/index',
    },
    {
      label: 'Configure security profiles',
      description: 'Switch to the user-guide task page when you are ready to set `spec.profile` on a real cluster.',
      docId: 'user-guide/openbaocluster/configuration/security-profiles',
    },
  ]}
/>
