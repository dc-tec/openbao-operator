---
title: Gateway API Support
hide_title: true
pageType: task
journey: configure
description: Configure Gateway API as the primary edge path for OpenBao, including passthrough versus termination, readiness checks, and controller compatibility.
---

<PageHeader
  title="Gateway API integration"
  lede="Gateway API is the recommended edge model for explicit route ownership, listener mode, and cross-namespace attachment. For most production deployments, start with TLS passthrough so OpenBao remains the TLS endpoint."
/>



<DecisionTable
  title="Choose the Gateway mode"
  columns={["Mode", "Use it when", "What the operator creates", "Watch for"]}
  rows={[
    {
      cells: [
        "TLS passthrough",
        "You want OpenBao to remain the TLS endpoint and keep end-to-end certificate identity.",
        "`TLSRoute`-based integration that forwards encrypted traffic to the cluster.",
        "Your controller and GatewayClass need usable `TLSRoute` support, and ACME depends on this model.",
      ],
      emphasis: "recommended",
    },
    {
      cells: [
        "TLS termination",
        "You need HTTP-aware policy, WAF behavior, or centralized certificate handling at the Gateway.",
        "`HTTPRoute` plus optional `BackendTLSPolicy` for re-encrypted traffic to OpenBao.",
        "Be explicit about backend trust and do not assume termination alone preserves the same trust model.",
      ],
    },
  ]}
/>

<DiagramFrame
  title="Gateway API modes"
  caption="Passthrough keeps OpenBao as the TLS endpoint. Termination shifts certificate presentation to the Gateway and then re-establishes trust on the backend hop."
  code={`flowchart TB
    subgraph Pass ["TLS passthrough"]
      ClientA["Client"] --> GatewayA["Gateway"]
      GatewayA --> BaoA["OpenBao"]
    end

    subgraph Term ["TLS termination"]
      ClientB["Client"] --> GatewayB["Gateway"]
      GatewayB --> Policy["BackendTLSPolicy"]
      Policy --> BaoB["OpenBao"]
    end

    classDef read fill:transparent,stroke:#79c0ab,stroke-width:2px,color:#e6f4ef;
    classDef process fill:transparent,stroke:#fdd0a4,stroke-width:2px,color:#e6f4ef;
    classDef write fill:transparent,stroke:#87d6be,stroke-width:2px,color:#e6f4ef;

    class ClientA,ClientB read;
    class GatewayA,GatewayB,Policy process;
    class BaoA,BaoB write;`}
/>

## Recommended path: TLS passthrough

<CommandBlock
  language="yaml"
  label="configure"
  title="Expose OpenBao through a passthrough Gateway listener"
  code={`spec:
  gateway:
    enabled: true
    tlsPassthrough: true
    hostname: bao.example.com
    gatewayRef:
      name: main-gateway
      namespace: gateway-system`}
>
  This is the default production recommendation because OpenBao remains the TLS endpoint. It fits the trust model used by External TLS and ACME particularly well.
</CommandBlock>

<ExpandableCallout type="example" title="Example Gateway listener for passthrough">

<CommandBlock
  language="yaml"
  label="inspect"
  title="Gateway listener with `TLS` passthrough"
  code={`apiVersion: gateway.networking.k8s.io/v1
kind: Gateway
metadata:
  name: main-gateway
  namespace: gateway-system
spec:
  gatewayClassName: traefik
  listeners:
    - name: websecure-passthrough
      port: 443
      protocol: TLS
      tls:
        mode: Passthrough`}
/>

</ExpandableCallout>

<Callout type="warning" title="ACME requires passthrough">

If `tls.mode` is `ACME`, do not terminate TLS at the Gateway. OpenBao must remain the TLS endpoint to complete the ACME lifecycle correctly.

</Callout>

## Alternative path: Gateway-side termination

<CommandBlock
  language="yaml"
  label="configure"
  title="Expose OpenBao with Gateway termination"
  code={`spec:
  gateway:
    enabled: true
    hostname: bao.example.com
    gatewayRef:
      name: main-gateway
      namespace: gateway-system`}
>
  Use Gateway-side termination when you need HTTP-aware policy, WAF behavior, or centralized certificate handling. In that model the operator can create a `BackendTLSPolicy` so the backend hop stays encrypted and validated.
</CommandBlock>

<ExpandableCallout type="note" title="What the operator adds for backend TLS">

When backend TLS is enabled, the operator creates a `BackendTLSPolicy` that pins backend validation to the cluster's CA ConfigMap and expected service hostname.

</ExpandableCallout>

## Compatibility and readiness

<DecisionTable
  kind="reference"
  title="What must be true"
  columns={["Check", "Why it matters", "What to confirm"]}
  rows={[
    {
      cells: [
        "Gateway reference exists",
        "The operator cannot integrate with a missing or mistyped target.",
        "The referenced `Gateway` object is present in the namespace you point to.",
      ],
      emphasis: "recommended",
    },
    {
      cells: [
        "GatewayClass exists and is accepted",
        "A route is only useful if the controller owning the GatewayClass is real and healthy.",
        "The selected `GatewayClass` exists and reports acceptance.",
      ],
    },
    {
      cells: [
        "Required feature support exists",
        "Passthrough needs `TLSRoute`; termination may need `BackendTLSPolicy` support.",
        "If the controller publishes `status.supportedFeatures`, verify the required features are present.",
      ],
    },
    {
      cells: [
        "Listener mode matches the chosen path",
        "A passthrough route attached to a terminating listener will never behave correctly.",
        "The selected listener protocol and mode are compatible with the route type the operator is going to create.",
      ],
    },
  ]}
/>

<DecisionTable
  kind="reference"
  title="Conditions to watch"
  columns={["Condition", "What it means", "Typical next move"]}
  rows={[
    {
      cells: [
        "`GatewayIntegrationReady=True`",
        "The operator verified the Gateway reference, class, listener path, and available controller support well enough for the chosen mode.",
        "Continue with end-to-end validation and client connectivity checks.",
      ],
      emphasis: "recommended",
    },
    {
      cells: [
        "`GatewayIntegrationReady=Unknown`",
        "The controller has not published enough status, or feature support cannot be verified from the available information.",
        "Wait for more Gateway status or inspect whether the controller publishes supported features at all.",
      ],
    },
    {
      cells: [
        "`GatewayIntegrationReady=False`",
        "The operator found a concrete incompatibility or missing dependency.",
        "Read the condition reason first: missing reference, unsupported feature, incompatible listener, or not-programmed gateway are the common cases.",
      ],
    },
  ]}
/>

<Callout type="note" title="Blue-green upgrades keep the route stable">

During blue-green cutover, the operator keeps the Gateway route attached to the stable external Service and updates the Service selector behind it. That means the route object stays steady while the selected revision changes underneath.

</Callout>

<NextActions
  title="Continue service boundary setup"
  items={[
    {
      label: "Network configuration",
      description: "Align trusted ingress peers and external dependency egress with the Gateway path you just chose.",
      docId: "user-guide/openbaocluster/configuration/network",
    },
    {
      label: "External access",
      description: "Switch back to the broader exposure decision page if you still need to compare Gateway with Ingress or direct Service exposure.",
      docId: "user-guide/openbaocluster/configuration/external-access",
    },
    {
      label: "TLS and workload identity",
      description: "Review the underlying TLS ownership model when deciding between passthrough and termination.",
      docId: "security/workload/tls",
    },
  ]}
/>
