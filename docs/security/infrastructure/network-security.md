---
title: Network Security
hide_title: true
pageType: concept
journey: security
description: Default-deny network posture for OpenBao Pods and lifecycle jobs, plus the control-plane and egress assumptions that support it.
---

<PageHeader
  title="Network policy model"
  lede="Default-deny posture for OpenBao Pods and lifecycle jobs, plus the ingress and egress paths the operator expects when cluster management and integrations are configured."
/>



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

<Callout type="note" title="Read replicas are client-serving Pods">

When steady read replicas are enabled, the main client Service can route to both voter and read-replica Pods. That does not create a separate ingress policy surface by itself; the same cluster ingress rules still govern the client listener, and the optional dedicated read Service remains only a second Service object selecting the same client-serving port on the read pool.

</Callout>

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

<Callout type="note" title="Steady read replicas are staged out of destructive workflows">

Blue-green promotion and restore intentionally drain the steady read pool before destructive membership or snapshot work starts, then restore it before the workflow completes. That is a network and safety boundary as much as a rollout detail: destructive workflows do not keep a second permanent client-serving non-voter tier online while peer removal or snapshot replacement is in progress.

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
