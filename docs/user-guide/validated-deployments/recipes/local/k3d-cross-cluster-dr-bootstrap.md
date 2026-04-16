---
title: k3d Cross-Cluster DR Bootstrap
hide_title: true
pageType: task
journey: validated-deployments
description: Stand up the validated local disaster-recovery environment with separate infra, source, and target clusters, shared Transit, RustFS storage, and passthrough ingress.
---

<PageHeader
  title="Bootstrap the local cross-cluster DR baseline"
  lede="This recipe prepares the validated local disaster-recovery lane: a shared trust-services cluster, a source cluster, and a target cluster, all wired so the restore event crosses the same trust, storage, and ingress boundaries used in a real DR rehearsal."
/>

<Checklist
    title="Recipe outcomes"
    items={[
      "an infra cluster that hosts shared trust services and the shared Transit key",
      "a healthy source cluster and target cluster with distinct namespaces and external endpoints",
      "shared RustFS storage available to both clusters for snapshot transfer",
      "known pre-restore state on both sides so the restore event can be verified later",
    ]}
  />


<Callout type="success" title="Validated coverage">

This bootstrap path matches the local DR lane that was proven end to end on March 16, 2026, including source backup, restore into the target cluster, target unseal, and credential plus data verification after restore.

</Callout>

<DecisionTable
  title="What this lane assumes"
  columns={["Assumption", "Why it exists", "What breaks if it is wrong"]}
  rows={[
    {
      cells: [
        "You can create three k3d-backed contexts",
        "The lane depends on an infra cluster plus separate source and target clusters.",
        "If you collapse everything into one cluster, you stop proving the cluster-boundary part of DR.",
      ],
      emphasis: "recommended",
    },
    {
      cells: [
        "You have bootstrap automation or manifests for the three-cluster lab",
        "The validated lane was assembled from repeatable infra, gateway, and operator setup rather than ad hoc kubectl edits.",
        "Without repeatable bootstrap artifacts, the restore result is hard to trust or reproduce.",
      ],
    },
    {
      cells: [
        "Shared Transit and shared storage are available before cluster apply",
        "Source and target clusters both depend on them from the start.",
        "The restore will fail later if these dependencies are improvised after the source cluster is already in use.",
      ],
    },
    {
      cells: [
        "Cutover remains manual",
        "The bootstrap only prepares the recovery pair; it does not automate failover.",
        "Treating the lane as automatic DR creates false confidence in behavior it does not validate.",
      ],
      emphasis: "caution",
    },
  ]}
/>

<DecisionTable
  kind="reference"
  title="Baseline defaults"
  columns={["Value", "Default", "Purpose"]}
  rows={[
    {cells: ["Infra context", "`k3d-openbao-dr-infra`", "Shared trust-services cluster."]},
    {cells: ["Source context", "`k3d-openbao-dr-source`", "Primary cluster that creates the snapshot."]},
    {cells: ["Target context", "`k3d-openbao-dr-target`", "Recovery target cluster."]},
    {cells: ["Source hostname", "`bao-dr-source.example.com`", "Source passthrough endpoint."]},
    {cells: ["Target hostname", "`bao-dr-target.example.com`", "Target passthrough endpoint."]},
    {cells: ["Transit endpoint", "`https://host.k3d.internal`", "Shared trust-services endpoint."]},
    {cells: ["Snapshot bucket", "`openbao-dr-backups`", "Shared RustFS bucket."]},
    {cells: ["Transit key", "`openbao-dr-shared-unseal`", "Shared seal root used by both clusters."]},
  ]}
/>

## Step 1: Bootstrap the three-cluster lab

<CommandBlock
  language="text"
  label="configure"
  title="Run the bootstrap automation or manifests for this baseline"
  code={`The validated bootstrap needs to create and wire:
- one infra cluster for shared trust services
- one source cluster
- one target cluster
- the Gateway API experimental bundle in each cluster
- a dedicated passthrough edge in each cluster
- a shared RustFS instance and bucket
- a shared external OpenBao trust-services endpoint in the infra cluster
- one operator install in the source cluster
- one operator install in the target cluster`}
>
  The exact command is specific to your k3d automation. The lane contract is the resulting topology, not the name of one local helper script.
</CommandBlock>

<Callout type="tip" title="Validated defaults">

The validated local proof used the public signed `edge` images by default:

