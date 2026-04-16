---
title: k3d Cross-Cluster DR Restore
hide_title: true
pageType: runbook
journey: validated-deployments
description: Restore the validated local disaster-recovery target from a source snapshot stored in RustFS while preserving the shared Transit seal-root assumptions the lane depends on.
---

<PageHeader
  title="Restore the target in the local DR lane"
  lede="This runbook restores the target cluster in the validated local DR lane from a source snapshot stored in RustFS. It assumes the source and target already share the same external Transit key and that you are ready to overwrite the target bootstrap state."
/>

<Checklist
    title="This runbook should leave you with"
    items={[
      "a fresh source snapshot written to shared RustFS storage",
      "an `OpenBaoRestore` object that completes on the target cluster",
      "a restored target that unseals with the shared Transit key",
      "post-restore proof that source credentials and source data replaced the target bootstrap state",
    ]}
  />


<Callout type="danger" title="Destructive operation">

This workflow overwrites the target cluster state. Existing auth methods, policies, and data on the target are replaced by the snapshot contents.

</Callout>

<DecisionTable
  title="Restore prerequisites"
  columns={["Requirement", "Why it exists", "What happens if it is wrong"]}
  rows={[
    {
      cells: [
        "Source and target share the same Transit root of trust",
        "Restored data must decrypt under the same external seal key after it lands on the target side.",
        "The restore can complete and the target can still remain sealed.",
      ],
      emphasis: "recommended",
    },
    {
      cells: [
        "The target cluster already exists and exposes restore auth",
        "The restore Job needs a live target to authenticate against and mutate.",
        "The `OpenBaoRestore` object will fail before it can apply the snapshot.",
      ],
    },
    {
      cells: [
        "The snapshot key comes from a fresh successful backup",
        "The runbook is supposed to prove actual source-state transfer, not a stale object lookup.",
        "You may restore the wrong data set and draw the wrong conclusion about the lane.",
      ],
    },
    {
      cells: [
        "Cutover is still manual",
        "Verification must happen before any client-facing change.",
        "You can move traffic to a target that restored incorrectly or still needs operator attention.",
      ],
      emphasis: "caution",
    },
  ]}
/>

<DecisionTable
  kind="reference"
  title="Validated lane defaults"
  columns={["Value", "Default", "Purpose"]}
  rows={[
    {cells: ["Source context", "`k3d-openbao-dr-source`", "Primary cluster that creates the backup."]},
    {cells: ["Target context", "`k3d-openbao-dr-target`", "Recovery target cluster."]},
    {cells: ["Source namespace", "`openbaocluster-dr-source`", "Namespace containing the source cluster."]},
    {cells: ["Target namespace", "`openbaocluster-dr-target`", "Namespace containing the target cluster."]},
    {cells: ["Restore name", "`openbaocluster-dr-target-restore`", "`OpenBaoRestore` name."]},
  ]}
/>

## Step 1: Trigger a fresh source backup

<CommandBlock
  language="bash"
  label="apply"
  title="Trigger the source backup"
  code={`kubectl --context k3d-openbao-dr-source -n openbaocluster-dr-source annotate \\
  openbaocluster openbaocluster-dr-source \\
  openbao.org/trigger-backup="$(date -u +%Y-%m-%dT%H:%M:%SZ)" --overwrite`}
/>

<CommandBlock
  language="bash"
  label="verify"
  title="Wait for the source cluster to finish backing up"
  code={`kubectl --context k3d-openbao-dr-source -n openbaocluster-dr-source \\
  get openbaocluster openbaocluster-dr-source \\
  -o jsonpath='{range .status.conditions[*]}{.type}={.status}{" reason="}{.reason}{"\\n"}{end}'`}
>
  Wait until `BackingUp=False` again before you capture the snapshot key.
</CommandBlock>

## Step 2: Capture the snapshot key

