---
title: Enter Safe Mode
description: Inspect break-glass status, repair the underlying failure, and acknowledge the nonce only after the cluster is safe to resume.
slug: /recover/safe-mode
hide_title: true
pageType: runbook
journey: operate
---

<PageHeader
  title="Safe mode and break-glass recovery"
  lede="Break glass or safe mode is the operator's explicit stop signal when continuing rollback automation could make availability or Raft safety worse. Use this page to inspect the break-glass state, stabilize the cluster, repair the failure, and then resume automation."
/>

<Checklist
    title="Use this runbook when"
    items={[
      'the operator reports break glass or safe mode on the cluster',
      'rollback automation stopped because continuing automatically is unsafe',
      'you need a clear pause before restarting pods or modifying Raft state',
      'you are ready to acknowledge a nonce only after the underlying issue is fixed',
    ]}
  />


<DecisionTable
  title="What safe mode means"
  columns={['Signal', 'What the operator is doing', 'Why it matters']}
  rows={[
    {
      cells: [
        'Risky automation halted',
        'The operator stops the affected upgrade or rollback workflow instead of pushing forward blindly.',
        'This is the point where a human has to evaluate whether the live cluster is still repairable.',
      ],
      emphasis: 'recommended',
    },
    {
      cells: [
        'status.breakGlass populated',
        'The cluster status contains the reason, message, nonce, and suggested next checks.',
        'Start with that status so the recovery path follows the recorded failure reason and suggested checks.',
      ],
    },
    {
      cells: [
        'Manual acknowledgment required',
        'Automation stays paused until `spec.breakGlassAck` matches the current nonce.',
        'Acknowledgment is the explicit signal that you have repaired the issue and accept resumed automation.',
      ],
    },
  ]}
/>

## Inspect the break-glass state

<CommandBlock
  language="bash"
  label="inspect"
  title="Read the break-glass status"
  code={`kubectl get openbaocluster <name> -n <namespace> -o jsonpath='{.status.breakGlass}' | jq`}
/>

<CommandBlock
  language="json"
  label="output"
  title="Typical break-glass payload"
  code={`{
  "active": true,
  "reason": "RollbackConsensusRepairFailed",
  "message": "Rollback consensus repair Job upgrade-prod-cluster-rollback-retry-1 failed; manual intervention required.",
  "nonce": "abc-123-def-456",
  "steps": [
    "Inspect rollback Job logs",
    "Inspect pod status",
    "Perform any required Raft recovery steps, then acknowledge the nonce"
  ]
}`}
>
  The `reason`, `message`, and `steps` fields are the fastest way to decide whether you are looking at an upgrade rollback problem, a cleanup failure after rollback, a Raft recovery problem, or a broader cluster-health issue.
</CommandBlock>

For blue-green rollback incidents, the most common reasons are:

- `RollbackConsensusRepairFailed`: the operator could not complete the rollback repair path while the cluster was still in `RollingBack`.
- `RollbackCleanupPeerRemovalFailed`: the rollback itself converged far enough to enter `RollbackCleanup`, but the peer-removal cleanup job failed and automation stopped before stale green peers were safely removed.

## Repair the underlying issue

Start with the operator-visible status and the last failed job, then move into the narrower runbook that matches the cluster state.

<CommandBlock
  language="bash"
  label="inspect"
  title="Capture the current failure surface"
  code={`kubectl get openbaocluster <name> -n <namespace> \\
  -o jsonpath='{range .status.conditions[*]}{.type}={.status} {.reason}{"\\n"}{end}'
kubectl get openbaocluster <name> -n <namespace> \\
  -o jsonpath='{.status.blueGreen.lastJobFailure}{"\\n"}'
kubectl logs -n <namespace> job/<job-from-status>
kubectl get pods -n <namespace> -l openbao.org/cluster=<name> -o wide
kubectl exec -n <namespace> -it <pod-name> -- bao operator raft list-peers`}
>
  These commands tell you whether the failure is still centered on the rollback job, whether the Pods are actually healthy, and whether Raft membership is still coherent enough for a safe retry.
</CommandBlock>

<Callout type="note" title="Use maintenance mode for controlled manual repair">

If you need to restart or delete managed Pods while admission policies require the `openbao.org/maintenance=true` signal, enable maintenance mode first and follow <SiteLink docId="user-guide/openbaocluster/operations/maintenance">Run Planned Maintenance</SiteLink>.

</Callout>

If the cluster needs a deeper incident path, move directly into the matching runbook instead of staying in generic safe mode:

- <SiteLink docId="user-guide/openbaocluster/recovery/failed-rollback">Recover from failed rollback</SiteLink>
- <SiteLink docId="user-guide/openbaocluster/recovery/no-leader">Recover from no leader</SiteLink>
- <SiteLink docId="user-guide/openbaocluster/recovery/sealed-cluster">Recover a sealed cluster</SiteLink>

## Acknowledge and resume automation

Only acknowledge the nonce after the cluster is healthy enough for the operator to continue the paused workflow.

<CommandBlock
  language="bash"
  label="apply"
  title="Acknowledge the current nonce"
  code={`kubectl patch openbaocluster <name> -n <namespace> --type merge -p '{
  "spec": {
    "breakGlassAck": "<NONCE_FROM_STATUS>"
  }
}'`}
>
  If the operator re-enters break glass later, it will issue a new nonce. Always use the current value from `status.breakGlass.nonce`, not a previously copied one.
</CommandBlock>

<NextActions
  title="Go deeper"
  items={[
    {
      label: 'Recover from failed rollback',
      description: 'Use the rollback-specific runbook when the break-glass reason is tied to blue-green rollback repair.',
      docId: 'user-guide/openbaocluster/recovery/failed-rollback',
    },
    {
      label: 'Recover from no leader',
      description: 'Switch here when the cluster cannot elect or keep a leader after the rollback failure.',
      docId: 'user-guide/openbaocluster/recovery/no-leader',
    },
    {
      label: 'Recover a sealed cluster',
      description: 'Use the seal-focused path when Pods are running but trust or unseal dependencies still block service.',
      docId: 'user-guide/openbaocluster/recovery/sealed-cluster',
    },
  ]}
/>
