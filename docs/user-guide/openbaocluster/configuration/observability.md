---
title: Observability
hide_title: true
pageType: task
journey: configure
description: Configure operator metrics, cluster telemetry, dashboards, alerts, and logs for routine operations and incident response.
---

<PageHeader
  title="Observability for operator and workload"
  lede="OpenBao Operator has two observability layers: the operator control plane itself, and the OpenBao workload it renders. Use this page to wire both layers into your monitoring stack, choose the scrape model your platform already supports, and focus on the signals that matter for upgrades, backups, and recovery."
/>



<DecisionTable
  title="Observe the right surface"
  columns={["Surface", "Configure it through", "Use it for", "Watch for"]}
  rows={[
    {
      cells: [
        "Operator metrics",
        "Helm or Kustomize settings on the operator installation",
        "Controller and provisioner reconcile health, errors, and platform-level backup or upgrade counters.",
        "The endpoint is HTTPS and RBAC-protected. Your scraper needs both network reachability and permission to GET `/metrics`.",
      ],
      emphasis: "recommended",
    },
    {
      cells: [
        "OpenBao workload telemetry",
        "The `spec.observability.metrics` block on each `OpenBaoCluster`, plus optional `spec.telemetry` overrides.",
        "Application-level metrics from the OpenBao Pods themselves.",
        "Configure this separately from operator metrics. Enabling one layer does not configure the other.",
      ],
    },
    {
      cells: [
        "OpenBao audit logs",
        "`spec.audit` plus `spec.auditFileStorage`, then a collector that reads the audit PVC.",
        "Security and compliance event trails from the OpenBao audit device.",
        "Audit records are sensitive. The PVC is a handoff buffer, not the final retention boundary.",
      ],
    },
    {
      cells: [
        "Logs and health probes",
        "Operator install values such as log level and health probe settings.",
        "Fast incident triage when the issue is not obvious from metrics alone.",
        "Use debug logging intentionally and temporarily, then return to the normal log level.",
      ],
    },
    {
      cells: [
        "Dashboards and alerts",
        "Grafana assets under `config/grafana/` and your own Prometheus or Alertmanager rules.",
        "A small, repeatable operator cockpit for upgrades, backups, and cluster readiness.",
        "Use dashboards for context and alerts for time-sensitive failures.",
      ],
    },
  ]}
/>

## Wire operator metrics

<Tabs groupId="operator-observability-prometheus-vmagent-plain">
  <TabItem value="prometheus-operator" label="Prometheus Operator">

<CommandBlock
  language="yaml"
  label="configure"
  title="Enable operator metrics with ServiceMonitor resources"
  code={`metrics:
  enabled: true
  rbac:
    enabled: true
    subjects:
      - name: prometheus-k8s
        namespace: monitoring
  serviceMonitor:
    enabled: true
    namespace: monitoring
    interval: 30s
    scrapeTimeout: 10s
    tlsConfig:
      insecureSkipVerify: true`}
>
  This is the cleanest path when Prometheus Operator is already your cluster standard. It creates ServiceMonitors for the controller and, in multi-tenant mode, the provisioner metrics Services.
</CommandBlock>

  </TabItem>
  <TabItem value="victoria-metrics" label="VictoriaMetrics Operator">

<CommandBlock
  language="yaml"
  label="configure"
  title="Enable operator metrics with VMServiceScrape resources"
  code={`metrics:
  enabled: true
  rbac:
    enabled: true
    subjects:
      - name: vmagent
        namespace: monitoring
  victoriaMetrics:
    enabled: true
    namespace: monitoring
    interval: 30s
    scrapeTimeout: 10s
    tlsConfig:
      insecureSkipVerify: true`}
>
  Use this when VictoriaMetrics is your standard scrape controller. The same HTTPS and RBAC constraints still apply to the metrics endpoint.
</CommandBlock>

  </TabItem>
  <TabItem value="plain-prometheus" label="Plain Prometheus">

