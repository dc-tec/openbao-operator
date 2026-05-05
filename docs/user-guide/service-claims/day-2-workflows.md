---
title: Operate claim services
description: Request bounded same-cluster service upgrades, manual backups, and restores without editing the claim-managed OpenBaoCluster directly.
slug: /service-claims/day-2-workflows
hide_title: true
pageType: task
journey: operate
---

<PageHeader
  title="Operate claim services"
  lede="Use explicit request objects for in-place upgrades, manual backups, and restores. Do not edit the materialized claim or the local OpenBaoCluster directly for these workflows."
/>

<Checklist
  title="Prerequisites"
  items={[
    'start from a same-cluster OpenBaoClusterClaim that already reached Ready at least once',
    'keep admission policies and the claim controller enabled',
    'configure a backup profile on the published service profile before using backup or restore requests',
    'treat restore as a destructive workflow and confirm the latest successful backup or selected completed backup request is the one you want to roll back to',
  ]}
/>

<DecisionTable
  title="Choose the bounded claim workflow"
  columns={['Workflow', 'Use it for', 'Backed by', 'Current scope']}
  rows={[
    {
      cells: [
        'Offering rollout',
        'Let a platform admin move selected claims bound to a service offering to the offering\'s current service-profile revision.',
        '`OpenBaoServiceOfferingRollout` creates `OpenBaoClusterClaimUpgradeRequest` objects',
        'Same-cluster materialized claims and in-place compatible changes only.',
      ],
      emphasis: 'recommended',
    },
    {
      cells: [
        'Upgrade request',
        'Move one existing claim to a newer compatible service-offering or service-profile revision.',
        '`OpenBaoClusterClaimUpgradeRequest`',
        'Same-cluster materialized claims and in-place compatible changes only.',
      ],
    },
    {
      cells: [
        'Manual backup request',
        'Capture an immediate snapshot without waiting for the scheduled backup window.',
        '`OpenBaoClusterClaimBackupRequest`',
        'Same-cluster materialized claims with backup already configured.',
      ],
    },
    {
      cells: [
        'Restore request',
        'Roll the claim-managed service back to the latest successful backup or a selected completed claim backup request.',
        '`OpenBaoClusterClaimRestoreRequest`',
        'Same-cluster materialized claims only. Restore source is bounded to the latest successful backup or a completed `OpenBaoClusterClaimBackupRequest` for the same claim and local cluster.',
      ],
    },
  ]}
/>

<Callout type="warning" title="Restore is destructive">

`OpenBaoClusterClaimRestoreRequest` does not let tenants pick an arbitrary snapshot key or external source. The request restores the latest successful backup for the claim-managed local cluster or a selected completed claim backup request. Treat it as a bounded rollback workflow, not as a general restore control plane.

When you need a specific claim-created snapshot, select the completed `OpenBaoClusterClaimBackupRequest` instead of supplying a raw object-store key.

</Callout>

## Request an in-place upgrade

Use an offering rollout when the platform owns the OpenBao version movement for
all or a selected subset of claims. First publish the new immutable
`OpenBaoServiceProfile`, then move the stable `OpenBaoServiceOffering` alias to
that revision, then create the rollout object.

<CommandBlock
  language="yaml"
  label="configure"
  title="Roll out the current offering revision"
  code={`apiVersion: openbao.org/v1alpha1
kind: OpenBaoServiceOffering
metadata:
  name: dev-internal
spec:
  currentRevisionRef:
    name: dev-internal-v2
---
apiVersion: openbao.org/v1alpha1
kind: OpenBaoServiceOfferingRollout
metadata:
  name: dev-internal-v2-rollout
spec:
  offeringRef:
    name: dev-internal
  targetRevisionRef:
    name: dev-internal-v2
  selector:
    namespaces:
      - team-a-prod
      - team-b-prod
  strategy:
    maxConcurrent: 1
    mode: InPlaceOnly`}
>
  The rollout selects claims currently applied through `dev-internal`, creates one namespaced `OpenBaoClusterClaimUpgradeRequest` per eligible claim, and waits for those request objects to report progress. The target revision must match `OpenBaoServiceOffering.spec.currentRevisionRef.name`; otherwise the rollout blocks without creating requests.
</CommandBlock>

Use an explicit upgrade request when the platform has published a newer service revision and the change is still inside the supported in-place boundary.

<CommandBlock
  language="yaml"
  label="configure"
  title="Upgrade through the stable offering alias"
  code={`apiVersion: openbao.org/v1alpha1
kind: OpenBaoClusterClaimUpgradeRequest
metadata:
  name: team-a-vault-upgrade
  namespace: team-a-prod
spec:
  claimRef:
    name: team-a-vault
  target:
    serviceOfferingRef:
      name: dev-internal`}
>
  The operator classifies the target first. Unsupported changes fail closed as `Blocked` instead of mutating the claim spec directly.
</CommandBlock>

## Request a manual backup

Use a manual backup request when the service needs a fresh snapshot before another controlled change.

<CommandBlock
  language="yaml"
  label="configure"
  title="Create a manual backup request"
  code={`apiVersion: openbao.org/v1alpha1
kind: OpenBaoClusterClaimBackupRequest
metadata:
  name: team-a-vault-backup
  namespace: team-a-prod
spec:
  claimRef:
    name: team-a-vault`}
