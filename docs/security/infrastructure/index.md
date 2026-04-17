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
  lede="Kubernetes-level controls such as RBAC, validating admission policies, and network boundaries."
  actions={[
    {label: 'Open RBAC architecture', docId: 'security/infrastructure/rbac', variant: 'primary'},
    {label: 'Review admission policies', docId: 'security/infrastructure/admission-policies', variant: 'secondary'},
  ]}
>
  <Checklist
    title="Scope"
    items={[
      'control-plane access review',
      'unsafe changes rejected before persistence',
      'tenant and egress network boundaries',
      'cluster prerequisites for the security model',
    ]}
  />
</PageHero>

<RouteList
  title="Platform control routes"
  items={[
    {
      eyebrow: '01',
      title: 'RBAC architecture',
      description: 'Split-controller model, narrow identities, and mutation-locked access boundaries.',
      docId: 'security/infrastructure/rbac',
    },
    {
      eyebrow: '02',
      title: 'Admission policies',
      description: 'CEL-based guardrails that reject unsafe configurations and pause sensitive reconciliation when enforcement disappears.',
      docId: 'security/infrastructure/admission-policies',
    },
    {
      eyebrow: '03',
      title: 'Network security',
      description: 'Default-deny traffic boundaries and the explicit egress model used for backups, upgrades, and integrations.',
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
      description: 'Pod hardening, TLS, and supply-chain controls.',
      docId: 'security/workload/index',
    },
    {
      label: 'Open tenant isolation',
      description: 'How platform controls support the multi-tenant security model.',
      docId: 'security/multi-tenancy/index',
    },
  ]}
/>
