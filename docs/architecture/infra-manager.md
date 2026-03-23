---
title: Infrastructure Manager
hide_title: true
pageType: concept
journey: architecture
description: Render OpenBaoCluster into converged configuration, StatefulSet resources, and rollout triggers for the workload path.
---

<PageHero
  variant="compact"
  eyebrow="Architecture / Workload Manager"
  title="Render the cluster spec into a converged StatefulSet and configuration."
  lede="The infrastructure manager is the workload path that turns `OpenBaoCluster` into running Kubernetes resources. It owns rendered configuration, StatefulSet-facing infrastructure, and the rollout triggers that keep configuration drift and pod lifecycle changes in sync."
  actions={[
    {label: 'Open component design', docId: 'architecture/components', variant: 'primary'},
    {label: 'Open cluster overview', docId: 'user-guide/openbaocluster/overview', variant: 'secondary'},
  ]}
>
  <Checklist
    title="Use this page when you need to"
    items={[
      'understand how spec changes become rendered workload resources',
      'trace config, Services, and StatefulSet ownership back to one manager',
      'see how TLS, unseal, and image verification feed into workload rendering',
      'reason about rollout triggers and workload-side safety boundaries',
    ]}
  />
</PageHero>

<ManagerAtAGlance
  sections={[
    {
      label: 'Control path',
      items: [
        'workload reconciler',
        'internal/app/openbaocluster workload orchestration',
        'internal/service/infra',
      ],
    },
    {
      label: 'Owns',
      items: [
        'rendered config.hcl',
        'StatefulSet, Services, ConfigMaps, and workload-facing Secrets',
        'static unseal key secret when the operator manages the seal',
      ],
    },
    {
      label: 'Writes',
      items: [
        'pod-template config hash annotations',
        'rendered workload resources and their ownership metadata',
        'state transitions that trigger safe workload rollout',
      ],
    },
    {
      label: 'Depends on',
      items: [
        'TLS mode and certificate material',
        'seal mode and credentials',
        'image verification policy and current workload health',
      ],
    },
  ]}
/>

## Architectural Placement

Infrastructure reconciliation belongs to the workload orchestration path:

1. `internal/controller/openbaocluster` receives a workload-side reconcile event.
2. The controller delegates into the `internal/app/openbaocluster` facade.
3. Workload orchestration calls `internal/service/infra` to render resources and apply them.

That split keeps controller code as reconcile plumbing while the infra manager owns the workload contract.

<DecisionTable
  kind="reference"
  title="Owned surfaces"
  columns={['Surface', 'What the manager decides', 'Why it matters']}
  rows={[
    {
      cells: ['Rendered config.hcl', 'Listener, storage, service registration, TLS, seal, and integration stanzas.', 'Configuration drift must stay aligned with the declared cluster spec.'],
      emphasis: 'recommended',
    },
    {
      cells: ['StatefulSet and Services', 'Replica intent, pod template, discovery Services, and workload wiring.', 'The workload path owns the pod lifecycle and cluster reachability model.'],
    },
    {
      cells: ['Pod template annotations', 'Config and certificate hashes that trigger rollout when rendered state changes.', 'Rendered changes need a safe, predictable rollout boundary.'],
    },
    {
      cells: ['Static unseal material', 'Operator-generated unseal Secret and mount wiring when static seal mode is used.', 'Seal bootstrap has to stay consistent with the rendered config and mounted files.'],
    },
  ]}
/>

## Render-Then-Apply Flow

<DiagramFrame
  title="Render then apply"
  caption="The infrastructure manager renders workload resources first, then applies only what changed. Hash annotations on the pod template convert rendered config drift into safe Kubernetes rollout behavior."
  code={`graph TD
    Spec["OpenBaoCluster spec"] --> Render["Render config and resources"]
    Render --> Config["config.hcl"]
    Render --> Resources["StatefulSet / Services / Secrets"]

    Config --> ConfigHash["Compute config hash"]
    Resources --> Diff["Detect resource drift"]

    ConfigHash --> Annotate["Update pod-template annotations"]
    Diff --> Apply["Apply changed resources"]
    Annotate --> Rollout["StatefulSet rollout"]
    Apply --> Rollout

    classDef process fill:transparent,stroke:#fdd0a4,stroke-width:2px,color:#f8fafc;
    classDef write fill:transparent,stroke:#87d6be,stroke-width:2px,color:#e6f4ef;
    classDef read fill:transparent,stroke:#79c0ab,stroke-width:2px,color:#e6f4ef;

    class Spec read;
    class Render,ConfigHash,Diff process;
    class Config,Resources,Annotate,Apply,Rollout write;`}
/>

## Configuration And Seal Rendering

The manager does not apply a static ConfigMap. It renders the config from the cluster spec and the selected integration modes.

```hcl
listener "tcp" {
  address         = "0.0.0.0:8200"
  cluster_address = "0.0.0.0:8201"
  tls_cert_file   = "/etc/bao/tls/tls.crt"
  tls_key_file    = "/etc/bao/tls/tls.key"
}

storage "raft" {
  path    = "/bao/data"
  node_id = "${HOSTNAME}"

  retry_join {
    auto_join              = "provider=k8s label_selector=\"openbao.org/cluster=prod-cluster\""
    leader_tls_servername  = "openbao-cluster-prod-cluster.local"
  }
}
```

<Tabs groupId="infra-static-seal-external-kms">
  <TabItem value="static" label="Static seal">

The manager generates the unseal material, stores it in `Secret/<cluster>-unseal-key`, mounts it into the pod, and renders a `seal "static"` stanza that points at the mounted file.

  </TabItem>
  <TabItem value="external" label="External KMS">

When `spec.unseal.type` points at a cloud KMS integration, the manager stops generating unseal material and renders the provider-specific seal stanza from the declared credentials and parameters.

  </TabItem>
</Tabs>

<Callout type="note" title="TLS mode changes workload rendering">

TLS mode affects both rendered config and mounted resource expectations. `OperatorManaged`, `External`, and `ACME` are not only certificate sources, they change what the workload pod expects on disk and what the hot-reload path watches.

</Callout>

## Safety Boundaries

<DecisionTable
  kind="reference"
  title="Safety boundaries"
  columns={['Concern', 'Manager behavior']}
  rows={[
    {
      cells: ['Config drift', 'Rendered config changes are converted into hash-based rollout triggers instead of relying on manual restarts.'],
      emphasis: 'recommended',
    },
    {
      cells: ['Image verification', 'When verification is enabled, the manager blocks or warns before unsafe workload images are applied, depending on policy.'],
    },
    {
      cells: ['Least privilege in multi-tenant mode', 'The workload path avoids broad tenant list/watch access and prefers direct reads plus requeue-based polling.'],
    },
    {
      cells: ['Lifecycle ownership', 'ConfigMaps, Services, Secrets, and StatefulSet resources stay owned by the OpenBaoCluster contract rather than becoming user-managed side effects.'],
    },
  ]}
/>

<NextActions
  title="Related deep dives"
  items={[
    {
      label: 'Cert manager',
      description: 'Follow how certificate sources, rotation, and hot reload affect what the workload path renders.',
      docId: 'architecture/cert-manager',
    },
    {
      label: 'Init manager',
      description: 'See how the workload path hands off from first-boot infrastructure into cluster initialization.',
      docId: 'architecture/init-manager',
    },
    {
      label: 'Configure the cluster',
      description: 'Compare the internal rendering model with the user-facing configuration guides.',
      docId: 'user-guide/openbaocluster/configuration/index',
    },
  ]}
/>