- `ghcr.io/dc-tec/openbao-operator:edge`
- `ghcr.io/dc-tec/openbao-backup:edge`

</Callout>

## Step 2: Apply the source and target clusters

<CommandBlock
  language="bash"
  label="apply"
  title="Apply the source and target OpenBaoCluster manifests"
  code={`kubectl --context <source-context> apply -f source-openbaocluster.yaml
kubectl --context <target-context> apply -f target-openbaocluster.yaml`}
>
  The source and target manifests must both reference the same Transit endpoint, CA bundle, SNI, and key name. That shared seal root is the invariant the restore event depends on.
</CommandBlock>

## Verify the bootstrap

<CommandBlock
  language="bash"
  label="verify"
  title="Check source and target readiness"
  code={`kubectl --context k3d-openbao-dr-source -n openbaocluster-dr-source \\
  get openbaocluster openbaocluster-dr-source \\
  -o jsonpath='{.status.phase}{"\\n"}{.status.readyReplicas}{"\\n"}{range .status.conditions[*]}{.type}={.status}{" reason="}{.reason}{"\\n"}{end}'

kubectl --context k3d-openbao-dr-target -n openbaocluster-dr-target \\
  get openbaocluster openbaocluster-dr-target \\
  -o jsonpath='{.status.phase}{"\\n"}{.status.readyReplicas}{"\\n"}{range .status.conditions[*]}{.type}={.status}{" reason="}{.reason}{"\\n"}{end}'`}
>
  The important steady-state expectation on both sides is `phase=Running`, `readyReplicas=1`, `Available=True`, `OpenBaoInitialized=True`, and `OpenBaoSealed=False`.
</CommandBlock>

<CommandBlock
  language="bash"
  label="verify"
  title="Check the source and target health endpoints"
  code={`curl -ksS --resolve bao-dr-source.example.com:10443:127.0.0.1 \\
  https://bao-dr-source.example.com:10443/v1/sys/health

curl -ksS --resolve bao-dr-target.example.com:11443:127.0.0.1 \\
  https://bao-dr-target.example.com:11443/v1/sys/health`}
/>

<CommandBlock
  language="bash"
  label="verify"
  title="Verify the pre-restore source and target state"
  code={`SOURCE_TOKEN="$(
  curl -ksS --resolve bao-dr-source.example.com:10443:127.0.0.1 \\
    -H 'Content-Type: application/json' \\
    -d '{"password":"source-demo-password"}' \\
    https://bao-dr-source.example.com:10443/v1/auth/userpass/login/demo-admin \\
  | jq -r '.auth.client_token'
)"

TARGET_TOKEN="$(
  curl -ksS --resolve bao-dr-target.example.com:11443:127.0.0.1 \\
    -H 'Content-Type: application/json' \\
    -d '{"password":"target-demo-password"}' \\
    https://bao-dr-target.example.com:11443/v1/auth/userpass/login/demo-admin \\
  | jq -r '.auth.client_token'
)"

curl -ksS --resolve bao-dr-source.example.com:10443:127.0.0.1 \\
  -H "X-Vault-Token: \${SOURCE_TOKEN}" \\
  https://bao-dr-source.example.com:10443/v1/secret/data/dr-control

curl -ksS --resolve bao-dr-target.example.com:11443:127.0.0.1 \\
  -H "X-Vault-Token: \${TARGET_TOKEN}" \\
  https://bao-dr-target.example.com:11443/v1/secret/data/dr-control`}
>
  The validated lane starts with `phase1-source` on the source side and `phase1-target` on the target side so the restore event can prove that target state was really replaced.
</CommandBlock>

<NextActions
  title="Continue the DR rehearsal"
  items={[
    {
      label: "DR restore runbook",
      description: "Run the destructive restore event once the source and target are both healthy and the pre-restore state is known.",
      docId: "user-guide/validated-deployments/runbooks/cross-cluster-dr-restore-rustfs",
    },
    {
      label: "Reference architecture",
      description: "Review the DR invariants again before you move the source snapshot into the target cluster.",
      docId: "user-guide/validated-deployments/architectures/local/k3d-cross-cluster-dr-transit-rustfs",
    },
    {
      label: "Restore from backup",
      description: "Use the generic restore guide for the operator-wide restore behavior behind this lane-specific runbook.",
      docId: "user-guide/openbaorestore/restore",
    },
  ]}
/>
