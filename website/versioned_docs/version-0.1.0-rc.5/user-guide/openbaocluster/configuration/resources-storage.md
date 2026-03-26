---
title: Resources and Storage
hide_title: true
pageType: task
journey: configure
description: Choose storage class, PVC size, and workload resource requests before the cluster becomes difficult to resize or move.
---

<PageHeader
  title="Choose storage and workload limits before the data path makes them expensive to change."
  lede="The operator renders the core workload for you, but it does not choose the platform capacity or storage class you intend to live with. Use this page to understand which resources the cluster owns, how PVC growth works, and where explicit sizing is safer than inheriting whatever the platform happens to default."
/>

<DecisionTable
  title="What the operator manages for an OpenBaoCluster"
  columns={["Surface", "What it does", "What still belongs to you"]}
  rows={[
    {
      cells: [
        "StatefulSet and Pod template",
        "Renders the OpenBao Pods, init container, probes, mounts, labels, and rollout behavior.",
        "Choose resource requests, limits, and the cluster shape that the generated Pods should follow.",
      ],
      emphasis: "recommended",
    },
    {
      cells: [
        "Services, ConfigMaps, and Secrets",
        "Creates the workload-facing Service surfaces plus the rendered configuration and runtime Secrets required by the chosen profile.",
        "Own the service-boundary decision, TLS ownership model, and any external secrets or certificate material that are not operator-managed.",
      ],
    },
    {
      cells: [
        "Data PVCs",
        "Creates one PVC per replica from the StatefulSet claim template and patches existing PVC size when you increase storage.",
        "Choose the correct StorageClass up front and verify that the underlying CSI driver supports the expansion behavior you expect.",
      ],
    },
    {
      cells: [
        "Default NetworkPolicy",
        "Applies the operator-managed baseline traffic rules for Pods in the cluster.",
        "Add any extra ingress or egress rules your environment requires and validate them against backup, restore, and edge traffic.",
      ],
    },
  ]}
/>

<DiagramFrame
  title="Managed resource footprint"
  caption="The OpenBaoCluster spec drives a rendered workload. The operator owns the generated Kubernetes resources, but the platform choices behind storage, capacity, and external dependencies still need to be deliberate."
  code={`flowchart LR
    CR["OpenBaoCluster"] --> STS["StatefulSet"]
    CR --> SVC["Services"]
    CR --> CFG["ConfigMaps and Secrets"]
    CR --> NET["NetworkPolicy"]
    STS --> PVC["Per-replica data PVCs"]
    STS --> Pods["OpenBao Pods"]

    classDef read fill:transparent,stroke:#79c0ab,stroke-width:2px,color:#e6f4ef;
    classDef write fill:transparent,stroke:#87d6be,stroke-width:2px,color:#e6f4ef;

    class CR read;
    class STS,SVC,CFG,NET,PVC,Pods write;`}
/>

## Set the baseline explicitly

<CommandBlock
  language="yaml"
  label="configure"
  title="Set storage and workload requests up front"
  code={`apiVersion: openbao.org/v1alpha1
kind: OpenBaoCluster
metadata:
  name: prod-cluster
  namespace: openbao
spec:
  version: "2.5.0"
  profile: Hardened
  replicas: 3
  resources:
    requests:
      cpu: "500m"
      memory: "1Gi"
    limits:
      cpu: "1000m"
      memory: "2Gi"
  storage:
    size: "50Gi"
    storageClassName: "fast-ssd"`}
>
  Start with explicit requests and an explicit `storageClassName` in production. Defaults are fine for evaluation, but they are a weak contract once the cluster carries real data.
</CommandBlock>

<DecisionTable
  kind="reference"
  title="Storage rules that become expensive later"
  columns={["Choice", "Operator behavior", "Why it matters"]}
  rows={[
    {
      cells: [
        "`spec.storage.storageClassName`",
        "The effective storage class becomes immutable after the first PVCs are created.",
        "Pick it before first reconcile if you care about IOPS, topology, encryption, or cost. Do not plan on fixing it in place later.",
      ],
      emphasis: "recommended",
    },
    {
      cells: [
        "`spec.storage.size`",
        "The operator supports expansion only. Decreasing size is rejected.",
        "Plan growth, not shrinkage. If the first size is too small, you can grow it, but you cannot safely reverse it through the API.",
      ],
    },
    {
      cells: [
        "Default StorageClass",
        "If you omit `storageClassName`, Kubernetes uses the cluster default when PVCs are created.",
        "That may be acceptable in development, but in production it is better to make the storage path explicit and auditable.",
      ],
    },
    {
      cells: [
        "Filesystem expansion",
        "Some CSI drivers finish expansion only after a restart. The operator surfaces that and can use a controlled restart path when maintenance is enabled.",
        "Do not assume a size increase is complete just because the PVC request changed. Check the PVC and cluster conditions before you move on.",
      ],
      emphasis: "caution",
    },
  ]}
/>

## Inspect the rendered storage state

<CommandBlock
  language="bash"
  label="inspect"
  title="Inspect the data PVCs for a cluster"
  code={`kubectl get pvc -n <namespace> -l openbao.org/cluster=<name>`}
>
  Check the requested size, bound StorageClass, and whether any PVC reports `FileSystemResizePending`.
</CommandBlock>

<CommandBlock
  language="bash"
  label="verify"
  title="Check the cluster storage condition"
  code={`kubectl get openbaocluster <name> -n <namespace> \\
  -o jsonpath='{range .status.conditions[*]}{.type}={.status}{"\\t"}{.reason}{"\\n"}{end}'`}
>
  A healthy cluster should eventually report `StorageConfigured=True`. If it does not, fix the storage-path mismatch before you continue with upgrades or backups.
</CommandBlock>

<Callout type="note" title="Controlled restarts still matter after a PVC expansion">

If your CSI driver requires a restart to finish filesystem resize, use the maintenance workflow instead of bouncing Pods ad hoc. The operator can only take the controlled restart path when `spec.maintenance.enabled=true`.

</Callout>

<NextActions
  title="Continue platform readiness"
  items={[
    {
      label: "Observability",
      description: "Wire metrics, dashboards, and alerting before the first storage or rollout problem becomes a blind incident.",
      docId: "user-guide/openbaocluster/configuration/observability",
    },
    {
      label: "Air-gapped and private registries",
      description: "Use the disconnected-environment guide when image sources, pull secrets, or mirrored helper images are part of the platform contract.",
      docId: "user-guide/openbaocluster/configuration/air-gapped",
    },
    {
      label: "Configure backups",
      description: "Snapshots depend on the storage path you chose and should be routine long before you need restore.",
      docId: "user-guide/openbaocluster/operations/backups",
    },
  ]}
/>
