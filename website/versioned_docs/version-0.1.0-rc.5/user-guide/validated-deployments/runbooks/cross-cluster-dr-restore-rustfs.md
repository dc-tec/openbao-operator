---
description: Step-by-step runbook for restoring the validated local cross-cluster DR target from a source snapshot using RustFS and shared Transit auto-unseal.
---

# Cross-Cluster DR Restore with RustFS

This runbook restores the target cluster in the validated local cross-cluster DR lane with:

- a manual backup from the source cluster
- an `OpenBaoRestore` resource on the target cluster
- shared RustFS object storage
- shared Transit auto-unseal on the source and target clusters

<Callout type="success" title="Validated manually">

This runbook matches the local end-to-end DR proof completed on March 16, 2026. In that proof, the source snapshot restored cleanly into the target cluster, the target cluster unsealed with the shared Transit key, `source-demo-password` succeeded on the target, `target-demo-password` failed, and the `dr-control` marker changed to `phase1-source`.

</Callout>

<Callout type="danger" title="Destructive operation">

This workflow overwrites the target cluster state. Existing auth methods, policies, and data on the target are replaced by the snapshot contents.

</Callout>

## Prerequisites

- The validated bootstrap from [k3d Cross-Cluster DR Bootstrap](../recipes/local/k3d-cross-cluster-dr-bootstrap.md) is already running.
- The source cluster is healthy and backup-ready.
- The target cluster is healthy and restore-ready.
- The source and target clusters use the same Transit key and trust bundle.
- `jq` is installed locally for the verification commands.

<Callout type="warning" title="Shared seal is mandatory">

Cross-cluster restore is only valid when the source and target share the same external seal root of trust. In the validated local lane, both clusters use the shared Transit key `openbao-dr-shared-unseal` through the same external Transit provider.

</Callout>

## Inputs

Replace or confirm these values for the validated lane:

| Value | Default | Purpose |
| :--- | :--- | :--- |
| Source context | `k3d-openbao-dr-source` | Primary cluster |
| Target context | `k3d-openbao-dr-target` | Recovery target |
| Source namespace | `openbaocluster-dr-source` | Namespace containing the source cluster |
| Target namespace | `openbaocluster-dr-target` | Namespace containing the target cluster |
| Source cluster | `openbaocluster-dr-source` | Snapshot source |
| Target cluster | `openbaocluster-dr-target` | Restore destination |
| Restore name | `openbaocluster-dr-target-restore` | `OpenBaoRestore` name |

## Step 1: Trigger a source backup

Create a manual source backup:

```bash
kubectl --context k3d-openbao-dr-source -n openbaocluster-dr-source annotate \
  openbaocluster openbaocluster-dr-source \
  openbao.org/trigger-backup="$(date -u +%Y-%m-%dT%H:%M:%SZ)" --overwrite
```

Watch the source cluster until `BackingUp=False` again:

```bash
kubectl --context k3d-openbao-dr-source -n openbaocluster-dr-source \
  get openbaocluster openbaocluster-dr-source \
  -o jsonpath='{range .status.conditions[*]}{.type}={.status}{" reason="}{.reason}{"\n"}{end}'
```

## Step 2: Capture the snapshot key

Read the object key from the source cluster status:

```bash
SNAPSHOT_KEY="$(
  kubectl --context k3d-openbao-dr-source -n openbaocluster-dr-source \
    get openbaocluster openbaocluster-dr-source \
    -o jsonpath='{.status.backup.lastBackupName}'
)"

printf '%s\n' "${SNAPSHOT_KEY}"
```

## Step 3: Apply the target restore

Apply an `OpenBaoRestore` manifest with the captured snapshot key:

