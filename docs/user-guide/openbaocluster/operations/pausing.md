---
title: Pause Reconciliation
description: Stop the operator from mutating a cluster temporarily while you inspect, repair, or stage a narrow maintenance action.
slug: /operate/pause-reconciliation
hide_title: true
pageType: task
journey: operate
---

<PageHeader
  title="Pause the operator only when you need a short-lived manual window."
  lede="Pausing tells the operator to stop normal reconciliation for a specific cluster while you inspect or repair it. Use it for deliberate tactical work, not as a substitute for recovery workflows or as a long-term steady state."
/>

<Checklist
    title="Use this control when"
    items={[
      'you need the operator to stop applying changes during a short repair window',
      'you are debugging a live cluster and do not want reconciliation to race your inspection',
      'you plan to resume normal management after the manual intervention finishes',
      'the cluster is not already in a dedicated safe-mode or recovery workflow',
    ]}
  />


<DecisionTable
  title="What pausing changes"
  columns={['Surface', 'What happens while paused', 'What does not change']}
  rows={[
    {
      cells: [
        'Managed resources',
        'The operator stops reconciling StatefulSets, Services, ConfigMaps, Secrets, and similar managed objects.',
        'Kubernetes native controllers such as StatefulSet behavior still exist and may react independently.',
      ],
      emphasis: 'recommended',
    },
    {
      cells: [
        'Self-healing',
        'The operator does not actively repair drift or push the workload back to the desired spec.',
        'A broken workload does not become safe just because reconciliation stopped.',
      ],
    },
    {
      cells: [
        'Deletion',
        'Finalizer and deletion paths can still run if the custom resource is deleted.',
        'Pausing is not a hard freeze of every lifecycle path.',
      ],
    },
  ]}
/>

## Pause the cluster

<CommandBlock
  language="bash"
  label="apply"
  title="Pause reconciliation for one cluster"
  code={`kubectl patch openbaocluster <name> -n <namespace> --type merge -p '{
  "spec": {
    "paused": true
  }
}'`}
/>

Use this only for the cluster you are actively working on. A paused cluster can drift further away from its desired state while the operator is quiet.

## Verify that the pause is active

<CommandBlock
  language="bash"
  label="verify"
  title="Check the paused flag"
  code={`kubectl get openbaocluster <name> -n <namespace> -o jsonpath='{.spec.paused}{"\\n"}'`}
>
  If updates appear to do nothing, check this field first before assuming the controller is broken.
</CommandBlock>

## Resume reconciliation

<CommandBlock
  language="bash"
  label="apply"
  title="Resume normal reconciliation"
  code={`kubectl patch openbaocluster <name> -n <namespace> --type merge -p '{
  "spec": {
    "paused": false
  }
}'`}
>
  You can also remove the field entirely. The important outcome is that the operator can reconcile the cluster again and converge any pending drift.
</CommandBlock>

<Callout type="warning" title="Pause is not safe mode">

If the operator entered break-glass or safe mode because of quorum loss or another critical failure, simply flipping `spec.paused` back to `false` is not the recovery step. Use the dedicated safe-mode or recovery workflow instead.

</Callout>

<NextActions
  title="Choose the next step"
  items={[
    {
      label: 'Run planned maintenance',
      description: 'Use the broader maintenance workflow when the change involves drains, restarts, scaling, or maintenance mode.',
      docId: 'user-guide/openbaocluster/operations/maintenance',
    },
    {
      label: 'Open safe mode recovery',
      description: 'Switch to the break-glass path when the cluster is no longer in an ordinary troubleshooting state.',
      docId: 'user-guide/openbaocluster/recovery/safe-mode',
    },
    {
      label: 'Troubleshoot the cluster',
      description: 'Go back to the symptom-driven troubleshooting route when you still need to diagnose the failure before changing anything else.',
      docId: 'user-guide/openbaocluster/operations/troubleshooting',
    },
  ]}
/>
