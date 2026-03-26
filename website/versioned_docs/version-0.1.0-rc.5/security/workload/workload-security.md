---
title: Pod and Runtime Security
hide_title: true
pageType: concept
journey: security
description: Pod hardening, projected-token handling, and runtime guardrails for operator-managed OpenBao workloads and lifecycle Jobs.
---

<PageHeader
  title="Keep OpenBao Pods inside the restricted runtime baseline from the first reconcile."
  lede="The operator assumes runtime hardening is the default, not an optional add-on. OpenBao Pods, init containers, and transient lifecycle Jobs are expected to run non-root, use explicit writable volumes, and consume only the identities and Linux privileges they actually need."
/>

<DecisionTable
  title="Runtime protections at a glance"
  columns={["Control", "Default posture", "Why it matters"]}
  rows={[
    {
      cells: [
        "Non-root execution",
        "OpenBao Pods and helper containers run with `runAsNonRoot` and a stable UID/GID baseline on standard Kubernetes.",
        "This narrows the blast radius of container breakout attempts and keeps the workload aligned with the Restricted Pod Security Standard.",
      ],
      emphasis: "recommended",
    },
    {
      cells: [
        "Read-only root filesystem",
        "The root filesystem is immutable and mutable paths are mounted explicitly.",
        "The workload cannot silently write arbitrary state into the container image layer.",
      ],
    },
    {
      cells: [
        "Dropped Linux capabilities and seccomp",
        "Capabilities are dropped and `RuntimeDefault` seccomp applies by default.",
        "The process keeps only the syscall and privilege surface needed to run the service.",
      ],
    },
    {
      cells: [
        "Explicit projected Kubernetes token",
        "The pod does not rely on default token automounting and uses a projected token only where the workload needs Kubernetes API access.",
        "This keeps API identity explicit, short-lived, and absent from containers that do not need it.",
      ],
    },
    {
      cells: [
        "Job resource guardrails",
        "Backup and restore Jobs run with explicit resource requests and limits.",
        "A lifecycle job should not starve the steady-state OpenBao Pods or neighboring workloads on the same node.",
      ],
    },
  ]}
/>

<DiagramFrame
  title="Runtime boundary inside an operator-managed Pod"
  caption="Mutable state, API identity, and rendered configuration are each introduced through explicit mounts instead of through an implicitly writable container image."
  code={`flowchart LR
    Controller["Controller"] --> StatefulSet["StatefulSet template"]
    StatefulSet --> Init["Config init container"]
    StatefulSet --> Bao["OpenBao container"]
    StatefulSet --> Volumes["Writable volumes"]
    StatefulSet --> Token["Projected API token"]
    StatefulSet --> Config["Rendered config.hcl"]

    Init --> Config
    Bao --> Volumes
    Bao --> Token
    Bao --> Config

    classDef read fill:transparent,stroke:#79c0ab,stroke-width:2px,color:#e6f4ef;
    classDef write fill:transparent,stroke:#87d6be,stroke-width:2px,color:#e6f4ef;
    classDef process fill:transparent,stroke:#fdd0a4,stroke-width:2px,color:#e6f4ef;

    class Controller,StatefulSet process;
    class Init,Token,Config read;
    class Bao,Volumes write;`}
/>

## Pod hardening baseline

<DecisionTable
  kind="reference"
  title="Baseline pod controls"
  columns={["Surface", "Expected setting", "Operational note"]}
  rows={[
    {
      cells: [
        "User and group",
        "Non-root by default on standard Kubernetes; OpenShift may assign UID/GID through SCC.",
        "Avoid assuming a fixed platform-level UID unless you intentionally override `spec.securityContext`.",
      ],
      emphasis: "recommended",
    },
    {
      cells: [
        "Filesystem",
        "Read-only root filesystem with explicit writable mounts for data, logs, and temporary paths.",
        "If a plugin or sidecar needs extra write paths, treat that as a deliberate exception to review.",
      ],
    },
    {
      cells: [
        "Privilege escalation",
        "`allowPrivilegeEscalation: false` with dropped capabilities.",
        "Setuid or unexpected ambient privileges should not become a recovery crutch.",
      ],
    },
    {
      cells: [
        "Seccomp",
        "`RuntimeDefault`.",
        "If your platform cannot support this baseline, resolve the platform issue rather than weakening the cluster silently.",
      ],
    },
  ]}
/>

<Callout type="note" title="Init container behavior">

The config-rendering init container inherits the same pod-level hardening contract and does not receive a Kubernetes API token mount by default. Its job is to render dynamic configuration such as Pod IP and hostname into `config.hcl`, not to act as a privileged bootstrap helper.

</Callout>

## Identity and token exposure

<DecisionTable
  kind="reference"
  title="Where runtime identity exists"
  columns={["Path", "How it works", "Why it stays narrow"]}
  rows={[
    {
      cells: [
        "OpenBao Pod token",
        "A projected ServiceAccount token is mounted only where the OpenBao container needs Kubernetes API access for peer discovery and service registration.",
        "The pod does not inherit a default automounted token, so identity is explicit rather than ambient.",
      ],
      emphasis: "recommended",
    },
    {
      cells: [
        "Init container",
        "No default token mount.",
        "Rendering config is not a reason to grant Kubernetes API identity.",
      ],
    },
    {
      cells: [
        "Backup, restore, and upgrade Jobs",
        "Each workflow uses its own operator-managed ServiceAccount and projected token path.",
        "Lifecycle Jobs should not reuse the long-running workload identity or borrow permissions accidentally.",
      ],
    },
  ]}
/>

## Namespace and job guardrails

<DecisionTable
  kind="reference"
  title="Controls around the workload"
  columns={["Control", "What it does", "Why it matters"]}
  rows={[
    {
      cells: [
        "Tenant namespace PSS labels",
        "Provisioned namespaces are labeled for Restricted Pod Security enforcement, audit, and warning.",
        "Insecure sidecars or helper workloads should fail at the namespace boundary rather than weakening the cluster quietly.",
      ],
      emphasis: "recommended",
    },
    {
      cells: [
        "Job resource defaults",
        "Backup and restore Jobs have explicit CPU and memory requests and limits.",
        "Operational workflows remain bounded and do not become an easy noisy-neighbor path.",
      ],
    },
    {
      cells: [
        "Operator-owned writable paths",
        "Mutable state is confined to PVCs, temporary volumes, and generated files the operator expects.",
        "The runtime contract stays reviewable because writes happen on known surfaces.",
      ],
    },
  ]}
/>

<Callout type="warning" title="Avoid runtime exceptions as a convenience">

If a deployment needs extra host access, extra capabilities, or a writable root filesystem, treat that as a security design change. In practice it usually means the surrounding platform integration should be fixed instead of weakening the OpenBao workload contract.

</Callout>

<NextActions
  title="Continue workload protections"
  items={[
    {
      label: "TLS and identity",
      description: "Review how peer trust, certificate rotation, and edge exposure build on top of this runtime baseline.",
      docId: "security/workload/tls",
    },
    {
      label: "Supply-chain verification",
      description: "See how the operator decides which images it will trust before these Pods ever start.",
      docId: "security/workload/supply-chain",
    },
    {
      label: "Tenant isolation",
      description: "Connect namespace hardening back to the shared-service isolation model.",
      docId: "security/multi-tenancy/tenant-isolation",
    },
  ]}
/>
