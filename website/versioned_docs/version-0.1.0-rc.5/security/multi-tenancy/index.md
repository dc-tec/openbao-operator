---
title: Tenant Isolation
hide_title: true
pageType: landing
journey: security
description: Multi-tenancy security model for OpenBao Operator, describing tenant isolation, namespace boundaries, and least-privilege access control.
---

<PageHero
  eyebrow="Security / Tenant Isolation"
  title="Treat multi-tenancy as an explicit isolation model, not a convenience feature."
  lede="OpenBao Operator is designed for a shared platform with strict tenant boundaries. The security model depends on explicit namespace introduction, split controller identities, admission guardrails, and network isolation rather than on broad cluster-scoped trust."
  actions={[
    {label: 'Open the isolation model', docId: 'security/multi-tenancy/tenant-isolation', variant: 'primary'},
    {label: 'Review tenant onboarding', docId: 'user-guide/openbaotenant/overview', variant: 'secondary'},
  ]}
>
  <Checklist
    title="Use this section when you need to"
    items={[
      'understand what the multi-tenant operating model actually guarantees',
      'review the boundary between the provisioner and namespace-restricted controller',
      'connect tenant isolation to RBAC and network controls',
      'decide whether the shared-service model fits your production requirements',
    ]}
  />
</PageHero>

<DecisionTable
  title="Tenant isolation pillars"
  columns={['Pillar', 'What it protects', 'Primary mechanism']}
  rows={[
    {
      cells: [
        'Namespace introduction',
        'Prevents the controller from discovering or managing arbitrary namespaces.',
        '`OpenBaoTenant` onboarding, explicit RoleBinding introduction, and no namespace-wide controller discovery.',
      ],
      emphasis: 'recommended',
    },
    {
      cells: [
        'Identity separation',
        'Keeps provisioning and workload management from sharing a single all-powerful credential.',
        'Split provisioner and controller identities with different RBAC scopes.',
      ],
    },
    {
      cells: [
        'Admission guardrails',
        'Blocks unsafe configuration drift and unauthorized mutation of managed resources.',
        'Validating admission policies and managed-resource ownership rules.',
      ],
    },
    {
      cells: [
        'Network isolation',
        'Prevents cross-tenant traffic and over-broad egress by default.',
        'Default-deny `NetworkPolicy` plus explicit allow rules.',
      ],
    },
  ]}
/>

<NextActions
  items={[
    {
      label: 'Read the isolation model',
      description: 'Go deeper into the exact namespace, RBAC, and secret-boundary behavior.',
      docId: 'security/multi-tenancy/tenant-isolation',
    },
    {
      label: 'Open RBAC architecture',
      description: 'See how the split-controller model enforces the tenant boundary at the identity layer.',
      docId: 'security/infrastructure/rbac',
    },
    {
      label: 'Open tenancy and governance',
      description: 'Switch to the user-guide path when you need the actual onboarding workflow instead of the security model.',
      docId: 'user-guide/openbaotenant/overview',
    },
  ]}
/>
