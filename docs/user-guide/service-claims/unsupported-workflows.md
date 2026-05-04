---
title: Unsupported claim workflows
description: Workflows that are intentionally out of scope for the current same-cluster claim-primary release.
slug: /service-claims/unsupported-workflows
hide_title: true
pageType: reference
journey: get-started
---

<PageHeader
  title="Keep unsupported claim workflows explicit"
  lede="Use this page when you need to know whether a blocked claim workflow is a bug, a supported boundary, or a feature that has not landed yet."
/>

<DecisionTable
  kind="reference"
  title="Unsupported in the current claim release"
  columns={['Workflow', 'Current status', 'Use instead for now']}
  rows={[
    {
      cells: [
        'Adopt an existing direct OpenBaoCluster into claim ownership',
        'Not supported. Adoption is a separate admin workflow that has not landed yet.',
        'Keep the workload on the direct-cluster path until a dedicated adoption workflow exists.',
      ],
      emphasis: 'recommended',
    },
    {
      cells: [
        'Migrate a claim between same-cluster and multi-cluster execution or relocate the target cluster',
        'Not supported. Migration remains a separate workflow design area.',
        'Treat the current release as same-cluster claim provisioning only.',
      ],
    },
    {
      cells: [
        'Treat direct post-materialization claim spec edits as rollout automation',
        'Not supported. Materialized service selection stays locked after binding.',
        'Use explicit request workflows for supported in-place upgrades, manual backups, and restore from the latest successful or selected completed claim backup request.',
      ],
    },
    {
      cells: [
        'Use bootstrap modes other than SelfInit',
        'Not supported. The current claim runtime is built around self-init.',
        'Keep non-SelfInit workflows on the direct-cluster path.',
      ],
    },
    {
      cells: [
        'Select an arbitrary snapshot or external source through a claim restore request',
        'Not supported. The current restore request model is bounded to the latest successful backup or a completed claim backup request for the same claim and local cluster.',
        'Use `source.mode: BackupRequest` for claim-created backups, or the direct `OpenBaoRestore` path when you need broader restore-source control.',
      ],
    },
    {
      cells: [
        'Project backup or other service shapes that do not map honestly to the direct same-cluster OpenBaoCluster API',
        'Not supported. The claim controller fails closed instead of inventing hidden runtime behavior.',
        'Use a direct OpenBaoCluster or wait until the direct workload API grows an explicit seam for that shape.',
      ],
    },
    {
      cells: [
        'Treat multi-cluster convergence as the main public claim story',
        'Not supported in this release. The bounded public surface is same-cluster claims.',
        'Keep multi-cluster work as a future extension instead of assuming it from the current claim API.',
      ],
    },
  ]}
/>

<Callout type="warning" title="Fail-closed behavior is intentional">

When a claim workflow cannot be projected honestly into the current same-cluster runtime contract, the controller blocks or fails the request instead of inventing hidden behavior. Treat that as a supported safety property, not as missing polish.

</Callout>

<NextActions
  title="Continue from the current supported scope"
  items={[
    {
      label: 'Request a service',
      description: 'Return to the supported same-cluster quickstart when the desired workflow is inside the current release scope.',
      docId: 'user-guide/service-claims/getting-started',
    },
    {
      label: 'Use direct clusters',
      description: 'Stay on the direct OpenBaoCluster path when you need full workload ownership or unsupported claim workflows.',
      docId: 'user-guide/openbaocluster/getting-started',
    },
    {
      label: 'Read service-claim architecture',
      description: 'Open the architecture page when you need the internal reasoning behind the current scope boundaries.',
      docId: 'architecture/service-claims',
    },
  ]}
/>
