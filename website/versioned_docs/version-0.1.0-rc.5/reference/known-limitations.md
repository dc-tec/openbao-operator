---
title: Known Limitations
description: Known limitations and explicit non-goals for OpenBao Operator 0.x release lines.
pageType: reference
journey: reference
---

<PageHero
  variant="compact"
  eyebrow="Reference / Constraints & Caveats"
  title="Use this page when you need to know whether a behavior is unsupported, unfinished, or intentionally out of scope."
  lede="Not every missing feature is an accidental gap. This page captures the current constraints and deliberate non-goals for the pre-GA line so operators and contributors can separate unsupported assumptions from issues the project actually intends to solve."
  actions={[
    {label: 'Open support policy', docId: 'reference/support-policy', variant: 'primary'},
    {label: 'Open upgrade compatibility', docId: 'reference/operator-upgrade-compatibility', variant: 'secondary'},
  ]}
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
      cells: ['Operator downgrade', 'Downgrades are not treated as a normal rollback path.', 'Use the recovery and restore guidance when a release cannot move forward safely.'],
      emphasis: 'caution',
    },
    {
      cells: ['External backup cleanup', '`DeleteAll` removes PVC-backed data but does not delete external object storage backups.', 'Clean external backup objects explicitly as part of decommission procedures.'],
    },
    {
      cells: ['etcd encryption verification', 'The operator cannot directly prove cluster-level etcd encryption at rest and surfaces a warning condition instead.', 'Validate cluster-level encryption controls outside the operator.'],
    },
    {
      cells: ['Helm CRD lifecycle', 'Helm does not automatically upgrade or delete CRDs.', 'Use release `crds.yaml` assets for CRD lifecycle operations.'],
    },
    {
      cells: ['Validation channels', '`edge` and `nightly` are not production support channels.', 'Pin explicit stable versions for production environments.'],
      emphasis: 'recommended',
    },
  ]}
/>

## Support window

Support is focused on the latest stable release line. See [Support Policy](support-policy.md) for the current maintenance contract.

<NextActions
  title="Related caveat and support pages"
  items={[
    {
      label: 'Support policy',
      description: 'Open the maintenance contract behind these constraints and the release lines that remain in scope.',
      docId: 'reference/support-policy',
    },
    {
      label: 'Deprecation policy',
      description: 'Use the lifecycle policy when a limitation is really part of the pre-GA API evolution story.',
      docId: 'reference/deprecation-policy',
    },
    {
      label: 'Decommission a cluster',
      description: 'Return to the operational decommission workflow for the data-path caveats around external backups.',
      docId: 'user-guide/openbaocluster/operations/deletion',
    },
  ]}
/>