<CommandBlock
  language="yaml"
  label="configure"
  title="Scrape the operator metrics services directly"
  code={`scrape_configs:
  - job_name: openbao-operator-controller
    scheme: https
    metrics_path: /metrics
    bearer_token_file: /var/run/secrets/kubernetes.io/serviceaccount/token
    tls_config:
      insecure_skip_verify: true
    static_configs:
      - targets:
          - <controller-metrics-service>.<operator-namespace>.svc:8443

  - job_name: openbao-operator-provisioner
    scheme: https
    metrics_path: /metrics
    bearer_token_file: /var/run/secrets/kubernetes.io/serviceaccount/token
    tls_config:
      insecure_skip_verify: true
    static_configs:
      - targets:
          - <provisioner-metrics-service>.<operator-namespace>.svc:8443`}
>
  Use this path when you do not run a scrape operator. Keep the ServiceAccount permission to GET `/metrics` and the TLS assumptions explicit.
</CommandBlock>

  </TabItem>
</Tabs>

<Callout type="note" title="Network policy still applies to scrapers">

If operator network policy is enabled, the monitoring namespace must carry the labels expected by `networkPolicy.metricsAllowedNamespaceLabels` so the scraper can actually reach the HTTPS metrics service.

</Callout>

## Enable OpenBao workload telemetry deliberately

<CommandBlock
  language="yaml"
  label="configure"
  title="Turn on workload telemetry for an OpenBaoCluster"
  code={`apiVersion: openbao.org/v1alpha1
kind: OpenBaoCluster
metadata:
  name: prod-cluster
spec:
  observability:
    metrics:
      enabled: true
      serviceMonitor:
        enabled: true
        interval: "30s"
        scrapeTimeout: "10s"`}
>
  This enables the OpenBao telemetry stanza with safe defaults and creates an active-node metrics Service plus a Prometheus Operator ServiceMonitor when that is the scrape model you use. Reach for `spec.telemetry` when you need lower-level OpenBao telemetry tuning.
</CommandBlock>

<CommandBlock
  language="yaml"
  label="configure"
  title="Configure authenticated workload scraping"
  code={`apiVersion: openbao.org/v1alpha1
kind: OpenBaoCluster
metadata:
  name: prod-cluster
spec:
  observability:
    metrics:
      enabled: true
      serviceMonitor:
        enabled: true
        interval: "30s"
        scrapeTimeout: "10s"
        labels:
          release: kube-prometheus-stack
        authorization:
          credentialsSecret:
            name: openbao-metrics-token
            key: token
        tlsConfig:
          serverName: prod-cluster-metrics.openbao.svc
          caConfigMap:
            name: prod-cluster-metrics-ca
            key: ca.crt`}
>
  Use this shape for production Prometheus Operator scraping. The Secret should contain a scoped OpenBao token that can read `sys/metrics`, and the CA reference should validate the OpenBao serving certificate.
</CommandBlock>

<Callout type="important" title="Hardened ServiceMonitor TLS">

Hardened clusters reject `spec.observability.metrics.serviceMonitor.tlsConfig.insecureSkipVerify: true`. Configure `serverName` and a CA reference instead of disabling certificate verification.

</Callout>

<CommandBlock
  language="yaml"
  label="configure"
  title="Scrape every OpenBao node through a metrics-only listener"
  code={`apiVersion: openbao.org/v1alpha1
kind: OpenBaoCluster
metadata:
  name: prod-cluster
spec:
  observability:
    metrics:
      enabled: true
      scrapeProfile: AllNodes
      metricsOnlyListener:
        port: 8202
        unauthenticatedMetricsAccess: true
      serviceMonitor:
        enabled: true
        labels:
          release: kube-prometheus-stack`}
>
  Use this profile when you need standby-node and per-node Raft visibility. The operator renders a dedicated metrics listener, a headless metrics Service, and ServiceMonitor relabeling for the pod and node names.
