---
title: Resources and Storage
hide_title: true
pageType: task
journey: configure
description: Choose storage class, PVC size, and workload resource requests for the cluster data path and capacity baseline.
---

<PageHeader
  title="Storage and workload sizing"
  lede="The operator renders the workload, but storage class and capacity choices still belong to the platform baseline. Use this page to set resource requests, understand PVC growth behavior, and make storage decisions explicit."
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
        "Audit file storage PVC",
        "Creates or mounts one RWX PVC when `spec.auditFileStorage` is configured.",
        "Choose storage that supports multi-node mounts, encryption at rest, and writable ownership for the OpenBao Pod security context.",
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
  Set explicit requests and an explicit `storageClassName` in production. Defaults are acceptable for evaluation, but they provide less predictable long-term storage behavior.
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
        "A size increase is not complete until the PVC and cluster conditions confirm it.",
      ],
      emphasis: "caution",
    },
  ]}
/>

## Size audit file storage

`spec.auditFileStorage` is separate from the Raft data path. Use it only when
file audit devices need a filesystem handoff for a collector.

<CommandBlock
  language="yaml"
  label="configure"
  title="Create a managed RWX audit PVC"
  code={`apiVersion: openbao.org/v1alpha1
kind: OpenBaoCluster
metadata:
  name: prod-cluster
spec:
  auditFileStorage:
    mode: ManagedPVC
    size: "20Gi"
    storageClassName: "rwx-encrypted"
  audit:
    - type: file
      path: file
      fileOptions:
        file_path: "/openbao/audit/audit.jsonl"
        format: "json"`}
>
  Set the size for expected collector lag, replay needs, and failure recovery. Long-term retention belongs in the downstream log or archive system.
</CommandBlock>

<DecisionTable
  kind="reference"
  title="Audit storage requirements"
  columns={["Requirement", "What to verify", "Operational note"]}
  rows={[
    {
      cells: [
        "ReadWriteMany",
        "The PVC must include `ReadWriteMany` and reach `Bound`.",
        "`AuditFileStorageReady=False` reports missing, pending, or non-RWX claims before the workload can be considered ready.",
      ],
      emphasis: "recommended",
    },
    {
      cells: [
        "Writable ownership",
        "The mounted path must be writable by the OpenBao runtime user or group.",
        "On standard Kubernetes the Pods run as UID `100` and GID `1000` with `fsGroup: 1000`. Verify that the CSI driver honors that, or pre-provision ownership for existing claims.",
      ],
    },
    {
      cells: [
        "Encryption and node placement",
        "The backing storage must match the sensitivity of audit records.",
        "Audit records can contain request metadata. Keep storage encryption, node access, and platform-admin access in the security review.",
      ],
    },
    {
      cells: [
        "Capacity and cleanup",
        "The PVC must absorb collector outages without filling the filesystem.",
        "Define alerting and cleanup outside the operator. The operator does not rotate, prune, or archive audit files.",
      ],
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
  title="Check audit storage readiness"
  code={`kubectl get openbaocluster <name> -n <namespace> \\
  -o jsonpath='{range .status.conditions[?(@.type=="AuditFileStorageReady")]}{.status}{"\\t"}{.reason}{"\\t"}{.message}{"\\n"}{end}'

kubectl get pvc -n <namespace> <audit-pvc-name> \\
  -o jsonpath='{.status.phase}{"\\t"}{.spec.accessModes}{"\\n"}'`}
>
  Use the condition for the operator view and the PVC command for the underlying Kubernetes storage state.
</CommandBlock>

<CommandBlock
  language="bash"
  label="verify"
  title="Check the cluster storage condition"
  code={`kubectl get openbaocluster <name> -n <namespace> \\
  -o jsonpath='{range .status.conditions[*]}{.type}={.status}{"\\t"}{.reason}{"\\n"}{end}'`}
>
  A healthy cluster should eventually report `StorageConfigured=True`. If it does not, fix the storage-path mismatch before continuing with upgrades or backups.
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
      description: "Configure backups against the storage path the cluster will use in steady state.",
      docId: "user-guide/openbaocluster/operations/backups",
    },
  ]}
/>
