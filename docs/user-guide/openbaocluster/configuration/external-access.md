---
title: External Access
slug: /configure/external-access
hide_title: true
pageType: task
journey: configure
description: Choose how clients reach OpenBao, decide where TLS terminates, and map that choice to Gateway API, Ingress, or direct service exposure.
---

<PageHeader
  title="Choose an external access path"
  lede="OpenBao can be exposed through Gateway API, Ingress, or a direct L4 Service. This page focuses on where TLS terminates, who owns certificate lifecycle, and how each edge path fits the intended operating posture."
/>



<DecisionTable
  title="Choose the access path deliberately"
  columns={["Path", "Use it when", "What the operator creates", "Watch for"]}
  rows={[
    {
      cells: [
        "Gateway API",
        "You want the strongest long-term edge model, clearer multi-tenancy, and first-class passthrough support.",
        "Routes and supporting integration objects for the configured mode.",
        "Controller feature support still matters, especially for `TLSRoute` and `BackendTLSPolicy`.",
      ],
      emphasis: "recommended",
    },
    {
      cells: [
        "Ingress",
        "You already have an ingress controller path and only need standard Kubernetes ingress semantics.",
        "An `Ingress` resource targeting the public OpenBao Service.",
        "This is usually a termination-oriented model and is less expressive than Gateway API for shared-platform routing.",
      ],
    },
    {
      cells: [
        "Direct Service exposure",
        "You want the simplest L4 path, often through a cloud load balancer or private network boundary.",
        "A `LoadBalancer` or `NodePort`-style Service configuration.",
        "You own more of the perimeter behavior yourself and lose the richer route-level policy surface.",
      ],
    },
  ]}
/>

<DecisionTable
  title="Where TLS should terminate"
  columns={["Pattern", "Use it when", "Why it fits or does not fit"]}
  rows={[
    {
      cells: [
        "Passthrough",
        "You want OpenBao to remain the TLS endpoint and preserve end-to-end certificate identity.",
        "This is the cleanest production default because the application keeps control of the server certificate and private key usage.",
      ],
      emphasis: "recommended",
    },
    {
      cells: [
        "Edge termination with backend TLS",
        "You need HTTP-aware controls, policy enforcement, or centralized certificate handling at the edge.",
        "This is valid, but you must be explicit about how trust is re-established between the edge and OpenBao.",
      ],
    },
    {
      cells: [
        "Temporary operator-managed trust",
        "You are standing up a development or internal evaluation environment quickly.",
        "This is appropriate for development and internal evaluation paths, but it does not match the Hardened production contract.",
      ],
      emphasis: "caution",
    },
  ]}
/>

<DiagramFrame
  title="Exposure paths"
  caption="The service boundary is a choice between where traffic enters, where TLS terminates, and how much of the edge behavior the operator is expected to own."
  code={`flowchart LR
    Client["Client"] --> Edge["Gateway / Ingress / L4 LB"]
    Edge --> OpenBao["OpenBao public Service"]
    OpenBao --> Pods["OpenBao Pods"]

    classDef read fill:transparent,stroke:#79c0ab,stroke-width:2px,color:#e6f4ef;
    classDef process fill:transparent,stroke:#fdd0a4,stroke-width:2px,color:#e6f4ef;
    classDef write fill:transparent,stroke:#87d6be,stroke-width:2px,color:#e6f4ef;

    class Client read;
    class Edge process;
    class OpenBao,Pods write;`}
/>

## Representative configurations

<Tabs groupId="external-access-gateway-ingress-service">
  <TabItem value="gateway" label="Gateway API">

<CommandBlock
  language="yaml"
  label="configure"
  title="Expose OpenBao through Gateway API"
  code={`spec:
  gateway:
    enabled: true
    tlsPassthrough: true
    hostname: bao.example.com
    gatewayRef:
      name: main-gateway
      namespace: gateway-system`}
>
  Start here when Gateway API is your edge standard. For most production clusters, use TLS passthrough unless you have a specific need for termination at the Gateway.
