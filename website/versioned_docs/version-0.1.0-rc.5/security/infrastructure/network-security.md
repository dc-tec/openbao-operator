---
title: Network Security
hide_title: true
pageType: concept
journey: security
description: Default-deny network posture for OpenBao Pods and lifecycle jobs, plus the control-plane and egress assumptions that support it.
---

<PageHero
  variant="compact"
  eyebrow="Security / Platform Controls"
  title="Start from default deny and open only the traffic the lifecycle actually needs."
  lede="The operator treats network policy as part of the security model, not as an optional hardening layer. OpenBao Pods and lifecycle jobs begin from explicit allowlists, and the allowed traffic should line up with clustering, management, and the integrations the cluster is deliberately configured to use."
  actions={[
    {label: 'Open network configuration', docId: 'user-guide/openbaocluster/configuration/network', variant: 'primary'},
    {label: 'Open admission policies', docId: 'security/infrastructure/admission-policies', variant: 'secondary'},
  ]}
>
  <Checklist
    title="Use this page when you need to"
    items={[
      'understand the default-deny perimeter around OpenBao Pods',
      'review the difference between workload traffic and backup or restore job egress',
      'check which traffic is allowed to peers, the API server, DNS, and ingress paths',
      'connect status conditions back to network assumptions and failures',
    ]}
  />
</PageHero>

<DiagramFrame
  title="OpenBao network perimeter"
  caption="The workload path starts closed and only opens the traffic required for cluster management, Raft, ingress, and explicitly configured integrations."
  code={`flowchart TB
    Operator["Operator Pod"]
    K8sAPI["Kubernetes API"]
    DNS["CoreDNS"]
    Peer["Raft peers"]

    subgraph Cluster ["OpenBao perimeter (default deny)"]
        Node["OpenBao Pod"]
    end

    Ingress["Gateway / trusted ingress peer"] --> Node
    Operator --> Node
    Peer --> Node
    Jobs["Backup / restore Job"] --> Node
    Node --> K8sAPI
    Node --> DNS
    Node --> Peer

    classDef read fill:transparent,stroke:#79c0ab,stroke-width:2px,color:#e6f4ef;
    classDef process fill:transparent,stroke:#fdd0a4,stroke-width:2px,color:#e6f4ef;
    classDef write fill:transparent,stroke:#87d6be,stroke-width:2px,color:#e6f4ef;

    class Operator,K8sAPI,DNS,Peer,Ingress read;
    class Jobs process;
    class Node write;`}
/>

<DecisionTable
  title="Network posture at a glance"
  columns={['Surface', 'Default posture', 'What opens it up']}
  rows={[
    {
      cells: [
        'Workload ingress',
        'Denied by default',
        'Operator probes, Raft peer traffic, and explicit ingress peers through gateway or configured trusted peers.',
      ],
      emphasis: 'recommended',
    },
    {
      cells: [
        'Workload egress',
        'Denied by default except for core cluster dependencies',
        'API server, DNS, Raft peer traffic, and explicitly configured integrations.',
      ],
    },
    {
      cells: [
        'Backup and restore job egress',
        'Separate from the main workload policy',
        'Explicit object-storage and identity reachability assumptions through job-level configuration.',
      ],
    },
    {
      cells: [
        'Controller ingress',
        'Restricted to health and metrics surfaces',
        'Monitoring and kubelet probe paths only when operator network policies are enabled.',
      ],
    },
  ]}
/>

## Workload traffic rules

<Tabs groupId="platform-controls-network">

<TabItem value="ingress" label="Ingress">

<DecisionTable
  kind="reference"
  title="Allowed ingress paths"
  columns={['Source', 'Typical port', 'Why it exists']}
  rows={[
    {
      cells: [
        'Operator',
        '`8200`',
        'Required for probes, initialization checks, and status-related interactions with the workload.',
      ],
      emphasis: 'recommended',
    },
    {
      cells: [
        'Raft peers',
        '`8201/TCP`',
        'Required for consensus, leader election, and replication between StatefulSet members.',
      ],
    },
    {
      cells: [
        'Gateway or trusted ingress peer',
        '`8200`',
        'Allowed only when gateway integration or trusted ingress peers are configured deliberately.',
      ],
    },
    {
      cells: [
        'Kube-system health paths',
        'Platform-dependent',
        'Some CNIs or DNS health checks require controlled access from system namespaces.',
      ],
    },
  ]}
/>

If you need additional ingress, use:

- `spec.network.ingressRules` for additive peer rules
- `spec.network.trustedIngressPeers` for user-managed ingress or passthrough proxies

</TabItem>

<TabItem value="egress" label="Egress">

<DecisionTable
  kind="reference"
  title="Allowed egress paths"
  columns={['Destination', 'Typical port', 'Why it exists']}
  rows={[
    {
      cells: [
        'Kubernetes API',
        '`443`',
        'Required for service registration, status-related interactions, and controlled API assumptions.',
      ],
      emphasis: 'recommended',
    },
    {
      cells: [
        'CoreDNS',
        '`53/TCP+UDP`',
        'Required to resolve peers and configured external services.',
      ],
    },
    {
      cells: [
        'Raft peers',
        '`8201/TCP`',
        'Required for replication and cluster membership traffic.',
      ],
    },
    {
      cells: [
        'Object storage, KMS, PKI, or other integrations',
        'Explicitly configured',
        'These should only open when the corresponding feature is intentionally enabled.',
      ],
    },
  ]}
/>

<Callout type="note" title="Lifecycle jobs use separate egress assumptions">

Backup and restore do not rely on the main workload policy alone. They use separate job identities and separate network assumptions, which is why `BackupConfigurationReady` and `RestoreConfigurationReady` exist as distinct status signals.

</Callout>

</TabItem>

</Tabs>

## Status checkpoints for network assumptions

When manifests alone do not explain the observed behavior, start with these conditions:

- `APIServerNetworkReady` for API-server reachability assumptions
- `GatewayIntegrationReady` for gateway listener and controller compatibility
- `BackupConfigurationReady` for object-storage and backup auth reachability
- `RestoreConfigurationReady` for restore job egress and identity assumptions

## Controller network posture

<DecisionTable
  kind="reference"
  title="Controller networking"
  columns={['Surface', 'Posture', 'Why']}
  rows={[
    {
      cells: [
        'Ingress',
        'Restricted to metrics and kubelet probes when operator network policies are enabled.',
        'The controller should not expose a broad incoming network surface.',
      ],
      emphasis: 'recommended',
    },
    {
      cells: [
        'Egress',
        'Primarily the Kubernetes API and essential control-plane services.',
        'Controller traffic should stay close to reconciliation and not behave like a general egress client.',
      ],
    },
  ]}
/>

<NextActions
  title="Continue platform controls"
  items={[
    {
      label: 'Admission policies',
      description: 'See how unsafe configuration and drift are blocked before they persist.',
      docId: 'security/infrastructure/admission-policies',
    },
    {
      label: 'RBAC architecture',
      description: 'Connect network boundaries back to the split-controller identity model.',
      docId: 'security/infrastructure/rbac',
    },
    {
      label: 'Configure network settings',
      description: 'Switch to the task page when you are ready to set ingress or egress rules on a real cluster.',
      docId: 'user-guide/openbaocluster/configuration/network',
    },
  ]}
/>