</CommandBlock>

The default `Active` scrape profile targets the active OpenBao pod on the API
listener using `/v1/sys/metrics?format=prometheus`. The `AllNodes` profile
targets every OpenBao pod through the metrics-only listener. Keep the metrics
Service reachable only from your monitoring namespace with NetworkPolicy when
unauthenticated metrics access is enabled.

## Collect OpenBao audit logs

<CommandBlock
  language="yaml"
  label="configure"
  title="Write audit records to a collector handoff PVC"
  code={`apiVersion: openbao.org/v1alpha1
kind: OpenBaoCluster
metadata:
  name: prod-cluster
spec:
  auditFileStorage:
    mode: ExistingPVC
    existingClaimName: prod-cluster-audit-rwx
  audit:
    - type: file
      path: file
      description: "File audit log for collection"
      fileOptions:
        file_path: "/openbao/audit/audit.jsonl"
        format: "json"`}
>
  Use this when a log collector such as Alloy should read OpenBao audit records from a shared filesystem. Each OpenBao Pod writes beneath its own PVC subdirectory while the rendered file path inside the Pod stays stable.
</CommandBlock>

Mount the audit PVC read-only into the collector and ship records to the log
system that owns search, retention, and compliance controls. The
[OpenBao Observability Reference Architecture](https://github.com/dc-tec/openbao-observability)
shows the intended companion pattern for Alloy, Loki, and Grafana assets.

<DecisionTable
  kind="reference"
  title="Audit collection boundaries"
  columns={["Boundary", "Recommendation", "Why it matters"]}
  rows={[
    {
      cells: [
        "OpenBao Pods",
        "Write only to paths under `spec.auditFileStorage.mountPath`.",
        "The operator validates file audit paths so records land on the mounted audit PVC instead of the read-only container filesystem.",
      ],
      emphasis: "recommended",
    },
    {
      cells: [
        "Collector",
        "Mount the audit PVC read-only and scope the collector to the cluster namespace.",
        "Audit records can contain sensitive request metadata and must not become writable from the collection path.",
      ],
    },
    {
      cells: [
        "Archive",
        "Move records from the handoff PVC to external retention-controlled storage.",
        "PVC storage is useful for buffering and replay, but it is not a tamper-proof compliance archive.",
      ],
    },
  ]}
/>

<Callout type="warning" title="Audit files are sensitive logs">

Restrict who can create Pods that mount the audit PVC, who can `exec` into OpenBao or collector Pods, and who can administer the backing storage. Kubernetes RBAC on the PVC object does not protect file contents once another workload can mount the volume.

</Callout>

<DecisionTable
  kind="reference"
  title="Promote a small set of signals first"
  columns={["Concern", "What to watch", "Why it matters"]}
  rows={[
    {
      cells: [
        "Availability",
        "`openbao_cluster_ready_replicas` and cluster conditions such as `Available` or `Degraded`",
        "This tells you whether the cluster is serving traffic rather than only whether Pods exist.",
      ],
      emphasis: "recommended",
    },
    {
      cells: [
        "Steady read pool",
        "`openbao_cluster_read_replicas_desired`, `_ready`, `_registered`, `_healthy`, plus read-replica conditions such as `ReadServingAvailable` and `ReadReplicasAutopilotHealthy`",
        "This tells you whether the steady read tier exists, has actually joined, and is still healthy enough for the topology you placed it in.",
      ],
    },
    {
      cells: [
        "Backup freshness",
        "`openbao_backup_last_success_timestamp`, `openbao_backup_consecutive_failures`, and the backup status conditions",
        "These signals show whether the snapshots you plan to restore are current and successful.",
      ],
    },
    {
      cells: [
        "Upgrade safety",
        "`openbao_upgrade_in_progress`, `openbao_upgrade_failure_total`, and rollback counters",
        "These signals distinguish orchestrated upgrade activity from normal steady state.",
      ],
    },
    {
      cells: [
        "Controller health",
        "`openbao_reconcile_errors_total` and sustained reconcile duration spikes",
        "This exposes control-plane failures before they become cluster-wide drift or stalled operations.",
      ],
    },
  ]}
/>

<CommandBlock
  language="yaml"
  label="apply"
  title="Start with focused alert rules"
  code={`apiVersion: monitoring.coreos.com/v1
kind: PrometheusRule
metadata:
  name: openbao-operator-alerts
spec:
  groups:
    - name: openbao-operator
      rules:
        - alert: OpenBaoClusterDown
          expr: openbao_cluster_ready_replicas == 0
          for: 1m
        - alert: OpenBaoBackupStale
          expr: time() - openbao_backup_last_success_timestamp > 86400
          for: 15m
        - alert: OpenBaoReadReplicaPoolDegraded
          expr: openbao_cluster_read_replicas_desired > openbao_cluster_read_replicas_healthy
          for: 10m
        - alert: OpenBaoReadReplicaPoolNotRegistered
          expr: openbao_cluster_read_replicas_desired > openbao_cluster_read_replicas_registered
          for: 10m
        - alert: OpenBaoReconcileErrors
          expr: rate(openbao_reconcile_errors_total[5m]) > 0.1
          for: 10m`}
>
  Keep the first alert set small. Availability, backup freshness, sustained read-pool degradation, and sustained reconcile failure are the highest-value starting signals.
</CommandBlock>

<Callout type="note" title="Alert rules are not operator-managed">

The OpenBaoCluster controller manages the workload `ServiceMonitor` when `spec.observability.metrics` enables it. `PrometheusRule` resources are user-applied observability objects, so the ServiceAccount or GitOps controller applying the alert rules needs `monitoring.coreos.com` `prometheusrules` permissions in that namespace.

</Callout>

## Dashboards, logs, and health

<CommandBlock
  language="bash"
  label="apply"
  title="Install the bundled Grafana dashboards"
  code={`kubectl apply -k config/grafana -n monitoring`}
>
  The per-feature dashboards under `config/grafana/dashboards/` are the better starting point. The old monolithic dashboard still exists, but it is no longer the recommended default.
</CommandBlock>

<CommandBlock
  language="yaml"
  label="configure"
  title="Raise log detail temporarily during investigation"
  code={`controller:
  extraArgs:
    - --zap-log-level=debug
    - --zap-stacktrace-level=error`}
>
  Use debug logging only long enough to capture the behavior you need. Reset to the normal log level once the incident or rollout check is complete.
</CommandBlock>

<Callout type="tip" title="Keep operator metrics and workload telemetry separate in your dashboards">

Build dashboards that show both surfaces together and still make it obvious whether a failure is in the operator control plane or in the OpenBao workload itself.

</Callout>

<Callout type="note" title="The overview dashboard now includes the steady read pool">

`config/grafana/dashboards/overview.json` now shows desired, ready, registered, and Autopilot-healthy read-replica counts next to the existing cluster-level signals. Use that view for the first operational pass, then build more topology-specific dashboards if your placement strategy needs them.

</Callout>

<NextActions
  title="Keep the operational loop tight"
  items={[
    {
      label: "Configure backups",
      description: "Configure backup telemetry before restore depends on it.",
      docId: "user-guide/openbaocluster/operations/backups",
    },
    {
      label: "Plan upgrades",
      description: "Use upgrade metrics and rollback signals to make rollout behavior observable before the first version change.",
      docId: "user-guide/openbaocluster/operations/upgrades",
    },
    {
      label: "Troubleshoot the cluster",
      description: "Move from baseline telemetry into symptom-driven incident routing when the service stops behaving normally.",
      docId: "user-guide/openbaocluster/operations/troubleshooting",
    },
  ]}
/>
