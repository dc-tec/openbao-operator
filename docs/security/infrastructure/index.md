---
title: Platform Controls
hide_title: true
pageType: landing
journey: security
description: Infrastructure security controls in OpenBao Operator, including RBAC architecture, validating admission policies, and network security boundaries.
---

<PageHero
  variant="landing"
  eyebrow="Security / Platform Controls"
  title="Platform security controls"
  lede="This section covers Kubernetes-level controls such as RBAC, validating admission policies, and network boundaries. Use it to review the protections around operator identities, unsafe object rejection, and traffic restrictions."
  actions={[
    {label: 'Open RBAC architecture', docId: 'security/infrastructure/rbac', variant: 'primary'},
    {label: 'Review admission policies', docId: 'security/infrastructure/admission-policies', variant: 'secondary'},
  ]}
>
  <Checklist
    title="Use this section when you need to"
    items={[
      'review who can do what in the control plane',
      'understand which unsafe changes are rejected before persistence',
      'verify tenant and egress network boundaries',
      'check which cluster prerequisites are required for the security model to hold',
    ]}
  />
</PageHero>

<RouteList
  title="Platform control routes"
  items={[
    {
      eyebrow: '01',
      title: 'RBAC architecture',
      description: 'Understand the split-controller model, narrow identities, and mutation-locked access boundaries.',
      docId: 'security/infrastructure/rbac',
    },
    {
      eyebrow: '02',
      title: 'Admission policies',
      description: 'See how CEL-based guardrails reject unsafe configurations and pause sensitive reconciliation when enforcement disappears.',
      docId: 'security/infrastructure/admission-policies',
    },
    {
      eyebrow: '03',
      title: 'Network security',
      description: 'Review default-deny traffic boundaries and the explicit egress model used for backups, upgrades, and integrations.',
      docId: 'security/infrastructure/network-security',
    },
  ]}
/>

<Callout type="note" title="Cluster prerequisites">

- Kubernetes `v1.33+` is required by OpenBao Operator. `ValidatingAdmissionPolicy` is GA on all supported versions.
- A CNI that actually enforces `NetworkPolicy` is required for the network isolation model to be real.

</Callout>

<NextActions
  items={[
    {
      label: 'Open workload protections',
      description: 'Move from platform enforcement into pod hardening, TLS, and supply-chain controls.',
      docId: 'security/workload/index',
    },
    {
      label: 'Open tenant isolation',
      description: 'See how these platform controls support the multi-tenant security model.',
      docId: 'security/multi-tenancy/index',
    },
  ]}
/>