```bash
cat <<EOF | kubectl --context k3d-openbao-dr-target apply -f -
apiVersion: openbao.org/v1alpha1
kind: OpenBaoRestore
metadata:
  name: openbaocluster-dr-target-restore
  namespace: openbaocluster-dr-target
spec:
  cluster: openbaocluster-dr-target
  force: true
  image: ghcr.io/dc-tec/openbao-backup:edge
  source:
    target:
      provider: s3
      endpoint: "http://rustfs.openbaocluster-dr-target.svc:19000"
      bucket: "openbao-dr-backups"
      usePathStyle: true
      credentialsSecretRef:
        name: rustfs-secret
    key: "${SNAPSHOT_KEY}"
  jwtAuthRole: openbao-operator-restore
EOF
```

## Operations

### Watch the restore CR

Watch the target `OpenBaoRestore` resource:

```bash
kubectl --context k3d-openbao-dr-target -n openbaocluster-dr-target \
  get openbaorestore openbaocluster-dr-target-restore -w
```

Inspect the final status:

```bash
kubectl --context k3d-openbao-dr-target -n openbaocluster-dr-target \
  get openbaorestore openbaocluster-dr-target-restore \
  -o jsonpath='{.status.phase}{"\n"}{range .status.conditions[*]}{.type}={.status}{" reason="}{.reason}{"\n"}{end}{.status.snapshotKey}{"\n"}'
```

The steady-state expectation is:

- `phase=Completed`
- `RestoreConfigurationReady=True`
- `RestoreComplete=True` with reason `RestoreSucceeded`

### Verify the target cluster health after restore

Check the target health endpoint:

```bash
curl -ksS --resolve bao-dr-target.example.com:11443:127.0.0.1 \
  https://bao-dr-target.example.com:11443/v1/sys/health
```

The target cluster ID should now match the source cluster ID from the snapshot lineage.

### Verify credential cutover

The restored target should reject the old target bootstrap password:

```bash
curl -ksS -o /tmp/target-login.json -w '%{http_code}\n' \
  --resolve bao-dr-target.example.com:11443:127.0.0.1 \
  -H 'Content-Type: application/json' \
  -d '{"password":"target-demo-password"}' \
  https://bao-dr-target.example.com:11443/v1/auth/userpass/login/demo-admin
```

The expected result is a non-`200` response.

The restored target should now accept the source password:

```bash
SOURCE_TOKEN="$(
  curl -ksS --resolve bao-dr-target.example.com:11443:127.0.0.1 \
    -H 'Content-Type: application/json' \
    -d '{"password":"source-demo-password"}' \
    https://bao-dr-target.example.com:11443/v1/auth/userpass/login/demo-admin \
  | jq -r '.auth.client_token'
)"

printf '%s\n' "${SOURCE_TOKEN}"
```

### Verify the restored application data

Read the `dr-control` marker from the restored target:

```bash
curl -ksS --resolve bao-dr-target.example.com:11443:127.0.0.1 \
  -H "X-Vault-Token: ${SOURCE_TOKEN}" \
  https://bao-dr-target.example.com:11443/v1/secret/data/dr-control
```

The expected result is:

- `marker=phase1-source`
- `sourceCluster=openbao-dr-source`

## Common failures

- The restore completes but the target remains sealed: the source and target do not actually share the same external seal root of trust.
- The restore Job cannot authenticate: verify that the target cluster exposes `spec.restore.jwtAuthRole` and that the generated restore `ServiceAccount` exists.
- The restore Job cannot reach storage: verify the RustFS endpoint, credentials Secret, and the exact object key.
- `target-demo-password` still works afterward: confirm that the restore used the expected source snapshot and that the target cluster fully returned to healthy state before re-testing.

## See also

- [k3d Cross-Cluster DR with Shared Transit and RustFS](../architectures/local/k3d-cross-cluster-dr-transit-rustfs.md)
- [k3d Cross-Cluster DR Bootstrap](../recipes/local/k3d-cross-cluster-dr-bootstrap.md)
- [Restore from an S3-Compatible Snapshot](restore-from-s3-compatible-snapshot.md)

