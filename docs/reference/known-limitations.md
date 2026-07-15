---
title: Known Limitations
description: Known limitations and explicit non-goals for OpenBao Operator 0.x release lines.
pageType: reference
journey: reference
---

<PageHeader
  title="Known limitations and non-goals"
  lede="Current constraints and explicit non-goals for the pre-GA line."
/>

<DecisionTable
  kind="reference"
  title="Current constraints"
  columns={['Area', 'Current limitation', 'What to do instead']}
  rows={[
    {
      cells: ['CRD versioning', 'The current served and storage API is `openbao.org/v1alpha1`; multi-version conversion webhooks are out of scope today.', 'Treat API evolution through the pre-GA contract and review release notes carefully.'],
    },
    {
      cells: ['Cluster adoption', 'The operator assumes it manages clusters it created and reconciles; generic import of arbitrary unmanaged OpenBao clusters is out of scope.', 'Create operator-managed clusters directly, or use backup and restore workflows when you need to move data into a new operator-managed cluster.'],
    },
    {
      cells: ['Operator downgrade', 'Routine downgrades are unsupported.', 'Use the recovery and restore guidance when a release cannot move forward safely.'],
      emphasis: 'caution',
    },
    {
      cells: ['External backup cleanup', '`DeleteAll` removes PVC-backed data but does not delete snapshot objects already written to external object storage.', 'Clean external backup objects explicitly as part of decommission procedures.'],
    },
    {
      cells: ['etcd encryption verification', 'The operator cannot directly prove cluster-level etcd encryption at rest and surfaces a warning condition instead.', 'Validate cluster-level encryption controls outside the operator.'],
    },
    {
      cells: ['Helm CRD lifecycle', 'Helm does not automatically upgrade or delete CRDs.', 'Use release `crds.yaml` assets for CRD lifecycle operations.'],
    },
    {
      cells: ['Built-in upgrade authentication', 'Built-in rolling and blue/green upgrade orchestration do not support `spec.upgrade.tokenSecretRef`; upgrade Jobs use JWT authentication only.', 'Configure `spec.upgrade.jwtAuthRole`, or use the default role created during initial `selfInit.oidc` bootstrap.'],
    },
    {
      cells: ['Audit file storage archival', '`spec.auditFileStorage` provides a PVC-backed collector handoff and replay buffer; it does not provide rotation, pruning, tamper-proof retention, or a collector.', 'Mount the audit PVC read-only into a collector and ship records to external retention-controlled storage.'],
    },
    {
      cells: ['Upgrade strategy switching', 'Switching an existing cluster between `RollingUpdate` and `BlueGreen` is not a supported in-place transition today.', 'Choose the strategy before the next rollout and keep it stable. Safe idle-only switching is tracked in [#548](https://github.com/dc-tec/openbao-operator/issues/548).'],
      emphasis: 'caution',
    },
    {
      cells: ['OpenBao 2.6.0 BlueGreen upgrade', 'OpenBao 2.6.0 cannot exchange Raft Autopilot health with pre-2.6 peers because its internal request-forwarding gRPC service name changed. The operator blocks pre-2.6 to 2.6-or-newer BlueGreen transitions before deploying Green until a compatible target is explicitly qualified.', 'Use a rolling upgrade on clusters already configured for RollingUpdate. Existing BlueGreen clusters must wait for a qualified upstream fix or restore a backup into a new target-version cluster.'],
      emphasis: 'caution',
    },
  ]}
/>

<NextActions
  title="Related caveat and recovery pages"
  items={[
    {
      label: 'Support policy',
      description: 'Maintenance contract and release lines that remain in scope.',
      docId: 'reference/support-policy',
    },
    {
      label: 'Upgrade compatibility',
      description: 'Rollback stance and CRD sequencing for operator upgrades.',
      docId: 'reference/operator-upgrade-compatibility',
    },
    {
      label: 'Decommission a cluster',
      description: 'Operational decommissioning workflow for external-backup caveats.',
      docId: 'user-guide/openbaocluster/operations/deletion',
    },
    {
      label: 'Restore from backup',
      description: 'Recovery or migration workflow when an in-place change is not available.',
      docId: 'user-guide/openbaorestore/restore',
    },
  ]}
/>
