---
title: Operator Invariants
description: Cross-cutting invariants preserved by OpenBao Operator across identity boundaries, production posture, configuration ownership, and lifecycle safety.
hide_title: true
pageType: concept
journey: architecture
---

<PageHeader
  title="Operator invariants"
  lede="Cross-cutting contracts behind the rest of the architecture. They explain why some designs are split, which shortcuts are blocked, and which safety properties must survive refactors, new features, and operational changes."
/>



<Callout type="note" title="Lifecycle contract">

`OpenBaoCluster` is an operator-owned lifecycle contract. It is not a generic import API for arbitrary unmanaged OpenBao clusters.

</Callout>

<DecisionTable
  title="Invariant families"
  columns={['Family', 'What the operator is trying to preserve', 'Why it matters']}
  rows={[
    {
      cells: [
        'Identity boundaries',
        'Provisioning, control-plane, and workload trust boundaries stay explicit and mutation-locked.',
        'This keeps privileged access narrow and prevents tenant onboarding or operator RBAC from drifting into normal reconcile paths.',
      ],
      emphasis: 'recommended',
    },
    {
      cells: [
        'Production posture',
        'Hardened production means self-init, trusted TLS, and a non-static unseal path.',
        'Production safety is part of the supported operating contract.',
      ],
    },
    {
      cells: [
        'Integration assumptions',
        'External dependencies surface as explicit status and readiness conditions.',
        'This makes environment assumptions visible before they become runtime failures.',
      ],
    },
    {
      cells: [
        'Lifecycle safety',
        'Disruptive workflows stay explicit, lock-aware, and separated from steady-state workload reconciliation.',
        'This prevents upgrades, backups, and restores from colliding or becoming invisible side effects.',
      ],
    },
  ]}
/>

## Identity boundary invariants

<DecisionTable
  kind="reference"
  title="Identity and access"
  columns={['Invariant', 'Why it exists', 'Primary enforcement']}
  rows={[
    {
      cells: [
        'Provisioner and controller identities stay separate.',
        'A long-running workload identity should not both mint and consume tenant access.',
        'Split ServiceAccounts, RBAC boundaries, and admission policies.',
      ],
      emphasis: 'recommended',
    },
    {
      cells: [
        'Rendered operator identities stay internally consistent and mutation-locked.',
        'Install drift in ServiceAccounts, RoleBindings, and admission subjects weakens the control-plane trust model.',
        'Helm/raw-manifest rendering, managed-resource locks, and break-glass allowlists.',
      ],
    },
    {
      cells: [
        'Tenant namespace access is introduced explicitly and remains provisioner-owned.',
        'Tenant onboarding should be deliberate, not discovered passively by a broad controller identity.',
        '`OpenBaoTenant` onboarding flow, tenant governance policy, and controller RBAC exclusions.',
      ],
    },
    {
      cells: [
        'Secret access stays name-scoped and non-enumerating.',
        'The operator needs to create or read specific secrets without gaining broad tenant secret visibility.',
        'Allowlisted Secret roles, blind-create patterns, and admission restrictions on managed Secret writes.',
      ],
    },
    {
      cells: [
        'Admission enforcement remains part of the normal safety model.',
        'Guardrails should fail early at the API boundary, and loss of those guardrails should pause sensitive reconciliation.',
        'Validating admission policy inventory, runtime admission tracking, and degraded conditions.',
      ],
    },
  ]}
/>

Related reading: <SiteLink docId="security/infrastructure/rbac">RBAC Architecture</SiteLink>, <SiteLink docId="security/infrastructure/admission-policies">Admission Policies</SiteLink>, and <SiteLink docId="user-guide/operator/identity-and-access">Operator Identity and Access</SiteLink>.

## Production posture invariants

<DecisionTable
  kind="reference"
  title="Production posture"
  columns={['Invariant', 'Why it exists', 'Primary enforcement']}
  rows={[
    {
      cells: [
        'Hardened production requires self-init, trusted TLS, and a non-static unseal path.',
        'This prevents root token Secret persistence and weak bootstrap or transport defaults in production.',
        'Cluster validation, the `ProductionReady` condition, and hardened-profile checks.',
      ],
      emphasis: 'recommended',
    },
    {
      cells: [
        '`OperatorManaged` TLS is not a hardened production path.',
        'Production trust should align with externally managed or OpenBao-native certificate models.',
        'Admission validation and `ProductionReady=False` when an unsupported TLS mode is selected.',
      ],
    },
    {
      cells: [
        'Self-init is the supported production bootstrap path.',
        'Production bootstrap should stay declarative and avoid persisting the initial root token.',
        'Self-init validation, hardened profile requirements, and production posture evaluation.',
      ],
    },
    {
      cells: [
        'Operator-owned configuration stays operator-owned.',
        'Networking, storage, seal settings, and listener identity need a single ownership model to stay correct.',
        'Rendered configuration ownership and admission policy enforcement.',
      ],
    },
  ]}
