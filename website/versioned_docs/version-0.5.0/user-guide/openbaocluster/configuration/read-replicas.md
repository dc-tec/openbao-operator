---
title: Read Replicas
slug: /configure/read-replicas
hide_title: true
pageType: task
journey: configure
description: Configure steady-state non-voter read replicas, their endpoint model, and the status and lifecycle signals that keep the pool operationally safe.
---

<PageHeader
  title="Configure read replicas"
  lede="Read replicas add a steady-state non-voter pool for read scaling and placement isolation. The operator keeps the main client endpoint stable, can create a dedicated read Service, and stages the pool deliberately during destructive workflows."
/>

<DecisionTable
  title="What enabling read replicas changes"
  columns={["Surface", "Operator behavior", "Why it matters"]}
  rows={[
    {
      cells: [
        "Workload topology",
        "Creates a second StatefulSet for the steady-state non-voter read pool.",
        "The read pool becomes a first-class workload tier with its own lifecycle, storage, and template overrides.",
      ],
      emphasis: "recommended",
    },
    {
      cells: [
        "Primary client endpoint",
        "Keeps the main client Service shared across voter and read-replica Pods.",
        "Clients can stay on one hostname while OpenBao handles standby or non-voter read traffic and forwards leader-only operations as needed.",
      ],
    },
    {
      cells: [
        "Dedicated read endpoint",
        "Optionally creates a separate read Service that selects only steady read replicas.",
        "Use it when a workload should opt into explicit read offload instead of relying on the shared primary endpoint.",
      ],
    },
    {
      cells: [
        "Destructive workflows",
        "Stages the steady read pool down for restore and blue-green cutover, then restores it before the workflow completes.",
        "This keeps peer removal, promotion, and restore semantics deterministic instead of mixing steady non-voters into destructive phases.",
      ],
    },
  ]}
/>

<DiagramFrame
  title="Endpoint model for read replicas"
  caption="The main endpoint stays stable and can reach any client-serving Pod. The optional read Service exists for explicit consumers that want only the read pool."
  code={`flowchart LR
    Client["Client"] --> Main["<cluster>-public"]
    Client --> Read["<cluster>-read (optional)"]
    Main --> Voters["Voter Pods"]
    Main --> Reads["Read-replica Pods"]
    Read --> Reads
    Reads --> Leader["OpenBao forwards writes to the leader"]

    classDef read fill:transparent,stroke:#79c0ab,stroke-width:2px,color:#e6f4ef;
    classDef process fill:transparent,stroke:#fdd0a4,stroke-width:2px,color:#e6f4ef;
    classDef write fill:transparent,stroke:#87d6be,stroke-width:2px,color:#e6f4ef;

    class Client read;
    class Main,Read process;
    class Voters,Reads,Leader write;`}
/>

<Callout type="note" title="The primary endpoint stays shared by default">

OpenBao `2.5.x` supports standby and non-voter read scaling together with request forwarding. The operator follows that contract by keeping the primary endpoint attached to the shared client Service instead of trying to classify requests at the edge. A separate read Service is still available when you want an explicit read-pool consumer path.

</Callout>

## Configure the read pool

<CommandBlock
  language="yaml"
  label="configure"
  title="Add a steady-state read-replica pool"
  code={`spec:
  replicas: 3
  readReplicas:
    replicas: 2
    service:
      enabled: true
      type: ClusterIP
    template:
      metadata:
        labels:
          workload.openbao.org/tier: read
      resources:
        requests:
          cpu: "250m"
          memory: "512Mi"
        limits:
          cpu: "1"
          memory: "1Gi"
      scheduling:
        topologySpreadConstraints:
          - maxSkew: 1
            topologyKey: topology.kubernetes.io/zone
            whenUnsatisfiable: ScheduleAnyway
            labelSelector:
              matchLabels:
                openbao.org/cluster: prod-cluster
        tolerations:
          - key: "workload"
            operator: "Equal"
            value: "read"
            effect: "NoSchedule"
    storage:
      size: 2Gi
      storageClassName: fast-ssd`}
>
  Use the read pool when you want a steady non-voter tier. `template` and `storage` overrides apply only to the read StatefulSet. Cluster-wide concerns such as TLS, unseal, plugins, and audit remain shared with the voter pool.
</CommandBlock>

<Callout type="warning" title="Read-pool storage stays validated against the voter pool">

Read-replica storage is intentionally narrower than a full second cluster spec. The operator rejects a read-pool PVC request that is smaller than the voter storage request, and an effective read `storageClassName` becomes immutable once read PVCs exist.

</Callout>

## Decide how clients should use the endpoints

<DecisionTable
  kind="reference"
  title="Client endpoint choices"
  columns={["Endpoint", "Use it when", "What to expect"]}
  rows={[
    {
      cells: [
        "Primary endpoint",
        "Most clients should keep using the existing cluster address.",
        "Requests can land on voters or read replicas. OpenBao then decides whether to serve the request on the target node or forward it to the active leader.",
      ],
      emphasis: "recommended",
    },
    {
      cells: [
        "Dedicated read Service",
        "You want explicit read offload, separate SLOs, or an endpoint that must never land on the voter pool directly.",
        "Traffic reaches only the steady read StatefulSet. Write-class requests still rely on OpenBao forwarding semantics rather than Kubernetes request inspection.",
      ],
    },
  ]}
/>

<Callout type="tip" title="Do not try to split reads and writes at the edge">

