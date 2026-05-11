---
title: Server Configuration
hide_title: true
pageType: task
journey: configure
description: Configure server-runtime defaults such as UI, listener behavior, audit devices, plugins, and Raft autopilot, and use the dedicated pages for adjacent configuration areas.
---

<PageHeader
  title="Configure the server runtime"
  lede="Use this page for the settings that shape how the OpenBao server runs: listener and lease behavior, Raft autopilot, audit devices, and plugin registration. Exposure, observability, and mirrored-image settings are documented separately."
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
        "Very long leases change the operational contract for the workloads that depend on the cluster.",
      ],
    },
    {
      cells: [
        "`raft.performanceMultiplier`",
        "Compensate for high-latency or slower control-plane environments.",
        "Change this deliberately and verify that measured latency or failure behavior requires the larger value.",
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
  Include audit devices in the cluster baseline so the service starts with the expected audit configuration.
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
>
  Use an `image` plugin when OpenBao should download the plugin from an OCI registry as part of server startup. The operator renders `plugin_directory = "/openbao/plugins"` and mounts a writable, pod-local volume at that path for OCI auto-download.
</CommandBlock>

<CommandBlock
  language="yaml"
  label="configure"
  title="Register preinstalled plugins"
  code={`spec:
  plugins:
    - type: secret
      name: local-example
      command: "openbao-plugin-secrets-example"
      version: "v1.0.0"
      binaryName: "openbao-plugin-secrets-example"
      sha256sum: "9fdd8be7947e4a4caf7cce4f0e02695081b6c85178aa912df5d37be97363144c"`}
>
  Use a `command` plugin when the binary is already available inside the OpenBao runtime image or another explicitly managed runtime path.
</CommandBlock>

<DecisionTable
  kind="reference"
  title="Plugin fields"
  columns={["Surface", "Use it for", "Operational note"]}
  rows={[
    {
      cells: [
        "`spec.plugins[].image`",
        "OCI-based plugin binaries that OpenBao downloads at startup.",
        "Set `spec.configuration.plugin.autoDownload: true`; OpenBao pods need registry egress or access to a reachable mirror.",
      ],
      emphasis: "recommended",
    },
    {
      cells: [
        "`spec.plugins[].command`",
        "Plugin binaries already present in the OpenBao runtime environment.",
        "The operator does not create a plugin-download volume for command-only plugins.",
      ],
    },
    {
      cells: [
        "`spec.configuration.plugin.autoRegister`",
        "Automatic plugin catalog registration.",
        "`args` and `env` on each plugin are only used when auto-register is enabled.",
      ],
    },
    {
      cells: [
        "`spec.configuration.plugin.downloadBehavior`",
        "OpenBao's OCI download mode.",
        "Use values supported by the OpenBao version in `spec.version`; plugin auto-download settings require OpenBao 2.5.0 or newer.",
      ],
    },
  ]}
/>

<Callout type="note" title="Downloaded plugins are runtime cache">

OCI-downloaded plugins are stored under `/openbao/plugins` on an ephemeral pod-local volume. Treat that directory as a writable startup cache, not durable storage. If the cluster runs in a private or disconnected environment, mirror the plugin image and make sure OpenBao's runtime OCI client can authenticate to that registry; Kubernetes `imagePullSecrets` only cover Kubernetes image pulls.

</Callout>

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
  Start with the operator defaults and override them only after measuring behavior that requires a change.
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
  If you disable cleanup, you are taking manual ownership of peer removal. This is usually a temporary operational exception rather than the steady-state configuration.
</CommandBlock>

  </TabItem>
</Tabs>

<CommandBlock
  language="bash"
  label="inspect"
  title="Inspect the full configuration schema"
  code={`kubectl explain openbaocluster.spec.configuration`}
>
  Use this when you need the exact field tree. Keep this page for defaults and decision boundaries rather than exhaustive field-by-field reference.
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
