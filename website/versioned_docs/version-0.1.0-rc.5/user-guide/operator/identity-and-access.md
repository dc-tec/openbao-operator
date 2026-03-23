---
title: Operator Identity And Access
description: Map controller, workload, backup, restore, and upgrade identities so rendered installs and OpenBao-side bindings stay aligned.
slug: /get-started/operator-identity
hide_title: true
pageType: concept
journey: get-started
---

<PageHero
  variant="compact"
  eyebrow="Supporting decision"
  title="Keep the operator identity surfaces separate in your head."
  lede="The controller, workload pods, and day 2 executor jobs do not share one identity. This page helps you trace which Kubernetes ServiceAccount maps to which OpenBao auth and authorization surface so custom installs do not drift."
  actions={[
    {label: 'Review operator authentication', docId: 'user-guide/operator/authn', variant: 'primary'},
    {label: 'Return to installation', docId: 'user-guide/operator/installation', variant: 'secondary'},
  ]}
>
  <Checklist
    title="Use this page when you need to"
    items={[
      'customize names, namespaces, or raw-manifest overlays',
      'explain why controller auth works but backup or restore auth does not',
      'separate Kubernetes RBAC from OpenBao-side role binding in your mental model',
      'verify which ServiceAccount a given day 2 job actually runs as',
    ]}
  />
</PageHero>

<DecisionTable
  kind="reference"
  title="Identity map"
  columns={['Actor', 'Kubernetes identity', 'OpenBao auth', 'Primary boundary']}
  rows={[
    {
      cells: ['Provisioner', 'Provisioner ServiceAccount in the operator namespace', 'None', 'Kubernetes RBAC only'],
      emphasis: 'recommended',
    },
    {
      cells: ['Controller', 'Controller ServiceAccount in the operator namespace', 'Projected JWT token bound to the `openbao-operator` role', 'Kubernetes RBAC plus OpenBao maintenance policy'],
    },
    {
      cells: ['Main OpenBao Pods', 'Per-cluster ServiceAccount in the tenant namespace', 'OpenBao server runtime auth and configured seal/unseal integration', 'Kubernetes workload identity plus OpenBao runtime configuration'],
    },
    {
      cells: ['Backup Job', 'Generated backup ServiceAccount in the tenant namespace', 'Projected JWT token or explicit backup token Secret', 'Snapshot policy plus backup-target credentials'],
    },
    {
      cells: ['Restore Job', 'Generated restore ServiceAccount in the tenant namespace', 'Projected JWT token or explicit restore token Secret', 'Restore policy plus restore-source credentials'],
    },
    {
      cells: ['Upgrade Job', 'Generated upgrade ServiceAccount in the tenant namespace', 'Projected JWT token', 'Upgrade policy for rolling or blue-green operations'],
    },
  ]}
/>

<DecisionTable
  kind="reference"
  title="Install-sensitive checks"
  columns={['Surface', 'What must match', 'Why it breaks when it drifts']}
  rows={[
    {
      cells: ['Controller identity', 'Rendered controller ServiceAccount name and operator namespace', 'The JWT role binding and admission-policy subjects stop pointing at the real controller.'],
      emphasis: 'recommended',
    },
    {
      cells: ['Projected token mount', 'The controller Deployment still mounts the `openbao-token` projected volume', 'The controller loses its default JWT auth path to OpenBao.'],
    },
    {
      cells: ['JWT audience', '`OPENBAO_JWT_AUDIENCE`, the projected token audience, and the OpenBao role `bound_audiences`', 'A valid controller identity still fails auth when the audience contract drifts.'],
    },
    {
      cells: ['Executor identities', 'Backup, restore, and upgrade Jobs use their own generated ServiceAccounts', 'Main workload identity does not automatically carry into day 2 executor jobs.'],
    },
  ]}
/>

<DecisionTable
  kind="reference"
  title="Common failure modes"
  columns={['Symptom', 'Most likely boundary', 'Check first']}
  rows={[
    {
      cells: ['`permission denied` when the controller talks to OpenBao', 'Controller JWT auth or OpenBao role binding', 'Operator authentication'],
      emphasis: 'recommended',
    },
    {
      cells: ['Custom raw-manifest install fails after namespace or prefix changes', 'Rendered identity drift', 'Operator installation render verification'],
    },
    {
      cells: ['Backup or restore auth fails while the main cluster stays healthy', 'Executor Job identity drift', 'Operator authorization plus backup or restore configuration'],
    },
    {
      cells: ['Tenant onboarding works, but controller access does not', 'Kubernetes RBAC or RoleBinding introduction', 'RBAC architecture'],
    },
  ]}
/>

<NextActions
  title="Go deeper"
  items={[
    {
      label: 'Operator authentication',
      description: 'See how the projected JWT token, audience, and role binding form the default auth path.',
      docId: 'user-guide/operator/authn',
    },
    {
      label: 'Operator authorization',
      description: 'Review which policies belong to controller, backup, restore, and upgrade work.',
      docId: 'user-guide/operator/authz',
    },
    {
      label: 'RBAC architecture',
      description: 'Move into Kubernetes permission boundaries when the problem is namespace or RoleBinding scoped.',
      docId: 'security/infrastructure/rbac',
    },
  ]}
/>
