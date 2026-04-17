---
title: EKS Hardened / Public ACME
hide_title: true
pageType: concept
journey: validated-deployments
description: Validated hardened cloud baseline for OpenBao on Amazon EKS with AWS KMS auto-unseal, a dedicated public passthrough Gateway, OpenBao-managed ACME, and S3 backups.
---

<PageHeader
  title="Hardened EKS baseline with public ACME"
  lede="This cloud baseline is the hardened EKS topology validated by the project. It uses AWS KMS for auto-unseal, a dedicated public passthrough Gateway, OpenBao-managed ACME, and S3 backups through a separate identity."
/>

<Checklist
    title="Validated coverage"
    items={[
      "a Hardened-profile cluster can bootstrap on EKS with KMS auto-unseal and signed helper images",
      "OpenBao-managed ACME can issue and serve the public certificate while the Gateway remains pure passthrough",
      "JWT bootstrap, admin JWT login, and S3 backups all work under the same hardened cloud posture",
      "the dedicated public edge can stay separate from the terminating admin edge used for the rest of the platform",
    ]}
  />


<Callout type="note" title="Baseline scope">

Cloud reference architecture. This is the production-style Amazon EKS baseline validated by the project.

</Callout>

<DecisionTable
  title="Baseline summary"
  columns={["Surface", "Choice", "Why it matters"]}
  rows={[
    {
      cells: [
        "Profile",
        "`spec.profile: Hardened`",
        "The baseline uses the hardened production-style posture rather than the development baseline defaults.",
      ],
      emphasis: "recommended",
    },
    {
      cells: [
        "Seal path",
        "AWS KMS via workload identity",
        "The main workload uses a cloud-native unseal path rather than static or external secret material.",
      ],
    },
    {
      cells: [
        "TLS model",
        "`spec.tls.mode: ACME`",
        "OpenBao remains the TLS endpoint and owns the public certificate lifecycle directly.",
      ],
    },
    {
      cells: [
        "Edge model",
        "Dedicated public Gateway API passthrough",
        "The hardened hostname stays isolated from the shared terminating admin edge and preserves `tls-alpn-01` behavior.",
      ],
    },
    {
      cells: [
        "Backup path",
        "S3 with a separate backup identity",
        "Backup execution remains separate from KMS unseal and public-edge concerns.",
      ],
    },
  ]}
/>

<DiagramFrame
  title="Baseline topology"
  caption="The hardened hostname lives on its own passthrough edge, OpenBao handles ACME itself, and the cluster still keeps backup and unseal identity surfaces separate."
  code={`flowchart LR
    Client["OpenBao client"] -->|"HTTPS (SNI)"| Edge["Dedicated public passthrough Gateway"]
    Edge -->|"TLS passthrough"| Public["OpenBao public Service / ACME endpoint"]
    Public --> Bao["OpenBao Pods"]

    Shared["Shared admin edge"] -->|"Terminating HTTPS"| AdminTools["ArgoCD / Grafana / Prometheus"]
    Operator["OpenBao Operator"] -->|"JWT bootstrap"| Bao
    Admin["Admin ServiceAccount token"] -->|"JWT login"| Bao
    Bao -->|"AWS KMS"| KMS["AWS KMS key"]
    Backup["Backup Job"] -->|"S3 snapshot upload"| S3["S3 bucket"]
    Cache["Shared ACME cache PVC"] <--> Bao
    Bao -->|"IRSA / workload identity"| MainIAM["Main workload IAM role"]
    Backup -->|"IRSA / workload identity"| BackupIAM["Backup IAM role"]

    classDef read fill:transparent,stroke:#79c0ab,stroke-width:2px,color:#e6f4ef;
    classDef process fill:transparent,stroke:#fdd0a4,stroke-width:2px,color:#e6f4ef;
    classDef write fill:transparent,stroke:#87d6be,stroke-width:2px,color:#e6f4ef;

    class Client,Admin read;
    class Edge,Shared,Operator,Backup process;
    class Public,Bao,KMS,S3,Cache,AdminTools,MainIAM,BackupIAM write;`}
/>

## Why this lane exists

<DecisionTable
  kind="reference"
  title="Key design choices"
  columns={["Choice", "What it protects", "Why it stays in the lane"]}
  rows={[
    {
      cells: [
        "Dedicated passthrough Gateway",
        "The public OpenBao hostname keeps end-to-end TLS ownership inside OpenBao.",
        "The hardened hostname should not inherit the operational compromises of a shared terminating admin edge.",
      ],
      emphasis: "recommended",
    },
    {
      cells: [
        "OpenBao-managed ACME",
        "Certificate issuance stays part of the OpenBao control surface.",
        "The lane is meant to prove the operator plus OpenBao certificate path, not an external certificate controller.",
      ],
    },
    {
      cells: [
        "Shared ACME cache",
        "Multi-replica certificate state remains consistent across Pods.",
        "The lane is not valid for HA ACME without an RWX-capable cache path.",
      ],
    },
    {
      cells: [
        "Separate admin edge",
        "Public ACME reachability does not force the rest of the platform onto the same public exposure contract.",
        "The hardened lane needs this separation to stay operationally realistic.",
      ],
    },
  ]}
/>

<Checklist
  tone="warning"
  title="Baseline requirements"
  items={[
    "keep the hardened hostname publicly reachable on port `443` for ACME validation",
    "keep the public OpenBao hostname on a dedicated passthrough Gateway instead of the shared terminating edge",
    "keep the ACME shared cache on RWX-capable storage for multi-replica safety",
    "keep signed helper images and hardened verification enabled",
    "keep backup and unseal IAM roles separate so the security model you validated is the one you actually operate",
  ]}
/>

<Callout type="success" title="Validated coverage">

The hardened EKS lane covered bootstrap, KMS auto-unseal, OpenBao-managed public ACME certificate issuance, Gateway passthrough, JWT bootstrap, human admin JWT login, and successful S3 backups.

</Callout>

<Callout type="warning" title="Out of scope">

This baseline does not cover source-restricted public hostnames, externally managed TLS, or a terminating Gateway in front of OpenBao. Those choices require a different topology.

</Callout>

<NextActions
  title="Next steps"
  items={[
    {
      label: "Deployment recipe",
      description: "Apply the exact EKS hardened lane with KMS, public ACME, dedicated passthrough, and S3 backup wiring.",
      docId: "user-guide/validated-deployments/recipes/cloud/amazon-eks-hardened-awskms-acme",
    },
    {
      label: "TLS and workload identity",
      description: "Review the generic TLS ownership and workload-identity model behind the lane.",
      docId: "security/workload/tls",
    },
    {
      label: "Platform controls",
      description: "Cross-check the hardened cloud lane against the admission, RBAC, and network controls the rest of the docs recommend.",
      docId: "security/infrastructure/index",
    },
  ]}
/>
