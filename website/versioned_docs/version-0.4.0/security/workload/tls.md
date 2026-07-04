---
title: TLS and Workload Identity
hide_title: true
pageType: concept
journey: security
description: How peer trust, certificate rotation, and workload-facing TLS identity work across operator-managed, external, and ACME-backed deployments.
---

<PageHeader
  title="TLS and workload identity"
  lede="How pods trust each other, how clients verify the service, where certificate authority material lives, and how each TLS mode changes workload identity and certificate handling."
/>



<DecisionTable
  title="Choose the TLS mode deliberately"
  columns={["Mode", "Use it when", "What the operator owns", "Watch for"]}
  rows={[
    {
      cells: [
        "External",
        "You already have a trusted PKI, cert-manager, or platform certificate workflow.",
        "The operator consumes existing Secrets and watches for rotation, but does not mint the trust chain.",
        "This is the preferred Hardened production path because CA authority stays outside the operator.",
      ],
      emphasis: "recommended",
    },
    {
      cells: [
        "ACME",
        "The service is exposed publicly and OpenBao should obtain certificates directly from an ACME provider.",
        "The operator wires the listener path, but OpenBao handles the certificate lifecycle itself.",
        "This works best when the service owns the public endpoint and you can meet the ACME challenge requirements.",
      ],
    },
    {
      cells: [
        "OperatorManaged",
        "You need a fast internal evaluation path or temporary development certificates.",
        "The operator generates and rotates the CA and leaf certificates inside the cluster.",
        "This is not the Hardened production posture because the operator holds certificate authority material.",
      ],
      emphasis: "caution",
    },
  ]}
/>

<DiagramFrame
  title="Certificate rotation and reload path"
  caption="When the certificate source changes, the operator updates the mounted material and the workload reloads it without rebuilding or reinstalling the cluster."
  code={`sequenceDiagram
    participant Source as Certificate source
    participant Operator as Operator
    participant Secret as Kubernetes Secret
    participant Pod as OpenBao Pod

    Source->>Operator: New certificate or renewal event
    Operator->>Secret: Update mounted TLS material
    Secret->>Pod: Projected volume refresh
    Pod->>Pod: Reload listener configuration

    Note over Pod: New certificate becomes active without a full cluster redeploy`}
/>

## Trust paths that matter

<DecisionTable
  kind="reference"
  title="TLS surfaces"
  columns={["Path", "What is being protected", "Primary concern"]}
  rows={[
    {
      cells: [
        "Client to service",
        "Application and operator clients verifying the OpenBao listener.",
        "The public or internal certificate presented by the service must chain to a trust source your clients already accept.",
      ],
      emphasis: "recommended",
    },
    {
      cells: [
        "Pod to pod",
        "Raft and internal service traffic between OpenBao members.",
        "The SAN set and CA distribution need to match pod and service DNS accurately so peers can authenticate each other.",
      ],
    },
    {
      cells: [
        "Edge proxy to backend",
        "Gateway, ingress, or mesh traffic between the edge and the cluster.",
        "Choose passthrough versus termination deliberately so you know where the private key lives and where client identity is enforced.",
      ],
    },
  ]}
/>

## Where key material lives

<DecisionTable
  kind="reference"
  title="Key and CA ownership"
  columns={["Mode", "Server private key", "CA or trust root", "Operational consequence"]}
  rows={[
    {
      cells: [
        "External",
        "Kubernetes Secret supplied by your PKI workflow",
        "External CA or organizational PKI",
        "Certificate lifecycle aligns with the rest of your platform and is easier to audit centrally.",
      ],
      emphasis: "recommended",
    },
    {
      cells: [
        "ACME",
        "Generated inside OpenBao",
        "Public ACME issuer",
        "The operator never needs the private key, but the cluster must satisfy the ACME issuance path.",
      ],
    },
    {
      cells: [
        "OperatorManaged",
        "Kubernetes Secret managed by the operator",
        "Operator-generated internal CA",
        "Fast to stand up, but the trust root now lives inside the same management plane you are trying to keep small and reviewable.",
      ],
      emphasis: "caution",
    },
  ]}
/>

## Exposure guidance

<DecisionTable
  title="Edge exposure choices"
  columns={["Pattern", "Use it when", "Why it is preferred or risky"]}
  rows={[
    {
      cells: [
        "TLS passthrough",
        "You want OpenBao to terminate TLS and preserve end-to-end certificate identity.",
        "This is usually the cleanest production path because the application keeps control of the server certificate and the edge stays as a transport router.",
      ],
      emphasis: "recommended",
    },
    {
      cells: [
        "Edge termination",
        "You need policy enforcement, client-auth handling, or platform certificate lifecycle at the edge.",
        "This can be valid, but you must be explicit about how trust is re-established between the proxy and OpenBao.",
      ],
    },
    {
      cells: [
        "Temporary self-signed or operator-generated edge trust",
        "Short-lived evaluation environments only.",
        "This path is easy to start but tends to leak into production unless you set a deliberate migration plan.",
      ],
      emphasis: "caution",
    },
  ]}
/>

<Callout type="note" title="Configuration ownership">

Use the configuration guides below when you need the exact cluster fields:

- <SiteLink docId="user-guide/openbaocluster/configuration/external-access">External access</SiteLink>
- <SiteLink docId="user-guide/openbaocluster/configuration/gateway-api">Gateway API support</SiteLink>
- <SiteLink docId="user-guide/openbaocluster/configuration/network">Network configuration</SiteLink>
- <SiteLink docId="user-guide/openbaocluster/configuration/unseal">Unseal configuration</SiteLink>

</Callout>

<NextActions
  title="Continue workload protections"
  items={[
    {
      label: "Supply-chain verification",
      description: "Review how the operator verifies and pins the images behind these workloads.",
      docId: "security/workload/supply-chain",
    },
    {
      label: "Production posture",
      description: "See how TLS mode choice feeds into the Hardened security profile.",
      docId: "security/fundamentals/profiles",
    },
    {
      label: "External access",
      description: "Switch to the task page when you need the concrete service and gateway configuration.",
      docId: "user-guide/openbaocluster/configuration/external-access",
    },
  ]}
/>