<CommandBlock
  language="bash"
  label="inspect"
  title="Read the backup object key from source status"
  code={`SNAPSHOT_KEY="$(
  kubectl --context k3d-openbao-dr-source -n openbaocluster-dr-source \\
    get openbaocluster openbaocluster-dr-source \\
    -o jsonpath='{.status.backup.lastBackupName}'
)"

printf '%s\\n' "\${SNAPSHOT_KEY}"`}
/>

## Step 3: Apply the restore on the target cluster

<CommandBlock
  language="yaml"
  label="apply"
  title="Apply the validated OpenBaoRestore manifest"
  code={`apiVersion: openbao.org/v1alpha1
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
    key: "<snapshot-key>"
  jwtAuthRole: openbao-operator-restore`}
>
  Replace `<snapshot-key>` with the exact value from the previous step before you apply the manifest.
</CommandBlock>

## Verify the restore

<CommandBlock
  language="bash"
  label="verify"
  title="Watch the restore object and inspect final status"
  code={`kubectl --context k3d-openbao-dr-target -n openbaocluster-dr-target \\
  get openbaorestore openbaocluster-dr-target-restore -w

kubectl --context k3d-openbao-dr-target -n openbaocluster-dr-target \\
  get openbaorestore openbaocluster-dr-target-restore \\
  -o jsonpath='{.status.phase}{"\\n"}{range .status.conditions[*]}{.type}={.status}{" reason="}{.reason}{"\\n"}{end}{.status.snapshotKey}{"\\n"}'`}
>
  The steady-state expectation is `phase=Completed`, `RestoreConfigurationReady=True`, and `RestoreComplete=True` with reason `RestoreSucceeded`.
</CommandBlock>

<CommandBlock
  language="bash"
  label="verify"
  title="Check the target health endpoint after restore"
  code={`curl -ksS --resolve bao-dr-target.example.com:11443:127.0.0.1 \\
  https://bao-dr-target.example.com:11443/v1/sys/health`}
>
  The restored target should return a normal OpenBao health response and the cluster lineage should now match the source snapshot.
</CommandBlock>

<CommandBlock
  language="bash"
  label="verify"
  title="Verify credential cutover and restored data"
  code={`curl -ksS -o /tmp/target-login.json -w '%{http_code}\\n' \\
  --resolve bao-dr-target.example.com:11443:127.0.0.1 \\
  -H 'Content-Type: application/json' \\
  -d '{"password":"target-demo-password"}' \\
  https://bao-dr-target.example.com:11443/v1/auth/userpass/login/demo-admin

SOURCE_TOKEN="$(
  curl -ksS --resolve bao-dr-target.example.com:11443:127.0.0.1 \\
    -H 'Content-Type: application/json' \\
    -d '{"password":"source-demo-password"}' \\
    https://bao-dr-target.example.com:11443/v1/auth/userpass/login/demo-admin \\
  | jq -r '.auth.client_token'
)"

curl -ksS --resolve bao-dr-target.example.com:11443:127.0.0.1 \\
  -H "X-Vault-Token: \${SOURCE_TOKEN}" \\
  https://bao-dr-target.example.com:11443/v1/secret/data/dr-control`}
>
  The old target password should fail. The source password should succeed, and the `dr-control` marker should now show `phase1-source`.
</CommandBlock>

<NextActions
  title="After the restore"
  items={[
    {
      label: "Reference architecture",
      description: "Review the DR invariants again before you consider any manual cutover or cloud equivalent.",
      docId: "user-guide/validated-deployments/architectures/local/k3d-cross-cluster-dr-transit-rustfs",
    },
    {
      label: "Troubleshoot the cluster",
      description: "Use the generic operator incident guide if restore succeeds but the target is still degraded or sealed.",
      docId: "user-guide/openbaocluster/operations/troubleshooting",
    },
    {
      label: "Restore overview",
      description: "Return to the generic restore model when you need the operator-wide explanation behind this lane-specific runbook.",
      docId: "user-guide/openbaorestore/overview",
    },
  ]}
/>
