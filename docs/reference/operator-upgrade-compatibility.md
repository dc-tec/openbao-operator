---
title: Upgrade Compatibility
description: Operator upgrade compatibility policy, supported upgrade paths, rollback stance, and required CRD upgrade sequence.
pageType: reference
journey: reference
---

<PageHeader
  title="Operator upgrade compatibility"
  lede="Supported operator upgrade paths, required CRD sequencing, and the project stance on downgrade and rollback."
/>

<DecisionTable
  kind="reference"
  title="Supported upgrade paths"
  columns={['Path', 'Project stance', 'Operator guidance']}
  rows={[
    {
      cells: ['Stable patch upgrades (`0.Y.Z -> 0.Y.(Z+1)`)', 'Supported', 'Use as the normal maintenance path.'],
      emphasis: 'recommended',
    },
    {
      cells: ['Stable minor upgrades (`0.Y.Z -> 0.(Y+1).0`)', 'Supported with release note review', 'Upgrade sequentially across minors and validate in staging.'],
    },
    {
      cells: ['Skipping multiple minors', 'Not recommended', 'Move sequentially so migrations and deprecations are not compressed into one jump.'],
      emphasis: 'caution',
    },
    {
      cells: ['Operator downgrades as routine rollback', 'Not supported', 'Plan downgrades only as recovery workflows with staging validation.'],
      emphasis: 'caution',
    },
  ]}
/>

<Callout type="warning" title="CRD-first upgrade rule">

Apply CRDs before upgrading the Helm release when the target release changed CRD content.

</Callout>

<CommandBlock
  language="bash"
  label="apply"
  title="CRD-first upgrade sequence"
  code={`kubectl apply -f https://github.com/dc-tec/openbao-operator/releases/download/X.Y.Z/crds.yaml
helm upgrade openbao-operator oci://ghcr.io/dc-tec/charts/openbao-operator \
  --namespace <operator-namespace>`}
>
  This matches the installation guidance and keeps the API surface aligned with the controller you are about to run.
</CommandBlock>

## Upgrade safety checklist

Before upgrade:

- confirm the target version in [Compatibility Matrix](compatibility.md)
- take and verify backups for managed clusters
- review release notes for deprecations and migrations

After upgrade:

- verify operator Deployments are `Running`
- verify CRD version and controller readiness
- verify managed cluster conditions and recent events

## Rollback stance

If an upgrade introduces issues:

1. prefer a forward fix on a newer stable release
2. if rollback is required, validate it as a recovery workflow in staging first
3. use backup and restore runbooks for data-path recovery scenarios

<NextActions
  title="Related upgrade references"
  items={[
    {
      label: 'Backup operations',
      description: 'Open the backup workflow before upgrades when you still need to capture or verify the recovery point.',
      to: '/docs/operate/backups',
    },
    {
      label: 'Restore from backup',
      description: 'Restore procedures for cases where rollback is no longer sufficient.',
      docId: 'user-guide/openbaorestore/restore',
    },
    {
      label: 'Status conditions and events',
      description: 'Readiness and failure signals before and after an upgrade.',
      docId: 'reference/status-and-events',
    },
  ]}
/>