/>

Claim backup requests are also the inventory of claim-created backups. Successful requests surface the resolved snapshot key in status and in the wide `kubectl get` view.

<CommandBlock
  language="bash"
  label="inspect"
  title="List claim-created backups"
  code={`kubectl get openbaoclusterclaimbackuprequest -n team-a-prod -o wide
kubectl get openbaoclusterclaimbackuprequest team-a-vault-backup -n team-a-prod -o yaml`}
/>

## Request a restore

Use a restore request when the tenant or platform admin needs to roll the service back to the latest successful backup or to a completed claim backup request.

<CommandBlock
  language="yaml"
  label="configure"
  title="Restore the latest successful backup"
  code={`apiVersion: openbao.org/v1alpha1
kind: OpenBaoClusterClaimRestoreRequest
metadata:
  name: team-a-vault-restore
  namespace: team-a-prod
spec:
  claimRef:
    name: team-a-vault`}
>
  The omitted `source` defaults to `LatestSuccessful`. The operator resolves the current same-cluster target, verifies backup state, and creates the underlying `OpenBaoRestore` execution in the target cluster namespace.
</CommandBlock>

<CommandBlock
  language="yaml"
  label="configure"
  title="Restore a selected claim backup request"
  code={`apiVersion: openbao.org/v1alpha1
kind: OpenBaoClusterClaimRestoreRequest
metadata:
  name: team-a-vault-restore-from-backup
  namespace: team-a-prod
spec:
  claimRef:
    name: team-a-vault
  source:
    mode: BackupRequest
    backupRequestRef:
      name: team-a-vault-backup`}
>
  The referenced backup request must be in the same namespace, target the same claim, have `status.state: Succeeded`, resolve to the current local cluster, and expose a non-empty `status.snapshotKey`.
</CommandBlock>

## Observe the workflow on the claim first

Start with the claim surface before dropping into raw workflow objects.

<CommandBlock
  language="bash"
  label="inspect"
  title="Inspect claim and workflow status"
  code={`kubectl get openbaoclusterclaim team-a-vault -n team-a-prod -o yaml
kubectl get openbaoserviceofferingrollout dev-internal-v2-rollout -o yaml
kubectl get openbaoclusterclaimupgraderequest team-a-vault-upgrade -n team-a-prod -o yaml
kubectl get openbaoclusterclaimbackuprequest -n team-a-prod -o wide
kubectl get openbaoclusterclaimbackuprequest team-a-vault-backup -n team-a-prod -o yaml
kubectl get openbaoclusterclaimrestorerequest team-a-vault-restore -n team-a-prod -o yaml`}
>
  Watch `status.phase`, `status.summary`, `status.upgrade`, `status.backup`, and `status.restore` on the claim. Request objects then give the narrower workflow view with current state, reason, and the resolved target object references.
</CommandBlock>

<CommandBlock
  language="bash"
  label="apply"
  title="Apply the checked-in request examples"
  code={`kubectl apply -f config/samples/claims/claim-day2-requests.yaml`}
>
  The sample contains one upgrade request, one manual backup request, and both restore source modes. Apply only the operation you intend to run against a ready claim.
</CommandBlock>

<DecisionTable
  kind="reference"
  title="What to expect during workflow execution"
  columns={['Workflow', 'Claim view while active', 'Request object state when healthy']}
  rows={[
    {
      cells: [
        'Offering rollout',
        'Selected claims usually move to `Degraded` one at a time while their generated upgrade requests run.',
        '`Running`, then `Succeeded`; `Blocked` if any generated upgrade request classifies the target as unsupported.',
      ],
      emphasis: 'recommended',
    },
    {
      cells: [
        'Upgrade request',
        'Claim usually moves to `Degraded` while `status.summary` points at the active upgrade request.',
        '`RollingOut`, then `Succeeded`.',
      ],
      emphasis: 'recommended',
    },
    {
      cells: [
        'Manual backup request',
        'Claim stays available while `status.backup.requestRef` and `status.summary` reflect the active backup.',
        '`Pending` or `Running`, then `Succeeded`.',
      ],
    },
    {
      cells: [
        'Restore request',
        'Claim usually moves to `Degraded` while `status.restore.requestRef`, `status.restore.executionRef`, and `status.summary` reflect the active restore.',
        '`Pending` or `Running`, then `Succeeded`.',
      ],
    },
  ]}
/>

<Callout type="note" title="Workflow serialization">

The request objects are immutable and the operator serializes same-kind maintenance work per claim. Create a new request for each new operation instead of editing an existing one.

</Callout>

<NextActions
  title="Continue claim operations"
  items={[
    {
      label: 'Troubleshoot claim services',
      description: 'Route blocked or failed day-2 workflows to the right controller, policy, or workload surface.',
      docId: 'user-guide/service-claims/troubleshooting',
    },
    {
      label: 'Open status and events',
      description: 'Review the exact claim and workflow state model when you need the precise reason values.',
      docId: 'reference/status-and-events',
    },
    {
      label: 'Read unsupported workflows',
      description: 'Check the current boundaries before assuming claim restore or upgrade supports broader migration scenarios.',
      docId: 'user-guide/service-claims/unsupported-workflows',
    },
  ]}
/>
