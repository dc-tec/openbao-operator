---
title: Plan Upgrades
description: Choose the right rollout strategy, stage upgrade auth deliberately, and verify the cluster before and after a version change.
slug: /operate/upgrades
hide_title: true
pageType: task
journey: operate
---

<PageHero
  eyebrow="Operate / Upgrades"
  title="Treat version changes as planned operations, not a quick spec patch."
  lede="The operator supports rolling and blue-green upgrades, but both paths depend on cluster health, backup posture, and explicit authentication for the executor Jobs. Use this page to choose the strategy, stage the right config, and verify the rollout cleanly."
  actions={[
    {label: 'Open backup operations', docId: 'user-guide/openbaocluster/operations/backups', variant: 'primary'},
    {label: 'Open upgrade manager architecture', docId: 'architecture/upgrade-manager', variant: 'secondary'},
  ]}
>
  <Checklist
    title="Use this page when you need to"
    items={[
      'move a cluster to a newer OpenBao version',
      'decide between rolling and blue-green rollout strategies',
      'wire executor auth before the first production upgrade window',
      'recover cleanly from an upgrade hold, retry, or rollback decision',
    ]}
  />
</PageHero>

<DecisionTable
  title="Choose the upgrade strategy"
  columns={['Strategy', 'Use it when', 'Operator behavior', 'Watch for']}
  rows={[
    {
      cells: [
        'RollingUpdate',
        'You want the default production path with the lowest resource overhead.',
        'The operator upgrades pods one by one, handles leader step-down when needed, and waits for each ordinal to converge before finishing.',
        'Failures hold the rollout until you fix the cause and send an explicit retry request.',
      ],
      emphasis: 'recommended',
    },
    {
      cells: [
        'BlueGreen',
        'You need staged validation, manual promotion, or stronger cutover boundaries for a risky change.',
        'The operator creates a parallel Green revision, validates it, promotes it, and then cleans up the old Blue revision.',
        'This path uses more cluster resources and introduces additional in-flight phases to watch.',
      ],
    },
  ]}
/>

<DiagramFrame
  title="Upgrade control flow"
  caption="Every upgrade starts with validation. After that, the controller either executes a partitioned rolling rollout or creates a parallel Green revision for promotion and cleanup."
  code={`flowchart LR
    Validate["Validate target version and config"] --> Snapshot["Optional pre-upgrade snapshot"]
    Snapshot --> Strategy{"Choose strategy"}
    Strategy --> Rolling["Rolling update"]
    Strategy --> Green["Deploy green revision"]
    Rolling --> Verify["Verify health and convergence"]
    Green --> Promote["Promote green and cut over"]
    Promote --> Verify
    Verify --> Complete["Complete"]
    Verify --> Hold["Hold for retry or operator action"]

    classDef read fill:transparent,stroke:#79c0ab,stroke-width:2px,color:#e6f4ef;
    classDef process fill:transparent,stroke:#fdd0a4,stroke-width:2px,color:#e6f4ef;
    classDef write fill:transparent,stroke:#87d6be,stroke-width:2px,color:#e6f4ef;

    class Validate,Snapshot read;
    class Strategy,Rolling,Green,Promote process;
    class Verify,Complete,Hold write;`}
/>

## Before you patch `spec.version`

- Confirm the cluster is initialized, healthy, and already safe to change. An upgrade is not the time to discover a broken backup path or an unstable seal configuration.
- Set `spec.version` to the target semantic version. The operator blocks downgrades and validates semver format.
- If you override `spec.image`, keep it aligned with `spec.version`. Semantic-version tags must match, and digest-pinned images still require `spec.version` as the authoritative intent.
- Make sure the upgrade executor Job can authenticate:
  - with the default JWT role created from `selfInit.oidc`, or
  - with an explicit `spec.upgrade.jwtAuthRole`
- If you want pre-upgrade snapshots, configure `spec.backup` first and make sure the backup auth path is already working.
- In the `Hardened` profile, explicitly allow egress to object storage or other external dependencies the upgrade path needs.

<Callout type="note" title="Upgrade auth is separate from backup auth">

The upgrade executor Job uses JWT auth to talk to OpenBao. Pre-upgrade snapshots use the backup configuration and backup identity path. Do not assume one automatically covers the other.

</Callout>

## Use the default rolling path

Use `RollingUpdate` when you want the lowest operational complexity and you do not need a second revision running in parallel.

<Tabs groupId="rolling-update-setup">

<TabItem value="default-jwt-bootstrap" label="JWT via self-init OIDC">

<CommandBlock
  language="yaml"
  label="configure"
  title="Use the default rolling path with OIDC bootstrap"
  code={`spec:
  version: "2.5.0"
  selfInit:
    enabled: true
    oidc:
      enabled: true
  upgrade:
    strategy: RollingUpdate`}
/>

</TabItem>

<TabItem value="explicit-upgrade-role" label="Explicit upgrade role">

<CommandBlock
  language="yaml"
  label="configure"
  title="Use a dedicated JWT role for upgrade Jobs"
  code={`spec:
  version: "2.5.0"
  upgrade:
    strategy: RollingUpdate
    jwtAuthRole: platform-upgrade`}
