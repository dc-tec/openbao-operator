---
title: Server Configuration
hide_title: true
pageType: task
journey: configure
description: Configure server-runtime defaults such as UI, listener behavior, audit devices, plugins, and Raft autopilot without mixing in unrelated exposure or observability concerns.
---

<PageHeader
  title="Tune the server runtime without turning this page into a dump of every field."
  lede="Use this page for the settings that shape how the OpenBao server itself runs: listener and lease behavior, Raft autopilot, audit devices, and plugin registration. Exposure, observability, and mirrored-image strategy each have their own configuration paths."
/>



<DecisionTable
  title="What this page owns"
  columns={["Surface", "Use it for", "Do not use it for"]}
  rows={[
    {
      cells: [
        "`spec.configuration`",
        "Listener behavior, UI, cache and lease settings, and Raft/autopilot tuning.",
        "Edge exposure patterns or gateway wiring.",
      ],
      emphasis: "recommended",
    },
    {
      cells: [
        "`spec.audit`",
        "Declarative audit-device setup that should exist when the cluster starts.",
        "General telemetry or metrics wiring.",
      ],
    },
    {
      cells: [
        "`spec.plugins` and plugin download settings",
        "Explicit OpenBao plugin registration and plugin fetch behavior.",
        "Mirrored base images or disconnected-registry strategy for the whole deployment.",
      ],
    },
    {
      cells: [
        "Raft autopilot",
        "Membership safety, dead-peer cleanup, and quorum behavior.",
        "Application-level backup, upgrade, or restore workflows.",
      ],
    },
  ]}
/>

<Callout type="note" title="Use the focused pages for adjacent concerns">

- <SiteLink docId="user-guide/openbaocluster/configuration/external-access">External access</SiteLink> owns exposure and ingress patterns.
- <SiteLink docId="user-guide/openbaocluster/configuration/observability">Observability</SiteLink> owns telemetry, scraping, and monitoring surfaces.
- <SiteLink docId="user-guide/openbaocluster/configuration/air-gapped">Air-gapped and private registries</SiteLink> owns mirrored-image and disconnected-environment strategy.

</Callout>

## Core server runtime

<CommandBlock
  language="yaml"
  label="configure"
  title="Start from the core server settings"
  code={`spec:
  configuration:
    ui: true
    cacheSize: 134217728
    disableCache: false
    defaultLeaseTTL: "720h"
    maxLeaseTTL: "8760h"
    listener:
      proxyProtocolBehavior: "use_proxy_protocol"
    raft:
      performanceMultiplier: 2`}
/>

<DecisionTable
  kind="reference"
  title="Common server knobs"
  columns={["Field", "Why you change it", "Operational note"]}
  rows={[
    {
      cells: [
        "`ui`",
        "Enable or disable the web UI intentionally.",
        "This is a service-boundary decision only if you also expose the route appropriately.",
      ],
      emphasis: "recommended",
    },
    {
      cells: [
        "`listener`",
        "Adjust listener behavior such as proxy-protocol handling.",
        "Keep listener-level TLS assumptions aligned with the external-access path you selected.",
      ],
    },
    {
      cells: [
        "`defaultLeaseTTL` / `maxLeaseTTL`",
        "Set sensible lease bounds for the workloads that depend on the cluster.",
        "Treat very long leases as an operational contract, not just a convenience setting.",
      ],
    },
    {
      cells: [
        "`raft.performanceMultiplier`",
        "Compensate for high-latency or slower control-plane environments.",
        "Change this deliberately and observe cluster behavior rather than cargo-culting larger values.",
      ],
    },
  ]}
/>

## Audit devices and plugins

<Tabs groupId="server-config-audit-plugins">
  <TabItem value="audit" label="Audit devices">

<CommandBlock
  language="yaml"
  label="configure"
  title="Enable declarative audit devices"
  code={`spec:
  audit:
    - type: file
      path: secure-audit
      description: "Secure audit logging"
      options:
        file_path: "/var/log/openbao/audit.log"
        format: "json"`}
>
  Audit devices belong in the cluster baseline so the service does not come up “temporarily unaudited” and stay that way by accident.
</CommandBlock>

  </TabItem>
  <TabItem value="plugins" label="Plugins">

