---
title: k3d Cross-Cluster DR
hide_title: true
pageType: concept
journey: validated-deployments
description: Validated local disaster-recovery baseline for OpenBao on k3d with a shared Transit seal root, RustFS snapshot transfer, and manual cutover across source and target clusters.
---

<PageHeader
  title="Local cross-cluster DR baseline"
  lede="This local DR baseline keeps the source, target, and shared trust services separated so backup, restore, unseal, and cutover all cross the same kinds of boundaries used in a real disaster-recovery event."
/>

<Checklist
    title="Validated coverage"
    items={[
      "a snapshot can leave the source cluster, cross an object-storage boundary, and restore into a different target cluster",
      "the restored target can unseal only because it shares the same external Transit root of trust as the source",
      "restore verification can confirm both credential cutover and data cutover before any manual failover happens",
      "the operator's backup and restore workflows still work when source and target are split across real ingress and storage boundaries",
    ]}
  />


<Callout type="note" title="Baseline scope">

This local disaster-recovery reference architecture uses k3d to validate the DR invariants for backup, restore, unseal, and manual cutover before moving to a cloud recovery pair.

</Callout>

<DecisionTable
  title="Baseline summary"
  columns={["Surface", "Choice", "Why it matters"]}
  rows={[
    {
      cells: [
        "Cluster split",
        "One infra cluster, one source cluster, one target cluster",
        "Restore crosses a real cluster boundary instead of staying inside one namespace or one API server.",
      ],
      emphasis: "recommended",
    },
    {
      cells: [
        "Seal path",
        "Shared external Transit key",
        "The target can only unseal restored data because it shares the same external seal root of trust as the source.",
      ],
    },
    {
      cells: [
        "Transfer boundary",
        "RustFS S3-compatible bucket",
        "Snapshots move through a real object-storage boundary that both clusters can reach independently.",
      ],
    },
    {
      cells: [
        "Edge model",
        "Dedicated passthrough endpoints for infra, source, and target",
        "The lane validates real ingress reachability without collapsing TLS termination into a single local shortcut.",
      ],
    },
    {
      cells: [
        "Cutover model",
        "Manual restore and manual client or DNS cutover",
        "The baseline covers restore correctness and does not include automatic failover orchestration.",
      ],
    },
  ]}
/>

<DiagramFrame
  title="Baseline topology"
  caption="The source cluster writes snapshots to shared storage, the target cluster restores from that storage, and both sides depend on the same external Transit key to make restored data usable."
  code={`flowchart LR
    Client["Operator or admin"] -->|"HTTPS (SNI)"| SourceEdge["Source passthrough edge"]
    Client -->|"HTTPS (SNI)"| TargetEdge["Target passthrough edge"]

    SourceEdge -->|"TLS passthrough"| SourceBao["Source OpenBao"]
    TargetEdge -->|"TLS passthrough"| TargetBao["Target OpenBao"]

    SourceOp["Source Operator"] -->|"backup orchestration"| SourceBao
    TargetOp["Target Operator"] -->|"restore orchestration"| TargetBao
    SourceBao -->|"snapshot upload"| RustFS["Shared RustFS bucket"]
    RestoreJob["Restore Job"] -->|"snapshot download"| RustFS
    RestoreJob -->|"snapshot restore"| TargetBao

    SourceBao -->|"Transit encrypt/decrypt"| Trust["Shared Transit provider"]
    TargetBao -->|"Transit encrypt/decrypt"| Trust
    InfraEdge["Infra passthrough edge"] -->|"TLS passthrough"| Trust

    classDef read fill:transparent,stroke:#79c0ab,stroke-width:2px,color:#e6f4ef;
    classDef process fill:transparent,stroke:#fdd0a4,stroke-width:2px,color:#e6f4ef;
    classDef write fill:transparent,stroke:#87d6be,stroke-width:2px,color:#e6f4ef;

    class Client,RestoreJob read;
    class SourceEdge,TargetEdge,InfraEdge,SourceOp,TargetOp process;
    class SourceBao,TargetBao,Trust,RustFS write;`}
/>

## Why this lane exists

<DecisionTable
  kind="reference"
  title="Key design choices"
  columns={["Choice", "What it protects", "Why it stays in the lane"]}
  rows={[
    {
      cells: [
        "Shared seal root",
        "Restored data is still decryptable after it lands in the target cluster.",
        "This is the core DR invariant. Without a shared external seal root, the target can restore bits and still fail to unseal.",
      ],
      emphasis: "recommended",
    },
    {
      cells: [
        "Separate infra cluster for trust services",
        "The shared Transit dependency stays independent from source and target failure domains.",
        "The lane should treat trust services as an external dependency, not as part of either OpenBao cluster.",
      ],
    },
    {
      cells: [
        "Shared RustFS bucket",
        "The restore uses the same object-transfer shape as a real remote-storage path.",
        "The lane exercises snapshot handoff through shared object storage instead of a local disk copy.",
      ],
    },
    {
      cells: [
        "Manual cutover",
        "Operators verify the target before traffic moves.",
        "The lane proves correctness and operator workflow, not automated failover policy.",
      ],
      emphasis: "caution",
    },
  ]}
/>

<Checklist
  tone="warning"
  title="Baseline requirements"
  items={[
    "keep the source and target on the same OpenBao version for the restore event",
    "keep the source and target pointed at the same Transit address, CA bundle, SNI, and key name",
    "keep shared object storage reachable from both clusters before you start a backup or restore",
    "keep the target cluster created ahead of time with restore auth configured",
    "perform cutover only after credential and data verification succeeds on the restored target",
  ]}
/>

<Callout type="success" title="Validated coverage">

The local DR lane proved source backup to RustFS, restore into a separate target cluster, target unseal with the shared Transit key, and post-restore checks that source credentials and source data replaced the target bootstrap state.

</Callout>

<Callout type="warning" title="Out of scope">

This baseline does not define automatic failover or a cloud DR reference. It covers a validated manual recovery flow with explicit preconditions for backup, restore, and cutover.

</Callout>

<NextActions
  title="Next steps"
  items={[
    {
      label: "Bootstrap recipe",
      description: "Stand up the infra, source, and target clusters that make up the DR validation environment.",
      docId: "user-guide/validated-deployments/recipes/local/k3d-cross-cluster-dr-bootstrap",
    },
    {
      label: "DR restore runbook",
      description: "Run the destructive restore workflow and verify that source state replaced target state cleanly.",
      docId: "user-guide/validated-deployments/runbooks/cross-cluster-dr-restore-rustfs",
    },
    {
      label: "Restore from backup",
      description: "Review the generic restore behavior behind the lane-specific runbook.",
      docId: "user-guide/openbaorestore/restore",
    },
  ]}
/>
