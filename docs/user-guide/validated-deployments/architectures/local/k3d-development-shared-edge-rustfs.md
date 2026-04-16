---
title: k3d Development / Shared Edge
hide_title: true
pageType: concept
journey: validated-deployments
description: Validated local baseline for a development-profile OpenBao deployment on k3d with operator-managed TLS, a shared terminating edge, RustFS backups, and blue/green upgrades.
---

<PageHeader
  title="Local development lane with shared edge and RustFS"
  lede="This validated local baseline covers development work on k3d with a shared terminating edge, operator-managed TLS, RustFS backups, and blue/green upgrades. Use it to exercise the cluster lifecycle with a low-friction local topology."
/>

<Checklist
    title="Validated coverage"
    items={[
      "a Development-profile cluster can bootstrap cleanly on k3d without extra cloud dependencies",
      "the shared terminating edge can front OpenBao alongside the rest of the local platform toolchain",
      "operator-managed TLS, self-init, JWT bootstrap, and RustFS backups can coexist in one repeatable lane",
      "blue/green upgrades can be rehearsed locally before moving to a hardened or cloud baseline",
    ]}
  />


<Callout type="note" title="Classification">

This local reference architecture uses k3d as a development environment for operator bring-up, UI checks, backup rehearsal, and upgrade behavior. It is a validated local lane rather than a production target.

</Callout>

<DecisionTable
  title="Lane summary"
  columns={["Surface", "Choice", "Why it matters"]}
  rows={[
    {
      cells: [
        "Profile",
        "`spec.profile: Development`",
        "The lane is intentionally optimized for speed and validation coverage, not a production-ready posture.",
      ],
      emphasis: "recommended",
    },
    {
      cells: [
        "TLS model",
        "`spec.tls.mode: OperatorManaged`",
        "The operator owns the internal server certificate path so the lane can stay self-contained.",
      ],
    },
    {
      cells: [
        "Edge model",
        "Shared terminating Gateway API edge",
        "The same local edge can front OpenBao, ArgoCD, Grafana, and other tools without a dedicated passthrough stack.",
      ],
    },
    {
      cells: [
        "Backup target",
        "RustFS via the S3-compatible API",
        "The baseline covers snapshot behavior against a real object-storage boundary without cloud credentials.",
      ],
    },
    {
      cells: [
        "Upgrade model",
        "`upgrade.strategy: BlueGreen`",
        "The lane doubles as a low-risk rehearsal environment for upgrade orchestration and cutover behavior.",
      ],
    },
  ]}
/>

<DiagramFrame
  title="Baseline topology"
  caption="The shared terminating edge stays simple, the operator owns TLS and bootstrap, and the backup path leaves the cluster through a separate RustFS boundary."
  code={`flowchart LR
    Client["Operator or admin"] -->|"HTTPS"| Edge["Shared Gateway API edge"]
    Edge -->|"Re-encrypted HTTPS"| Public["OpenBao public Service"]
    Public --> Bao["OpenBao Pods"]

    Operator["OpenBao Operator"] -->|"JWT bootstrap"| Bao
    Admin["Admin ServiceAccount token"] -->|"JWT login"| Bao
    Demo["Optional demo userpass login"] --> Bao
    Backup["Backup Job"] -->|"S3-compatible snapshots"| RustFS["RustFS bucket"]
    Upgrade["Blue/green upgrade flow"] --> Bao

    classDef read fill:transparent,stroke:#79c0ab,stroke-width:2px,color:#e6f4ef;
    classDef process fill:transparent,stroke:#fdd0a4,stroke-width:2px,color:#e6f4ef;
    classDef write fill:transparent,stroke:#87d6be,stroke-width:2px,color:#e6f4ef;

    class Client,Admin,Demo read;
    class Edge,Operator,Backup,Upgrade process;
    class Public,Bao,RustFS write;`}
/>

## Why this lane exists

<DecisionTable
  kind="reference"
  title="Key design choices"
  columns={["Choice", "What it optimizes", "Why it stays in the lane"]}
  rows={[
    {
      cells: [
        "Shared terminating edge",
        "Fast local bring-up with one ingress surface for multiple tools.",
        "This keeps the lane practical for day-to-day operator work and avoids dedicating a separate passthrough edge to a dev profile.",
      ],
      emphasis: "recommended",
    },
    {
      cells: [
        "RustFS for backups",
        "A real S3-compatible transfer boundary with no cloud dependency.",
        "The lane exercises snapshot upload and retention behavior against an S3-compatible API.",
      ],
    },
    {
      cells: [
        "Blue/green upgrades",
        "Upgrade behavior can be rehearsed locally before a hardened rollout.",
        "The development lane is where you want fast iteration on rollout logic and status transitions.",
      ],
    },
    {
      cells: [
        "Optional demo login",
        "UI access stays easy during development and demos.",
        "It is a convenience for local validation only and should never be mistaken for a production auth pattern.",
      ],
      emphasis: "caution",
    },
  ]}
/>

<Checklist
  tone="warning"
  title="Stay on the validated path"
  items={[
    "keep `spec.profile: Development` and do not treat the lane as a production hardening reference",
    "keep the shared terminating Gateway in front of the cluster instead of switching to passthrough mid-lane",
    "keep the RustFS endpoint reachable from the tenant namespace and use a dedicated backup Secret",
    "treat the demo `userpass` login as local-only convenience and remove it from any shared environment",
    "disable AppArmor only when the local runtime requires it and document that as a node limitation, not a preferred default",
  ]}
/>

<Callout type="success" title="Validated coverage">

The local development lane exercised self-init bootstrap, JWT admin login, optional demo UI login, shared-edge exposure, scheduled and manual backup behavior to RustFS, and blue/green upgrade rehearsal.

</Callout>

<Callout type="warning" title="Out of scope">

This lane does not define a production reference or a hardened security posture. It is a local validation environment for shared-edge routing, backup behavior, and upgrade rehearsal.

</Callout>

<NextActions
  title="Use the lane"
  items={[
    {
      label: "Deployment recipe",
      description: "Reproduce the exact development lane with tenant onboarding, self-init, shared-edge exposure, and RustFS backups.",
      docId: "user-guide/validated-deployments/recipes/local/development-self-init-userpass",
    },
    {
      label: "Backup operations",
      description: "Review the generic backup model behind the RustFS portion of this validated lane.",
      docId: "user-guide/openbaocluster/operations/backups",
    },
    {
      label: "Get Started",
      description: "Use the operator onboarding path when you need the product-wide default workflow instead of a validated local lane.",
      docId: "user-guide/index",
    },
  ]}
/>
