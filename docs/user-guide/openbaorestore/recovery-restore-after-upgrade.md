---
title: Recover After Upgrade Restore
description: Use the override-lock restore path only when a failed upgrade or rollback blocks the normal restore workflow.
hide_title: true
pageType: runbook
journey: operate
---

<PageHeader
  title="Override-lock restore after a failed upgrade"
  lede="This runbook covers the case where the normal restore path is blocked by an existing cluster operation lock, usually after a failed rollback or another crashed automation loop. Use it for recovery when the restore request cannot proceed through the normal lock path."
/>

<Checklist
    title="Use this runbook when"
    items={[
      'the cluster is stuck behind an operation lock after a failed upgrade or rollback',
      'a normal OpenBaoRestore request is blocked by that lock',
      'you accept that the current cluster state will be overwritten by the snapshot',
      'you already identified the last known good snapshot you are willing to restore',
    ]}
  />


<Callout type="danger" title="This bypasses the existing operation lock">

This path ignores the current lock owner, overwrites cluster state with the selected snapshot, and is meant for disaster recovery under operator supervision. Do not use it when the normal restore workflow is still available.

</Callout>

<DecisionTable
  title="Know what this override changes"
  columns={['Field', 'What it does', 'Why it matters']}
  rows={[
    {
      cells: [
        '`force: true`',
        'Allows restore to proceed on an unhealthy cluster.',
        'You are explicitly acknowledging that normal safety checks are being relaxed for recovery.',
      ],
      emphasis: 'recommended',
    },
    {
      cells: [
        '`overrideOperationLock: true`',
        'Clears the existing cluster operation lock so restore can proceed.',
        'This is what makes the workflow break-glass instead of routine restore.',
      ],
    },
    {
      cells: [
        '`spec.breakGlassAck` on OpenBaoCluster',
        'May still be required later if the cluster remains in break-glass mode after restore.',
        'The restore override is separate from the cluster break-glass acknowledgment flow.',
      ],
    },
  ]}
/>

## Create the break-glass restore request

<CommandBlock
  language="yaml"
  label="configure"
  title="Force restore past an existing operation lock"
  code={`apiVersion: openbao.org/v1alpha1
kind: OpenBaoRestore
metadata:
  name: emergency-restore-001
  namespace: security
spec:
  cluster: prod-cluster
  source:
    target:
      provider: s3
      endpoint: https://s3.amazonaws.com
      bucket: openbao-backups
      region: us-east-1
      credentialsSecretRef:
        name: s3-credentials
    key: clusters/prod/last-good-snapshot.snap
  jwtAuthRole: openbao-operator-restore
  force: true
  overrideOperationLock: true`}
>
  `force` is required when restore targets an unhealthy cluster. `overrideOperationLock` is what bypasses the stuck upgrade or backup lock. Keep them together only for this break-glass path.
</CommandBlock>

<CommandBlock
  language="bash"
  label="apply"
  title="Apply the override restore"
  code={`kubectl apply -f emergency-restore.yaml`}
/>

## Verify the restore and plan the follow-up

<CommandBlock
  language="bash"
  label="verify"
  title="Inspect restore and cluster state"
  code={`kubectl get openbaorestore <name> -n <namespace> -o yaml
kubectl describe openbaocluster <cluster> -n <namespace>
kubectl get jobs -n <namespace>`}
>
  A completed restore only means the restore workflow finished. The target cluster may still require unseal, Raft repair, or break-glass acknowledgment before it is truly operational again.
</CommandBlock>

<NextActions
  title="Finish the recovery"
  items={[
    {
      label: 'Recover a sealed cluster',
      description: 'Use this when the restored workload starts but cannot unseal cleanly.',
      docId: 'user-guide/openbaocluster/recovery/sealed-cluster',
    },
    {
      label: 'Recover from no leader',
      description: 'Use this when the restored snapshot leaves the cluster needing Raft peer or quorum repair.',
      docId: 'user-guide/openbaocluster/recovery/no-leader',
    },
    {
      label: 'Enter safe mode',
      description: 'Go here when the operator still requires explicit break-glass acknowledgment after the restore completes.',
      docId: 'user-guide/openbaocluster/recovery/safe-mode',
    },
  ]}
/>