/>

Related reading: <SiteLink docId="security/fundamentals/profiles">Security Profiles</SiteLink>, <SiteLink docId="security/workload/tls">TLS and Identity</SiteLink>, and <SiteLink docId="user-guide/openbaocluster/configuration/self-init">Self-Initialization</SiteLink>.

## Integration invariants

<DecisionTable
  kind="reference"
  title="External dependencies and readiness"
  columns={['Invariant', 'Why it exists', 'Primary enforcement']}
  rows={[
    {
      cells: [
        'Gateway, ACME, audit storage, and API-server assumptions surface as explicit conditions.',
        'Environment and controller dependencies should become visible status contracts before they become runtime failures.',
        '`GatewayIntegrationReady`, `ACMEIntegrationReady`, `ACMECacheReady`, `AuditFileStorageReady`, and `APIServerNetworkReady` conditions.',
      ],
      emphasis: 'recommended',
    },
    {
      cells: [
        'Audit file storage stays an explicit integration point.',
        'The operator can mount and validate the handoff PVC, but retention, tamper resistance, and collection pipelines belong to the surrounding platform.',
        '`spec.auditFileStorage`, `spec.audit`, status readiness, and workload mount guardrails.',
      ],
    },
    {
      cells: [
        'Backup and restore identity stays separate from the main workload identity.',
        'Day-2 Jobs should not silently inherit the wrong auth path, cloud permissions, or egress assumptions from the StatefulSet.',
        'Dedicated job identities, readiness evaluation, and explicit backup/restore status reasons.',
      ],
    },
  ]}
/>

Related reading: <SiteLink docId="reference/status-and-events">Status and Events</SiteLink>, <SiteLink docId="user-guide/openbaocluster/configuration/observability">Observability</SiteLink>, and <SiteLink docId="user-guide/openbaocluster/operations/backups">Configure Backups</SiteLink>.

## Lifecycle safety invariants

<DecisionTable
  kind="reference"
  title="Disruptive operation safety"
  columns={['Invariant', 'Why it exists', 'Primary enforcement']}
  rows={[
    {
      cells: [
        'Only one disruptive operation owns the cluster operation lock at a time.',
        'Upgrades, backups, and restores should not collide on the same cluster.',
        '`status.operationLock`, shared lifecycle coordination, and manager-specific lock handling.',
      ],
      emphasis: 'recommended',
    },
    {
      cells: [
        'Restore remains destructive, explicit, and lock-aware.',
        'Restore should never be mistaken for a routine reconcile side effect.',
        '`OpenBaoRestore`, restore validation, override-lock handling, and operation-lock checks.',
      ],
    },
    {
      cells: [
        'Break-glass access stays explicit and narrow.',
        'Administrative escape hatches should exist without turning privileged mutation into the normal operating model.',
        'Configured maintenance groups, admission exceptions, and recovery runbooks.',
      ],
    },
    {
      cells: [
        'OpenBao remains the source of truth for data consistency and snapshot semantics.',
        'The operator should orchestrate and guard the lifecycle, not reimplement the data plane.',
        'Supervisor pattern, API-driven backup/restore flows, and OpenBao-led snapshot semantics.',
      ],
    },
  ]}
/>

Related reading: <SiteLink docId="architecture/operation-lifecycle">Operation Lifecycle</SiteLink>, <SiteLink docId="user-guide/openbaorestore/restore">Restore from Backup</SiteLink>, and <SiteLink docId="user-guide/openbaocluster/recovery/index">Recovery and Restore</SiteLink>.

<Callout type="tip" title="Invariant changes affect the contract">

If a change weakens one of these invariants, update the related architecture, security, and user-guide pages in the same change set. It changes the product operating contract.

</Callout>

<NextActions
  title="Continue through the architecture"
  items={[
    {
      label: 'Component design',
      description: 'Where controllers, app facades, and managers split responsibilities to preserve these invariants.',
      docId: 'architecture/components',
    },
    {
      label: 'Operation lifecycle',
      description: 'Shared lifecycle coordination for upgrades, backups, and restores.',
      docId: 'architecture/operation-lifecycle',
    },
    {
      label: 'Security overview',
      description: 'User-facing security model behind these invariants.',
      docId: 'security/index',
    },
  ]}
/>
