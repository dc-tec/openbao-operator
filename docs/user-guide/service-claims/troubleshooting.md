---
title: Troubleshoot claim services
description: Capture the failing claim surface first, then route catalog, bootstrap, materialization, workflow, and exposure failures to the right fix.
slug: /service-claims/troubleshooting
hide_title: true
pageType: task
journey: operate
---

<PageHeader
  title="Troubleshoot claim services"
  lede="Capture the claim, the materialized local cluster, and the connection contract first. Then route the symptom to catalog binding, bootstrap dependencies, materialization, or edge publication."
/>

<CommandBlock
  language="bash"
  label="inspect"
  title="Capture the claim surface first"
  code={`kubectl describe openbaoclusterclaim <name> -n <namespace>
kubectl get openbaoclusterclaim <name> -n <namespace> -o yaml
kubectl get openbaoclusterclaimupgraderequest -n <namespace>
kubectl get openbaoclusterclaimbackuprequest -n <namespace>
kubectl get openbaoclusterclaimrestorerequest -n <namespace>
kubectl get openbaocluster -n <namespace>
kubectl get secret <name>-connection -n <namespace>`}
>
  Start here. The claim phase, conditions, materialization status, and connection contract usually show whether the failure is before materialization, inside the local workload, or at the external edge.
</CommandBlock>

<DecisionTable
  title="Choose the first troubleshooting route"
  columns={['Symptom', 'Start here', 'Likely surface', 'Escalate when']}
  rows={[
    {
      cells: [
        'Claim stays Pending',
        'Check the referenced tenant, the service offering, and whether any referenced bootstrap source is still missing.',
        'Tenant onboarding, catalog lookup, or bootstrap dependency readiness.',
        'The claim still has no local materialization reference after the tenant and catalog objects are healthy.',
      ],
      emphasis: 'recommended',
    },
    {
      cells: [
        'Claim fails during materialization',
        'Read the claim conditions and rollout status, then inspect the local materialized OpenBaoCluster reference if one exists.',
        'Unsupported same-cluster shape, admission rejection, or workload-side reconcile failure.',
        'The claim repeats the same materialization failure after the unsupported shape or rejected object is corrected.',
      ],
    },
    {
      cells: [
        'Claim is Ready but there is no external endpoint',
        'Check the selected exposure class, then inspect ingress or gateway readiness.',
        'Ingress or gateway integration is not ready yet.',
        'The edge object reports ready but the claim still never publishes the endpoint.',
      ],
    },
    {
      cells: [
        'Backup or restore request blocks or fails',
        'Check the active request state and reason first, then inspect `status.backup`, `status.restore`, and the local materialized OpenBaoCluster.',
        'Claim workflow request object, backup configuration, or underlying restore execution.',
        'The request repeats the same failure after the referenced backup state, target object, or policy surface is corrected.',
      ],
    },
    {
      cells: [
        'Spec edits are denied after the claim is bound',
        'Check whether the edit changed service selection or another locked materialized surface.',
        'Expected guardrail behavior, not a reconcile bug.',
        'The denied mutation is supposed to be part of the supported bounded claim surface.',
      ],
    },
  ]}
/>

## Claim stays pending

Check these first:

1. `OpenBaoTenant.status.provisioned` for the referenced tenant
2. the referenced `OpenBaoServiceOffering` and the immutable service-profile revision behind it
3. any bootstrap source Secret or ConfigMap referenced by the bootstrap profile

A missing bootstrap source stays on the claim surface as a pending dependency instead of silently producing a half-rendered local cluster.

## Claim fails during materialization

Use the claim status first. The same-cluster path now surfaces local admission failures and unsupported shapes directly on the claim instead of hiding them only in controller logs.

Typical examples:

- same-cluster materialization requested a backup shape that cannot be projected honestly into `OpenBaoCluster.spec.backup`
- the local materialized cluster would violate admission
- the requested bootstrap mode is outside the supported claim surface
- a supported day-2 change was attempted through direct claim spec edits instead of an explicit workflow request

## No endpoint is published yet

For external publication, do not treat an empty endpoint as a generic claim failure immediately.

Check next:

1. `status.connection.endpoint`
2. the claim phase and `status.materialization.localRef`
3. ingress or gateway object readiness behind the selected exposure class
4. the local materialized OpenBaoCluster status if the edge object is ready but the claim is not

## Guardrails reject the edit you tried

These are expected claim safety rules:

- post-materialization service selection is locked
- claim-managed local clusters are protected from direct mutation and deletion
- connection and projected bootstrap artifacts are operator-managed outputs, not tenant-editable inventory

If the workflow you need conflicts with those rules, check whether it is listed as unsupported rather than trying to work around the guardrail.

## Backup or restore request blocks or fails

Check these surfaces in order:

1. the request object `status.state`, `status.reason`, and resolved object references
2. `OpenBaoClusterClaim.status.backup` or `OpenBaoClusterClaim.status.restore`
3. the local materialized `OpenBaoCluster`
4. the underlying `OpenBaoRestore` when a restore request already created one

To see backups created through the claim workflow, list the claim backup requests in the tenant namespace:

```bash
kubectl get openbaoclusterclaimbackuprequest -n <namespace> -o wide
```

Common examples:

- no successful backup exists yet for the restore request to consume
- a restore request selects a backup request that is missing, not `Succeeded`, targets another claim or local cluster, or has no `status.snapshotKey`
- another non-terminal backup or restore request is already active for the same claim
- the local cluster is deleting or no longer exists
- the underlying restore execution failed after request admission

<NextActions
  title="Route to the right next page"
  items={[
    {
      label: 'Read unsupported workflows',
      description: 'Confirm whether the blocked change is intentionally outside the current claim surface.',
      docId: 'user-guide/service-claims/unsupported-workflows',
    },
    {
      label: 'Review claim exposure',
      description: 'Use the exposure page when ingress or gateway readiness is the failing surface.',
      docId: 'user-guide/service-claims/exposure',
    },
    {
      label: 'Open cluster troubleshooting',
      description: 'Switch to the direct workload troubleshooting page when the local OpenBaoCluster is already the failing surface.',
      docId: 'user-guide/openbaocluster/operations/troubleshooting',
    },
  ]}
/>
