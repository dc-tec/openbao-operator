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
      cells: ['Service claims', 'The current `OpenBaoClusterClaim` surface is bounded to same-cluster provisioning plus explicit in-place compatible upgrade, manual backup, and restore from the latest successful or selected completed claim backup request. Adoption, migration, arbitrary restore-source selection, non-`SelfInit` bootstrap modes, and broader multi-cluster claim convergence are out of scope.', 'Use direct `OpenBaoCluster` and `OpenBaoRestore` workflows when you need the full workload or restore surface, and wait for explicit adoption or migration workflows before moving existing direct clusters into claim ownership.'],
      emphasis: 'caution',
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
      cells: ['Upgrade strategy switching', 'Switching an existing cluster between `RollingUpdate` and `BlueGreen` is not a supported in-place transition today.', 'Choose the upgrade strategy before the next rollout and keep it stable for that cluster.'],
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
