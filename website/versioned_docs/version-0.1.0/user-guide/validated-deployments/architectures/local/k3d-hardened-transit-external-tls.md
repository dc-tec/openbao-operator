---
title: k3d Hardened / External TLS
hide_title: true
pageType: concept
journey: validated-deployments
description: Validated local baseline for a hardened OpenBao deployment on k3d with Transit auto-unseal, external TLS Secrets, and user-managed passthrough access.
---

<PageHeader
  title="Use this lane to rehearse a hardened deployment with external certificates and a separate unseal root."
  lede="This local baseline is the closest validated rehearsal path to a hardened deployment that keeps TLS outside the operator, keeps the seal dependency external, and exposes OpenBao through user-managed TCP passthrough instead of a shared terminating edge."
/>

<Checklist
    title="This lane proves"
    items={[
      "a Hardened cluster can bootstrap locally without falling back to operator-managed TLS or static unseal",
      "Transit auto-unseal can stay outside the cluster while self-init and JWT bootstrap still succeed",
      "externally managed TLS Secrets and passthrough traffic can remain separate from the operator's edge integration model",
      "the local environment can rehearse a production-style trust split without pretending k3d itself is the production target",
    ]}
  />


<Callout type="note" title="Classification">

Local reference architecture. k3d is not the production target, but this lane is the closest validated local analogue to a hardened deployment with an external seal provider and externally managed certificates.

</Callout>

<DecisionTable
  title="Lane summary"
  columns={["Surface", "Choice", "Why it matters"]}
  rows={[
    {
      cells: [
        "Profile",
        "`spec.profile: Hardened`",
        "The lane is valuable only if the operator enforces the production-style posture and rejects the relaxed development shortcuts.",
      ],
      emphasis: "recommended",
    },
    {
      cells: [
        "Seal path",
        "External Transit provider",
        "The unseal root stays outside the cluster so the lane tests a real external dependency instead of collapsing back into static secrets.",
      ],
    },
    {
      cells: [
        "TLS model",
        "`spec.tls.mode: External` with externally provisioned Secrets",
        "Certificate lifecycle stays separate from the operator, which is the point of the lane.",
      ],
    },
    {
      cells: [
        "Edge model",
        "User-managed Traefik TCP passthrough",
        "The validated path keeps passthrough isolated from any shared terminating listener and treats edge routing as a deliberate external dependency.",
      ],
    },
    {
      cells: [
        "Validation scope",
        "Local validation environment plus hardened E2E lifecycle coverage",
        "The lane proves bootstrap, unseal, admin JWT login, and passthrough access in a repeatable local rehearsal path.",
      ],
    },
  ]}
/>

<DiagramFrame
  title="Validated lane topology"
  caption="The lane keeps the unseal root external, keeps TLS external, and keeps the passthrough path user-managed. That separation is the reason this local baseline is useful."
  code={`flowchart LR
    Client["OpenBao client"] -->|"HTTPS (SNI)"| Edge["Traefik IngressRouteTCP"]
    Edge -->|"TLS passthrough"| Public["OpenBao public Service"]
    Public --> Bao["OpenBao Pods"]

    Operator["OpenBao Operator"] -->|"JWT bootstrap"| Bao
    Admin["Admin ServiceAccount token"] -->|"JWT login"| Bao
    Bao -->|"Transit encrypt/decrypt"| Transit["Shared Transit provider"]
    CertMgr["cert-manager"] -->|"TLS Secret issuance"| TLS["External TLS Secrets"]
    TLS --> Bao

    classDef read fill:transparent,stroke:#79c0ab,stroke-width:2px,color:#e6f4ef;
    classDef process fill:transparent,stroke:#fdd0a4,stroke-width:2px,color:#e6f4ef;
    classDef write fill:transparent,stroke:#87d6be,stroke-width:2px,color:#e6f4ef;

    class Client,Admin read;
    class Edge,Operator,CertMgr process;
    class Public,Bao,Transit,TLS write;`}
/>

## Why this lane exists

<DecisionTable
  kind="reference"
  title="Key design choices"
  columns={["Choice", "What it protects", "Why it stays in the lane"]}
  rows={[
    {
      cells: [
        "Transit stays external",
        "The seal root is not stored or derived from the cluster itself.",
        "A hardened rehearsal lane should prove the dependency on an external seal provider instead of hiding it.",
      ],
      emphasis: "recommended",
    },
    {
      cells: [
        "TLS stays external",
        "Certificate issuance and trust material do not collapse into operator-managed defaults.",
        "This keeps the certificate contract aligned with the kind of deployment that already has an external PKI owner.",
      ],
    },
    {
      cells: [
        "Passthrough is user-managed",
        "The OpenBao server remains the TLS endpoint.",
        "The lane intentionally avoids turning shared ingress behavior into part of the hardened contract.",
      ],
    },
  ]}
/>

<Checklist
  tone="warning"
  title="Stay on the validated path"
  items={[
    "keep `spec.profile: Hardened`",
    "keep the shared Transit provider reachable and trusted",
    "keep `tls.mode: External` and provide the expected CA and server Secrets",
    "keep the passthrough route managed outside `spec.gateway`",
    "disable AppArmor only when the local runtime requires it and treat that as a local-environment concession, not a preferred default",
  ]}
/>

<Callout type="success" title="What this lane validated">

The validated local lane exercised hardened bootstrap with self-init, Transit auto-unseal through a shared external OpenBao service, JWT login for a human admin `ServiceAccount`, and passthrough external access with externally managed TLS Secrets.

</Callout>

<Callout type="warning" title="What this lane is not">

This is not a cloud reference, not a GitOps reference, and not proof that `spec.gateway` itself is the right path for hardened passthrough. It is a local rehearsal lane with explicit external dependencies.

</Callout>

<NextActions
  title="Use the lane"
  items={[
    {
      label: "Deployment recipe",
      description: "Follow the exact steps that reproduce this validated lane in the local environment.",
      docId: "user-guide/validated-deployments/recipes/local/hardened-transit-external-tls",
    },
    {
      label: "External access",
      description: "Compare the lane's user-managed passthrough choice with the generic service-boundary guidance in the main docs.",
      docId: "user-guide/openbaocluster/configuration/external-access",
    },
    {
      label: "TLS and identity",
      description: "Review the security model behind external certificates and server-side TLS ownership.",
      docId: "security/workload/tls",
    },
  ]}
/>
