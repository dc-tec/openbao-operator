---
title: k3d Hardened / ACME
hide_title: true
pageType: concept
journey: validated-deployments
description: Validated local baseline for a hardened OpenBao deployment on k3d with Transit auto-unseal, an internal ACME issuer, and user-managed TLS passthrough.
---

<PageHeader
  title="Local hardened lane with private ACME"
  lede="This validated local baseline keeps the hardened posture, keeps the unseal root external, and keeps OpenBao as the TLS endpoint while an internal ACME CA validates certificate issuance through a user-managed passthrough edge."
/>

<Checklist
    title="Validated coverage"
    items={[
      "a Hardened cluster can bootstrap locally while keeping the seal dependency outside the tenant namespace",
      "OpenBao-managed ACME issuance works when the validator reaches the passthrough edge with the expected hostname",
      "Transit auto-unseal and ACME trust material can coexist in one shared trust-services dependency",
      "local rehearsal covers ACME readiness and probe behavior before a public-cloud baseline",
    ]}
  />


<Callout type="note" title="Classification">

This local reference architecture uses k3d to rehearse hardened deployments that keep TLS passthrough in OpenBao and use a private ACME trust chain. It is a local validation lane rather than a production target.

</Callout>

<DecisionTable
  title="Lane summary"
  columns={["Surface", "Choice", "Why it matters"]}
  rows={[
    {
      cells: [
        "Profile",
        "`spec.profile: Hardened`",
        "The lane only matters if the operator enforces the hardened policy surface while ACME and Transit are both active.",
      ],
      emphasis: "recommended",
    },
    {
      cells: [
        "Seal path",
        "Shared external OpenBao Transit provider",
        "The seal root stays outside the cluster, which keeps the lane aligned with a real external dependency model.",
      ],
    },
    {
      cells: [
        "TLS model",
        "`spec.tls.mode: ACME` with an internal ACME CA",
        "OpenBao remains the TLS endpoint while the lane avoids dependence on public ACME during local validation.",
      ],
    },
    {
      cells: [
        "Edge model",
        "User-managed Traefik TCP passthrough",
        "TLS must reach OpenBao untouched for ACME, so the edge must pass traffic through instead of terminating it.",
      ],
    },
    {
      cells: [
        "Validation scope",
        "Local ACME lifecycle coverage plus hardened bootstrap",
        "The baseline covers ACME readiness, trust material, and bootstrap behavior together.",
      ],
    },
  ]}
/>

<DiagramFrame
  title="Baseline topology"
  caption="The same external trust-services dependency supplies both Transit auto-unseal and the private ACME directory, while the ingress layer remains pure passthrough."
  code={`flowchart LR
    Client["OpenBao client"] -->|"HTTPS (SNI)"| Edge["Traefik IngressRouteTCP"]
    Edge -->|"TLS passthrough"| ACMESvc["OpenBao ACME Service"]
    ACMESvc --> Bao["OpenBao Pods"]

    Operator["OpenBao Operator"] -->|"JWT bootstrap"| Bao
    Admin["Admin ServiceAccount token"] -->|"JWT login"| Bao
    Bao -->|"Transit encrypt/decrypt"| Trust["Shared trust-services OpenBao"]
    Bao -->|"ACME directory + tls-alpn-01"| Trust
    DNS["ACME hostname resolution"] --> Edge

    classDef read fill:transparent,stroke:#79c0ab,stroke-width:2px,color:#e6f4ef;
    classDef process fill:transparent,stroke:#fdd0a4,stroke-width:2px,color:#e6f4ef;
    classDef write fill:transparent,stroke:#87d6be,stroke-width:2px,color:#e6f4ef;

    class Client,Admin read;
    class Edge,Operator,DNS process;
    class ACMESvc,Bao,Trust write;`}
/>

## Why this lane exists

<DecisionTable
  kind="reference"
  title="Key design choices"
  columns={["Choice", "What it protects", "Why it stays in the lane"]}
  rows={[
    {
      cells: [
        "Private ACME issuer",
        "The lane can prove ACME behavior without assuming public DNS and internet reachability.",
        "Local hardened rehearsal should prove OpenBao-managed issuance, not public CA operations.",
      ],
      emphasis: "recommended",
    },
    {
      cells: [
        "Hostname resolves back to the passthrough edge",
        "The validator actually reaches the endpoint OpenBao serves for `tls-alpn-01`.",
        "Successful name resolution is the invariant; the local CoreDNS rewrite is only one implementation of it.",
      ],
    },
    {
      cells: [
        "Shared trust-services dependency",
        "Transit and ACME both depend on the same external trust boundary.",
        "This keeps the local lane close to the production-style trust split without requiring cloud services.",
      ],
    },
    {
      cells: [
        "Passthrough stays user-managed",
        "OpenBao remains the TLS endpoint.",
        "The lane intentionally avoids making shared terminating ingress behavior part of the hardened ACME contract.",
      ],
    },
  ]}
/>

<Checklist
  tone="warning"
  title="Stay on the validated path"
  items={[
    "keep `spec.profile: Hardened` and keep the trust-services endpoint reachable for both Transit and ACME",
    "keep the ACME hostname resolving back to the passthrough edge from the validating environment",
    "keep the passthrough route targeting the dedicated `-acme` Service on port `443`",
    "mount both the Transit CA bundle and the ACME issuer CA bundle in the Secret expected by the lane",
    "disable AppArmor only when the local runtime forces it and treat that as a local node concession",
  ]}
/>

<Callout type="success" title="Validated coverage">

The validated local lane exercised hardened bootstrap with self-init, Transit auto-unseal through shared trust services, OpenBao-managed ACME issuance from a private CA, human admin JWT login, and external access over user-managed passthrough.

</Callout>

<Callout type="warning" title="Out of scope">

This lane does not prove public ACME behavior or replace a cloud ingress baseline. It is a local rehearsal environment for the hardened ACME control path.

</Callout>

<NextActions
  title="Use the lane"
  items={[
    {
      label: "Deployment recipe",
      description: "Apply the exact steps that reproduce the validated hardened ACME lane in the local environment.",
      docId: "user-guide/validated-deployments/recipes/local/hardened-transit-acme-tls",
    },
    {
      label: "TLS and workload identity",
      description: "Review the generic TLS ownership and identity model behind the lane.",
      docId: "security/workload/tls",
    },
    {
      label: "EKS Hardened",
      description: "Compare the local private-ACME rehearsal path with the public-ACME cloud baseline.",
      docId: "user-guide/validated-deployments/architectures/cloud/amazon-eks-hardened-awskms-acme",
    },
  ]}
/>