The operator does not inspect HTTP methods or paths to decide which Pod should receive a request. Gateway API, Ingress, or a load balancer should send traffic to the chosen Service, and OpenBao should handle standby-read and request-forwarding behavior itself.

</Callout>

## Watch the read-pool status separately

<DecisionTable
  kind="reference"
  title="Read-replica conditions"
  columns={["Condition", "What it proves", "Typical next move"]}
  rows={[
    {
      cells: [
        "`ReadReplicasReady`",
        "The read StatefulSet has the expected number of Ready Pods.",
        "If false, inspect the read StatefulSet, read Pod events, and any template or storage overrides.",
      ],
      emphasis: "recommended",
    },
    {
      cells: [
        "`ReadServingAvailable`",
        "At least one Ready read replica is actually serving reads.",
        "If false, inspect `/sys/health` on the read Pods and verify the pool is unsealed and has joined correctly.",
      ],
    },
    {
      cells: [
        "`RaftMembershipReady`",
        "The expected steady non-voter peers are registered in Raft membership.",
        "If false, inspect membership and safe scale-down behavior before assuming the topology is stable.",
      ],
    },
    {
      cells: [
        "`ReadReplicasAutopilotHealthy`",
        "Autopilot considers the steady read-replica peers healthy from the integrated-storage timing perspective.",
        "If false, inspect Autopilot state, inter-node latency, and the read pool placement before assuming the pool is a good fit for that topology. If `Unknown`, confirm the `openbao-operator` policy can `read` `sys/storage/raft/autopilot/state`.",
      ],
    },
    {
      cells: [
        "`ReadReplicaStorageConfigured`",
        "The read-pool PVC surface matches the declared storage contract.",
        "If false, inspect read PVC provisioning, binding, and the effective storage class or size.",
      ],
    },
  ]}
/>

<Callout type="note" title="Storage status is pool-aware">

`status.readReplicas.storage` records the read-pool PVC state separately from the voter pool. The existing top-level storage condition remains voter-scoped so its meaning does not change under existing automation.

</Callout>

<Callout type="note" title="Read-pool health and read-pool usefulness are different questions">

`ReadReplicasReady`, `ReadServingAvailable`, and `RaftMembershipReady` prove that the operator-created
topology converged. `ReadReplicasAutopilotHealthy` adds a stricter integrated-storage view of whether
those non-voters are healthy under the current timing and replication conditions.

</Callout>

<Callout type="warning" title="There is no published RTT limit for read replicas">

OpenBao does not publish a hard maximum network-latency number for steady non-voters. Cross-zone or
cross-region placement can work, but it is an environment-specific validation question rather than a
fixed supported RTT budget. Validate the actual placement with your own latency, replication, and
Autopilot checks instead of relying on a generic rule of thumb.

</Callout>

<Callout type="tip" title="Healthy-cluster request behavior may still rely on forwarding">

In local validation, healthy-cluster reads against read replicas still relied on OpenBao request-forwarding
behavior for the tested KV path. Treat the shared primary endpoint and dedicated read Service as endpoint
selection choices, not as a promise that every read is served locally from the target Pod.

</Callout>

<Callout type="warning" title="Do not assume no-quorum read continuity">

Local validation under a stricter leader-voter partition plus added cross-node latency produced
request stalls and timeouts rather than a clean “reads continue locally” outcome. Treat no-quorum
read behavior as environment-specific and validate it directly in the failure topology you care
about before you rely on it for continuity planning.

</Callout>

## Know the day-2 behavior before enabling the pool

<DecisionTable
  kind="reference"
  title="Operational behavior"
  columns={["Workflow", "What the operator does", "Why it is deliberate"]}
  rows={[
    {
      cells: [
        "Scale up or change template",
        "Reconciles the read StatefulSet independently from the voter StatefulSet.",
        "Read-pool rollout and voter rollout do not need to mutate together.",
      ],
      emphasis: "recommended",
    },
    {
      cells: [
        "Disable `spec.readReplicas`",
        "Removes steady non-voters safely, scales the read StatefulSet to zero, then deletes the read StatefulSet, read Service, and read ConfigMap.",
        "The topology change is non-destructive to retained PVCs, so disabling the feature does not silently delete storage.",
      ],
    },
    {
      cells: [
        "Rolling upgrade",
        "Upgrades the read pool before allowing voter partition rollout to proceed.",
        "This preserves read continuity and keeps voter-side disruption behind a converged read pool.",
      ],
    },
    {
      cells: [
        "Blue-green upgrade or restore",
        "Stages steady read replicas down first, performs the destructive work, restores the read pool, and only then marks the workflow complete.",
        "Steady non-voters are kept out of promotion and restore-critical phases so membership cleanup and completion semantics remain deterministic.",
      ],
    },
  ]}
/>

<NextActions
  title="Continue the rollout design"
  items={[
    {
      label: "External access",
      description: "See how the shared primary endpoint and optional read Service fit Gateway, Ingress, and direct Service exposure.",
      docId: "user-guide/openbaocluster/configuration/external-access",
    },
    {
      label: "Gateway API",
      description: "Review how the current Gateway integration keeps one route on the shared client Service.",
      docId: "user-guide/openbaocluster/configuration/gateway-api",
    },
    {
      label: "Plan upgrades",
      description: "Understand how rolling and blue-green workflows stage or restore the steady read pool.",
      docId: "user-guide/openbaocluster/operations/upgrades",
    },
    {
      label: "Restore from backup",
      description: "See how restore drains the steady read pool before the restore Job and restores it before completion.",
      docId: "user-guide/openbaorestore/restore",
    },
  ]}
/>
