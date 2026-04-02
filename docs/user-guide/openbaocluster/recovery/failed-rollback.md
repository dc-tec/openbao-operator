---
title: Recover from Failed Rollback
description: Diagnose blue-green rollback failures, decide whether to retry, pause and repair, or restore from backup, and resume automation only when the cluster is safe again.
slug: /recover/failed-rollback
hide_title: true
pageType: runbook
journey: operate
---

<PageHeader
  title="Repair rollback failures without forcing a downgrade."
  lede="A failed rollback means blue-green automation stopped because continuing automatically could worsen Raft safety or cluster availability. Start with the status surface and the last failed rollback Job, then decide whether the right next step is a retry, a controlled pause for manual repair, or a restore from backup."
/>

<Checklist
    title="Use this runbook when"
    items={[
      'a blue-green rollback enters break glass mode',
      'the rollback consensus repair job failed and automation stopped',
      'you need to decide whether the rollback can safely retry or needs manual repair',
      'you need to restore from a known-good snapshot because rollback repair is no longer enough',
    ]}
  />


<Callout type="failure" title="Do not try to downgrade around the failure">

Do not force `spec.version` back to an older release to escape the incident. Downgrades are blocked for a reason. Repair the rollback surface first, then either let the operator continue safely or move into the dedicated restore workflow.

</Callout>

<DecisionTable
  title="Choose the rollback recovery path"
  columns={['Situation', 'Use this path', 'Why']}
  rows={[
    {
      cells: [
        'The rollback failed for a transient reason and the cluster is healthy again.',
        'Retry the rollback by acknowledging the current break-glass nonce.',
        'The operator already knows what work to retry; you only need to confirm the cluster is safe to continue.',
      ],
      emphasis: 'recommended',
    },
    {
      cells: [
        'The cluster still needs manual Raft, workload, or infrastructure repair.',
        'Pause reconciliation, repair manually, then resume with the nonce acknowledgment.',
        'This keeps the operator from racing your repairs while you stabilize the cluster.',
      ],
    },
    {
      cells: [
        'The cluster state is beyond safe rollback repair.',
        'Restore from a known-good snapshot.',
        'Restore is the explicit recovery path when continuing the rollback is no longer trustworthy.',
      ],
    },
  ]}
/>

## Inspect break glass and blue-green state

<CommandBlock
  language="bash"
  label="inspect"
  title="Capture the rollback failure surface"
  code={`kubectl get openbaocluster <name> -n <namespace> -o jsonpath='{.status.breakGlass}' | jq
kubectl get openbaocluster <name> -n <namespace> \\
  -o jsonpath='{.status.blueGreen.phase}{"\\n"}{.status.blueGreen.lastJobFailure}{"\\n"}'
kubectl get openbaocluster <name> -n <namespace> \\
  -o jsonpath='{range .status.conditions[*]}{.type}={.status} {.reason}{"\\n"}{end}'`}
>
  The expected break-glass pattern here is `reason=RollbackConsensusRepairFailed` while the blue-green phase is still in `RollingBack`.
</CommandBlock>

Break-glass reasons during rollback now map to two distinct failure surfaces:

- `RollbackConsensusRepairFailed` usually means the rollback repair Job failed while the phase is still `RollingBack`.
- `RollbackCleanupPeerRemovalFailed` usually means the cleanup Job that removes stale green peers failed while the phase is `RollbackCleanup`.

## Inspect the failed rollback job and live cluster state

<CommandBlock
  language="bash"
  label="inspect"
  title="Inspect the rollback job, Pods, and Raft peers"
  code={`kubectl get jobs -n <namespace> -l openbao.org/cluster=<name>
kubectl logs -n <namespace> job/<job-from-status>
kubectl get pods -n <namespace> -l openbao.org/cluster=<name> -o wide
kubectl exec -n <namespace> -it <pod-name> -- bao operator raft list-peers`}
>
  Look for network isolation between blue and green Pods, stuck or sealed Pods, peer membership that no longer matches the intended rollback topology, or executor-job failures that prevented the rollback from completing.
</CommandBlock>

If the break-glass reason is `RollbackCleanupPeerRemovalFailed`, spend extra time verifying that no stale green peers remain in Raft membership before you acknowledge the nonce. A retry will create a fresh rollback-cleanup attempt, so the live peer list needs to match the rollback intent first.

<Callout type="note" title="Use maintenance mode before disruptive manual repair">

If your repair requires deleting or restarting managed Pods and your admission policy expects the maintenance annotation, enable maintenance mode first and follow <SiteLink docId="user-guide/openbaocluster/operations/maintenance">Run Planned Maintenance</SiteLink>.

</Callout>

## Apply the recovery path that matches the diagnosis

<Tabs groupId="failed-rollback-path">

<TabItem value="retry" label="Retry the rollback">

Use this when the failure was transient and the cluster is healthy again.

<CommandBlock
  language="bash"
  label="apply"
  title="Acknowledge the current break-glass nonce"
  code={`kubectl patch openbaocluster <name> -n <namespace> --type merge -p '{
  "spec": {
    "breakGlassAck": "<NONCE_FROM_STATUS>"
  }
}'`}
/>

Then watch the replacement rollback Job and `status.blueGreen.phase` until the rollback either completes or enters a new break-glass event with a new nonce.

</TabItem>

<TabItem value="pause" label="Pause and repair">

Use this when the cluster needs manual repair before any automation should continue.

<CommandBlock
  language="bash"
  label="apply"
  title="Pause reconciliation before manual repair"
  code={`kubectl patch openbaocluster <name> -n <namespace> --type merge -p '{
  "spec": {
    "paused": true
  }
}'`}
/>

After you repair the cluster, resume reconciliation and acknowledge the current nonce in the same change:

<CommandBlock
  language="bash"
  label="apply"
  title="Resume reconciliation after repair"
  code={`kubectl patch openbaocluster <name> -n <namespace> --type merge -p '{
  "spec": {
    "paused": false,
    "breakGlassAck": "<NONCE_FROM_STATUS>"
  }
}'`}
/>

</TabItem>

<TabItem value="restore" label="Restore from backup">

Use this when the rollback surface is no longer safe to repair in place.

1. stop any further automation
2. identify the last known-good snapshot
3. follow <SiteLink docId="user-guide/openbaorestore/recovery-restore-after-upgrade">Recover After Upgrade Restore</SiteLink>

</TabItem>

</Tabs>

## Reduce repeat rollback failures

- enable pre-upgrade snapshots with `spec.upgrade.preUpgradeSnapshot=true` or `spec.upgrade.blueGreen.preUpgradeSnapshot=true`
- verify backup destination and backup auth before the upgrade window starts
- monitor `status.blueGreen.phase`, `status.blueGreen.lastJobFailure`, and cluster health throughout the rollout

<NextActions
  title="Continue with the right recovery path"
  items={[
    {
      label: 'Enter safe mode',
      description: 'Inspect and acknowledge the break-glass nonce only after the rollback surface is actually repaired.',
      docId: 'user-guide/openbaocluster/recovery/safe-mode',
    },
    {
      label: 'Recover after upgrade restore',
      description: 'Use the override-lock restore path when the rollback or upgrade state blocks the ordinary restore workflow.',
      docId: 'user-guide/openbaorestore/recovery-restore-after-upgrade',
    },
    {
      label: 'Run planned maintenance',
      description: 'Use the maintenance workflow when you need explicit, admission-safe manual repair windows.',
      docId: 'user-guide/openbaocluster/operations/maintenance',
    },
  ]}
/>