</CommandBlock>

  </TabItem>
  <TabItem value="ingress" label="Ingress">

<CommandBlock
  language="yaml"
  label="configure"
  title="Expose OpenBao with a standard Ingress"
  code={`spec:
  ingress:
    enabled: true
    host: "bao.example.com"
    annotations:
      nginx.ingress.kubernetes.io/backend-protocol: "HTTPS"`}
>
  This is useful when you already operate an ingress-controller standard and do not need the richer Gateway API route model.
</CommandBlock>

  </TabItem>
  <TabItem value="service" label="Service">

<CommandBlock
  language="yaml"
  label="configure"
  title="Expose OpenBao directly through an L4 Service"
  code={`spec:
  service:
    type: LoadBalancer
    annotations:
      service.beta.kubernetes.io/aws-load-balancer-type: "nlb"`}
>
  Use this when you want the simplest network boundary and your platform already treats a load balancer or private network edge as the primary perimeter.
</CommandBlock>

  </TabItem>
</Tabs>

## Match the TLS mode to the exposure path

<DecisionTable
  kind="reference"
  title="TLS mode pairings"
  columns={["TLS mode", "Good exposure fit", "Why"]}
  rows={[
    {
      cells: [
        "External",
        "Gateway passthrough, Ingress re-encryption, or direct Service exposure",
        "This keeps CA ownership outside the operator and works cleanly with both internal and externally terminated edges.",
      ],
      emphasis: "recommended",
    },
    {
      cells: [
        "ACME",
        "Gateway passthrough or direct public exposure",
        "OpenBao must remain the TLS endpoint to complete ACME challenge and certificate lifecycle correctly.",
      ],
    },
    {
      cells: [
        "OperatorManaged",
        "Development or internal evaluation paths only",
        "This keeps startup simple, but it makes the operator the certificate authority and does not match the Hardened production posture.",
      ],
      emphasis: "caution",
    },
  ]}
/>

## External TLS Secret contract

<Callout type="note" title="What `tls.mode: External` actually expects">

For the cluster-facing TLS path, the operator reads fixed same-namespace Secret names:

- `<cluster-name>-tls-ca` with key `ca.crt`
- `<cluster-name>-tls-server` with keys `tls.crt` and `tls.key`

The server certificate must chain to the CA bundle and cover the internal service name `openbao-cluster-<cluster-name>.local`, plus any configured ingress hostname, gateway hostname, and `spec.tls.extraSANs`.

</Callout>

<Callout type="tip" title="Do not confuse edge TLS Secrets with the cluster server TLS Secrets">

`spec.ingress.tlsSecretName` only controls the Secret referenced by the managed Ingress resource. It does not rename the Secrets that the OpenBao Pods use for their own listener certificate and CA trust.

</Callout>

<Callout type="note" title="Traefik v3 backend trust">

If you use Traefik v3 with backend TLS validation, configure a `ServersTransport` that trusts the generated CA Secret for the cluster. This is an implementation detail of the ingress path, not a reason to change the underlying TLS model.

</Callout>

<NextActions
  title="Continue service boundary setup"
  items={[
    {
      label: "Gateway API support",
      description: "Use the detailed Gateway API page when Gateway is the primary edge path.",
      docId: "user-guide/openbaocluster/configuration/gateway-api",
    },
    {
      label: "Network configuration",
      description: "Align ingress peers, DNS, API-server egress, and external service egress with the exposure path you chose.",
      docId: "user-guide/openbaocluster/configuration/network",
    },
    {
      label: "Unseal configuration",
      description: "Use the exact Secret and mounted-file contract page when external TLS and unseal credentials both depend on the same trust workflow.",
      docId: "user-guide/openbaocluster/configuration/unseal",
    },
    {
      label: "TLS and workload identity",
      description: "Go deeper on certificate ownership and trust boundaries when you need the security model behind these choices.",
      docId: "security/workload/tls",
    },
  ]}
/>
