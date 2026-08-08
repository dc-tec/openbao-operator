---
title: Restore a snapshot
description: Create an immutable OpenBaoRestore request, monitor the restore Job, and use lock override only for disaster recovery.
eyebrow: Operate · Restore
weight: 10
verifiedBy:
  - api/v1alpha1/openbaorestore_types.go
  - api/v1alpha1/openbaocluster_operations_types.go
  - config/policy/openbao-validate-openbaorestore.yaml
  - config/rbac/openbaocluster_restore_role.yaml
  - internal/service/restore/manager_validation.go
  - internal/service/restore/manager_running.go
  - internal/service/restore/read_replica_restore.go
  - internal/service/workloadidentity/readiness.go
  - internal/service/restore/manager_test.go
  - internal/service/bootstrap/unseal_validation_transit.go
---

An `OpenBaoRestore` is an explicit, immutable request to download a snapshot and force it into a target cluster. It
uses a dedicated Job and identity, owns the cluster operation lock while destructive work runs, and records a terminal
result.

{{< callout type="danger" title="Restore overwrites OpenBao Raft state" >}}
The selected snapshot replaces the target's current logical state, including stored secrets, policies, auth
configuration, and keys represented in that snapshot. Verify the namespace, cluster name, bucket, and object key with
a second operator before applying the request.
{{< /callout >}}

## Before you begin

{{< checklist title="Restore preflight" >}}
- The target `OpenBaoCluster` exists in the same namespace as the restore request.
- The exact snapshot key exists and has been tested in an isolated restore rehearsal.
- The restore Job can reach object storage and the target cluster.
- A restore JWT role or labeled static-token Secret grants update on `sys/storage/raft/snapshot-force`.
- The storage identity is bound to the generated `<cluster>-restore-serviceaccount` or supplied explicitly.
- Prevent an upgrade or backup from running concurrently.
{{< /checklist >}}

Without `force`, the target must be initialized and must not have `Upgrading=True`. `force: true` skips those two
cluster-state checks; it does not validate the snapshot or prove the target is otherwise safe.

For Hardened targets, configure explicit, port-scoped `spec.network.egressRules` on the target cluster and set
`credentialsSecretRef`, workload-identity metadata, or S3 `roleArn` on the restore source.

## Prepare a cross-cluster restore

A snapshot does not replace the target's unseal root. The target cluster must be able to unwrap the restored barrier
keys with a compatible external seal. For Transit unseal, use the same Transit key and equivalent endpoint, trust,
and credential configuration on the source and target.

Keep the source and target on the same OpenBao version for the restore event. The operator does not validate snapshot
format compatibility across versions, so treat a cross-version restore as unqualified until it has a separate
rehearsal and support decision.

Keep traffic on the source until the target is unsealed, has a leader and expected Raft membership, accepts a real
human or workload login, and returns representative restored data. Cut over traffic manually after those checks.

## Choose restore authentication

Prefer JWT auth. When self-init OIDC bootstrap is enabled, an empty restore role resolves to
`openbao-operator-restore`, which is bound to the generated restore ServiceAccount during initial bootstrap.

If you use a static token, the same-namespace Secret must have both identity labels:

{{< command label="configure" title="Create a scoped restore-token Secret" >}}
apiVersion: v1
kind: Secret
metadata:
  name: restore-token
  namespace: <namespace>
  labels:
    openbao.org/cluster: <cluster>
    openbao.org/credential-purpose: restore-token
stringData:
  token: <openbao-token>
{{< /command >}}

Reference it as `spec.tokenSecretRef.name`. A configured `jwtAuthRole` takes precedence.

## Create the restore request

{{< command label="configure" title="Restore an S3 snapshot" >}}
apiVersion: openbao.org/v1alpha1
kind: OpenBaoRestore
metadata:
  name: prod-restore-001
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
        name: s3-restore-credentials
    key: clusters/security/prod-cluster/last-good.snap
  jwtAuthRole: openbao-operator-restore
{{< /command >}}

The target shape is shared with backups. GCS uses `provider: gcs`, `bucket`, optional `gcs.project`, and a
`credentials.json` Secret. Azure uses `provider: azure`, `bucket`, `azure.storageAccount`, optional
`azure.container`, and an `accountKey` or `connectionString` Secret.

Apply the request once:

{{< command label="apply" title="Start the restore" >}}
kubectl apply -f restore.yaml
{{< /command >}}

Admission requires the caller to have `restore` on the named target `OpenBaoCluster`. Referenced Secrets require
`get`; cloud identity metadata requires `usecloudidentities`; a custom restore image requires
`usecustomexecutables`. The spec cannot be edited after creation. Create a new request to change any restore intent.

## Monitor the lifecycle

{{< command label="verify" title="Watch restore and cluster state together" >}}
kubectl -n <namespace> get openbaorestore <restore-name> -w
kubectl -n <namespace> get openbaorestore <restore-name> -o yaml
kubectl -n <namespace> get jobs -l openbao.org/cluster=<cluster>
kubectl -n <namespace> get openbaocluster <cluster> -o yaml
{{< /command >}}

`status.phase` moves through `Pending`, `Validating`, and `Running`, then ends at `Completed` or `Failed`.
`RestoreConfigurationReady` reports the operator-known auth, storage identity, Secret, and egress prerequisites.

When steady read replicas are configured, the operator drains them before creating the restore Job. After the Job
succeeds, it releases the restore lock and waits for the desired read replicas, `ReadReplicasReady`,
`ReadServingAvailable`, and `RaftMembershipReady` before marking the restore `Completed`.

{{< callout type="note" title="Completed is not full service validation" >}}
Without steady read replicas, `Completed` means the restore Job succeeded. It does not require every voter to be
Ready or prove client access. Verify seal state, leader, Raft membership, application data, and authentication after
every restore.
{{< /callout >}}

## Override a stuck operation lock

Use this only when disaster recovery cannot wait for an upgrade or backup lock to clear normally:

{{< command label="configure" title="Force restore ownership of the operation lock" >}}
spec:
  force: true
  overrideOperationLock: true
{{< /command >}}

`overrideOperationLock` requires `force: true`. The controller can replace a non-restore operation lock and records an
`OperationLockOverride` condition, Warning Event, and audit signal. It does not acknowledge
`status.breakGlass`; that is a separate rollback-recovery decision.

After a forced restore, verify `bao status`, `bao operator raft list-peers`, declared replica readiness, client login,
and representative application data before returning traffic. If the target remains sealed or leaderless, continue
with the corresponding recovery page.