/>

</TabItem>

<TabItem value="private-registry" label="Private registry image">

<CommandBlock
  language="yaml"
  label="configure"
  title="Keep the image and semantic version aligned"
  code={`spec:
  version: "2.5.0"
  image: "registry.example.com/openbao/openbao:2.5.0"
  upgrade:
    strategy: RollingUpdate
    jwtAuthRole: platform-upgrade`}
/>

</TabItem>

</Tabs>

<CommandBlock
  language="bash"
  label="apply"
  title="Patch the cluster to the target version"
  code={`kubectl patch openbaocluster <name> -n <namespace> --type merge -p '{
  "spec": {
    "version": "2.5.0",
    "upgrade": {
      "strategy": "RollingUpdate"
    }
  }
}'`}
/>

<Callout type="note" title="Rolling upgrades can step down more than once">

If leadership moves while the rollout is in progress, you may see multiple step-down Jobs across the same upgrade. That is expected and does not mean the controller restarted the entire workflow.

</Callout>

## Use blue-green when you need a controlled cutover

Choose `BlueGreen` when you need parallel validation, a manual promotion point, or stronger rollback boundaries before the new revision takes over production traffic.

<CommandBlock
  language="yaml"
  label="configure"
  title="Configure a blue-green upgrade with automatic rollback"
  code={`spec:
  version: "2.5.0"
  upgrade:
    strategy: BlueGreen
    preUpgradeSnapshot: true
    blueGreen:
      autoPromote: true
      autoRollback:
        enabled: true
        onJobFailure: true
        onValidationFailure: true`}
/>

<CommandBlock
  language="yaml"
  label="configure"
  title="Add a pre-promotion verification hook"
  code={`spec:
  upgrade:
    strategy: BlueGreen
    blueGreen:
      verification:
        prePromotionHook:
          image: curlimages/curl
          command: ["curl", "-f", "https://green-cluster:8200/v1/sys/health"]`}
>
  Use the hook to prove the Green revision is really healthy before promotion. If the hook fails, the operator either holds or rolls back depending on the `autoRollback` settings.
</CommandBlock>

<DecisionTable
  kind="reference"
  title="Control in-flight upgrades"
  columns={['Control', 'What it does', 'Use it when']}
  rows={[
    {
      cells: [
        'spec.upgrade.requests.retry',
        'Restarts a failed rolling upgrade after you fix the underlying cause.',
        'The operator preserved `status.upgrade.lastErrorReason` and is waiting for an explicit retry.',
      ],
      emphasis: 'recommended',
    },
    {
      cells: [
        'spec.upgrade.requests.promote',
        'Approves promotion when `blueGreen.autoPromote=false` and the Green revision is already healthy.',
        'You want a manual checkpoint before switching traffic.',
      ],
    },
    {
      cells: [
        'blueGreen.autoRollback',
        'Aborts or rolls back automatically when validation or execution fails in supported phases.',
        'You want the operator to recover from bad Green revisions without waiting for a human to react.',
      ],
    },
  ]}
/>

## Verify the rollout outcome

<CommandBlock
  language="bash"
  label="verify"
  title="Inspect the upgrade result"
  code={`kubectl get openbaocluster <name> -n <namespace> -o yaml
kubectl get pods -n <namespace>
kubectl get jobs -n <namespace>`}
>
  Look for an idle cluster rather than just a patched spec. The right end state is healthy pods, no unresolved upgrade error reason, and a condition surface that matches the cluster features you enabled.
</CommandBlock>

<DecisionTable
  kind="reference"
  title="What good looks like after the upgrade"
  columns={['Surface', 'Healthy signal', 'Why it matters']}
  rows={[
    {
      cells: [
        'Cluster phase',
        'Phase returns to Running.',
        'The lifecycle is no longer in an in-flight upgrade state.',
      ],
      emphasis: 'recommended',
    },
    {
      cells: [
        'Availability',
        'Available=True and the workload Pods are Ready.',
        'The new revision is actually serving instead of only existing on paper.',
      ],
    },
    {
      cells: [
        'Upgrade status',
        'No unresolved `status.upgrade.lastErrorReason` and no stalled blue-green phase.',
        'The controller does not think operator action is still required.',
      ],
    },
    {
      cells: [
        'Protection path',
        'Backup status and external dependency conditions remain healthy.',
        'A successful version change should not quietly break the next restore or backup window.',
      ],
    },
  ]}
/>

<NextActions
  title="Keep the change safe"
  items={[
    {
      label: 'Review the production checklist',
      description: 'Use the readiness gate before you call the upgraded cluster your new baseline.',
      docId: 'user-guide/openbaocluster/operations/production-checklist',
    },
    {
      label: 'Open backup operations',
      description: 'Make sure the snapshot path remains healthy before and after the next change window.',
      docId: 'user-guide/openbaocluster/operations/backups',
    },
    {
      label: 'Troubleshoot the cluster',
      description: 'Use the incident guide when TLS, gateway, auth, or runtime assumptions fail after the rollout.',
      docId: 'user-guide/openbaocluster/operations/troubleshooting',
    },
  ]}
/>
