---
title: Plan Upgrades
description: Choose the right rollout strategy, stage upgrade auth deliberately, and verify the cluster before and after a version change.
slug: /operate/upgrades
hide_title: true
pageType: task
journey: operate
---

<PageHeader
  title="Upgrade planning and rollout"
  lede="The operator supports rolling and blue-green upgrades, but both paths depend on cluster health, backup posture, and explicit authentication for the executor Jobs. Use this page to choose the strategy, stage the right config, and verify the rollout cleanly."
/>



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

<Callout type="note" title="Switch strategies only while the cluster is idle">

The operator supports `RollingUpdate` to `BlueGreen` and `BlueGreen` to `RollingUpdate` without renaming or replacing the active StatefulSet. Admission allows the change only after initialization, while every voter is Ready, `status.currentVersion` equals `spec.version`, and no upgrade, backup, restore, resize, restart, Green workload, pending request, failure, or safe-mode recovery is active.

Change only `spec.upgrade.strategy`, then wait until `status.acceptedUpgradeStrategy` reports the new value. Change `spec.version` or other workload settings only after that acknowledgment. A strategy change that does not meet these conditions is rejected with recovery guidance.

</Callout>

<Callout type="warning" title="OpenBao 2.6.x cannot complete a mixed-version BlueGreen upgrade">

OpenBao 2.6.0 changed its internal request-forwarding gRPC service name. During a pre-2.6 to 2.6.x `BlueGreen` upgrade, Green peers cannot report Raft Autopilot health to the Blue leader and therefore cannot be promoted safely. The operator rejects pre-2.6 to 2.6-or-newer transitions before creating Green resources until a compatible target is explicitly qualified.

Fresh 2.6.x clusters and the `RollingUpdate` path remain supported. If a pre-2.6 cluster is configured for `BlueGreen`, first let the cluster return to a healthy `Idle` state, switch only the strategy to `RollingUpdate`, wait for `status.acceptedUpgradeStrategy=RollingUpdate`, and then request the 2.6.x version change.

</Callout>

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

## Prepare the rollout

- Confirm the cluster is initialized, healthy, and already safe to change. An upgrade is not the time to discover a broken backup path or an unstable seal configuration.
- Set `spec.version` to the target semantic version. The operator blocks downgrades and validates semver format.
- If you override `spec.image`, keep it aligned with `spec.version`. Semantic-version tags must match, and digest-pinned images still require `spec.version` as the authoritative intent.
- Make sure the upgrade executor Job can authenticate:
  - with the default JWT role created from `selfInit.oidc`, or
  - with an explicit `spec.upgrade.jwtAuthRole`
- If you want pre-upgrade snapshots, configure `spec.backup` first and make sure the backup auth path is already working.
- In the `Hardened` profile, explicitly allow egress to object storage or other external dependencies the upgrade path needs.

## Change the strategy of an existing cluster

Use this sequence in either direction. It deliberately separates the control-plane strategy transition from the next workload change.

1. Finish or recover every active upgrade, backup, restore, resize, restart, promotion, rollback, or safe-mode workflow.
2. Verify `status.phase=Running`, `status.currentVersion=spec.version`, all voter and configured read replicas are Ready, `Available=True`, and BlueGreen is absent or `Idle` with no Green revision.
3. Ensure the BlueGreen executor prerequisites are configured before switching to `BlueGreen`: an explicit `spec.upgrade.jwtAuthRole` or the default role created by enabled `selfInit.oidc`, plus a resolvable upgrade executor image. The referenced role must already grant the BlueGreen raft join, configuration, remove-peer, promote, and demote capabilities. Self-init policies are created during initial bootstrap and are not rewritten by a later strategy change, so update the role policy first when a rolling-origin cluster was initialized with rolling-only permissions.
4. Patch only `spec.upgrade.strategy`.
5. Wait for `status.acceptedUpgradeStrategy` to equal the requested strategy.
6. Patch `spec.version`, `spec.image`, replicas, storage, or restart controls in a later request.

<CommandBlock
  language="bash"
  label="switch"
  title="Switch an idle cluster from BlueGreen to RollingUpdate"
  code={`kubectl patch openbaocluster <name> -n <namespace> --type merge -p '{
  "spec": {
    "upgrade": {
      "strategy": "RollingUpdate"
    }
  }
}'

kubectl get openbaocluster <name> -n <namespace> \
  -o jsonpath='{.status.acceptedUpgradeStrategy}{"\\n"}'`}
/>

<CommandBlock
  language="bash"
  label="switch"
  title="Switch an idle cluster from RollingUpdate to BlueGreen"
  code={`kubectl patch openbaocluster <name> -n <namespace> --type merge -p '{
  "spec": {
    "upgrade": {
      "strategy": "BlueGreen"
    }
  }
}'

kubectl get openbaocluster <name> -n <namespace> \
  -o jsonpath='{.status.acceptedUpgradeStrategy}{"\\n"}'`}
/>

The active StatefulSet and its PVCs remain the stable workload after either transition. A later rolling rollout continues against a revisioned StatefulSet when switching from BlueGreen; a later blue-green rollout treats the original unrevisioned StatefulSet as Blue when switching from RollingUpdate.

<Callout type="note" title="Upgrade auth is separate from backup auth">

The upgrade executor Job uses JWT auth to talk to OpenBao. Pre-upgrade snapshots use the backup configuration and backup identity path. Do not assume one automatically covers the other.

</Callout>

<Callout type="warning" title="Custom upgrade executables need delegated RBAC">

Setting `spec.upgrade.image` or a blue-green `prePromotionHook` selects custom executables for the upgrade path. The identity applying that `OpenBaoCluster` needs `usecustomexecutables` on the cluster; existing `usehelperimages` bindings remain accepted for compatibility.

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

<Callout type="note" title="Read replicas change the rollout ordering">

When steady read replicas are configured, the operator upgrades the read pool first and only then starts the voter partition rollout. For blue-green, the operator stages the steady read pool down before cutover, restores it afterward, and only then returns the workflow to `Idle`.

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
        'The operator preserved `status.upgrade.failure.reason` (and the deprecated `lastError*` compatibility fields) and is waiting for an explicit retry.',
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
  Look for an idle cluster rather than just a patched spec. The right end state is healthy pods, no unresolved upgrade failure state, and a condition surface that matches the cluster features you enabled.
</CommandBlock>

<DecisionTable
  kind="reference"
  title="Expected signals after the upgrade"
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
        'No unresolved `status.upgrade.failure.reason` (or deprecated `lastErrorReason`) and no stalled blue-green phase.',
        'The controller does not think operator action is still required.',
      ],
    },
    {
      cells: [
        'Protection path',
        'Backup status and external dependency conditions remain healthy.',
        'A successful version change keeps the next restore or backup window intact.',
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