<CommandBlock
  language="yaml"
  label="configure"
  title="Register OCI-based plugins declaratively"
  code={`spec:
  configuration:
    plugin:
      autoDownload: true
      downloadBehavior: "direct"
  plugins:
    - type: secret
      name: aws
      image: "ghcr.io/openbao/openbao-plugin-secrets-aws"
      version: "v1.0.0"
      binaryName: "openbao-plugin-secrets-aws"
      sha256sum: "9fdd8be7947e4a4caf7cce4f0e02695081b6c85178aa912df5d37be97363144c"`}
/>

  </TabItem>
</Tabs>

## Raft autopilot defaults

<DiagramFrame
  title="Autopilot ownership"
  caption="The operator keeps autopilot aligned with the cluster profile and replica count so peer cleanup and quorum behavior stay in bounds as the cluster changes."
  code={`flowchart LR
    Cluster["OpenBaoCluster"] --> Profile["Profile and replicas"]
    Profile --> Operator["Operator"]
    Operator --> Autopilot["Autopilot settings"]
    Autopilot --> Raft["Raft membership behavior"]

    classDef read fill:transparent,stroke:#79c0ab,stroke-width:2px,color:#e6f4ef;
    classDef process fill:transparent,stroke:#fdd0a4,stroke-width:2px,color:#e6f4ef;
    classDef write fill:transparent,stroke:#87d6be,stroke-width:2px,color:#e6f4ef;

    class Cluster,Profile read;
    class Operator process;
    class Autopilot,Raft write;`}
/>

<DecisionTable
  kind="reference"
  title="Autopilot defaults"
  columns={["Setting", "Default", "Why it exists"]}
  rows={[
    {
      cells: [
        "`cleanupDeadServers`",
        "`true`",
        "Dead peers should not linger indefinitely in a Kubernetes-managed environment.",
      ],
      emphasis: "recommended",
    },
    {
      cells: [
        "`deadServerLastContactThreshold`",
        "`5m`",
        "The operator uses a shorter threshold than the generic upstream default because cluster nodes and Pods are expected to churn faster in Kubernetes.",
      ],
    },
    {
      cells: [
        "`serverStabilizationTime`",
        "`10s`",
        "New servers should prove they are healthy before becoming stable voters.",
      ],
    },
    {
      cells: [
        "`minQuorum`",
        "Calculated from profile and replica count",
        "Hardened favors HA safety; Development favors flexibility for small clusters.",
      ],
    },
  ]}
/>

<Tabs groupId="server-config-autopilot">
  <TabItem value="override" label="Override defaults">

<CommandBlock
  language="yaml"
  label="configure"
  title="Customize autopilot explicitly"
  code={`spec:
  profile: Hardened
  replicas: 5
  configuration:
    raft:
      autopilot:
        minQuorum: 4
        deadServerLastContactThreshold: "10m"
        lastContactThreshold: "30s"
        maxTrailingLogs: 2000
        serverStabilizationTime: "30s"`}
>
  Override only when you have a concrete reason. Most clusters should start with the operator defaults and change them only after observing real failure or latency behavior.
</CommandBlock>

  </TabItem>
  <TabItem value="disable-cleanup" label="Disable cleanup">

<CommandBlock
  language="yaml"
  label="configure"
  title="Disable automatic dead-peer cleanup"
  code={`spec:
  configuration:
    raft:
      autopilot:
        cleanupDeadServers: false`}
>
  If you disable cleanup, you are taking manual ownership of peer removal. That is usually a temporary operational exception, not a steady-state recommendation.
</CommandBlock>

  </TabItem>
</Tabs>

<CommandBlock
  language="bash"
  label="inspect"
  title="Inspect the full configuration schema"
  code={`kubectl explain openbaocluster.spec.configuration`}
>
  Use this when you need the exact field tree. Keep this page for the defaults and decision boundaries, not as a full API dump.
</CommandBlock>

<NextActions
  title="Continue cluster baseline"
  items={[
    {
      label: "Observability",
      description: "Move to the telemetry and scraping page when you need monitoring rather than server-runtime tuning.",
      docId: "user-guide/openbaocluster/configuration/observability",
    },
    {
      label: "Air-gapped and private registries",
      description: "Use the disconnected-environment guide for mirrored images and private registry behavior.",
      docId: "user-guide/openbaocluster/configuration/air-gapped",
    },
    {
      label: "Operate",
      description: "Carry the resulting baseline into upgrades, backups, and troubleshooting once the cluster is live.",
      docId: "user-guide/openbaocluster/operations/index",
    },
  ]}
/>
