---
title: Observability
hide_title: true
pageType: task
journey: configure
description: Wire operator metrics, cluster telemetry, dashboards, alerts, and logs before you have to troubleshoot the service under pressure.
---

<PageHero
  eyebrow="Configure / Platform Readiness"
  title="Observe both the operator and the workload before you call the cluster ready."
  lede="OpenBao Operator has two observability layers: the operator control plane itself, and the OpenBao workload it renders. Use this page to wire both layers into your monitoring stack, choose the scrape model your platform already supports, and promote only the signals that help you operate upgrades, backups, and recovery."
  actions={[
    {label: "Open production checklist", docId: "user-guide/openbaocluster/operations/production-checklist", variant: "primary"},
    {label: "Open troubleshooting", docId: "user-guide/openbaocluster/operations/troubleshooting", variant: "secondary"},
  ]}
>
  <Checklist
    title="Use this page when you need to"
    items={[
      "scrape controller and provisioner metrics from the operator installation",
      "enable telemetry on the OpenBao workload without turning this page into a generic metrics reference",
      "promote a small set of alerts for availability, backups, upgrades, and reconcile health",
      "keep dashboards, logs, and health probes available before the first real incident",
    ]}
  />
</PageHero>

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
        "This is separate from operator metrics. Do not assume enabling one layer automatically covers the other.",
      ],
    },
    {
      cells: [
        "Logs and health probes",
        "Operator install values such as log level and health probe settings.",
        "Fast incident triage when the issue is not obvious from metrics alone.",
        "Use debug logging intentionally and temporarily. Do not leave broad debug enabled as the long-term default.",
      ],
    },
    {
      cells: [
        "Dashboards and alerts",
        "Grafana assets under `config/grafana/` and your own Prometheus or Alertmanager rules.",
        "A small, repeatable operator cockpit for upgrades, backups, and cluster readiness.",
        "Dashboards should support decisions. They should not become an excuse to avoid explicit alerts on the failure modes that matter.",
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
  Use this only when you do not run a scrape operator. Keep the ServiceAccount permission to GET `/metrics` and the TLS assumptions explicit.
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
  This enables the OpenBao telemetry stanza with safe defaults and creates a Prometheus Operator ServiceMonitor when that is the scrape model you use. Use `spec.telemetry` only when you need lower-level OpenBao telemetry tuning.
</CommandBlock>

<DecisionTable
  kind="reference"
  title="Promote a small set of signals first"
  columns={["Concern", "What to watch", "Why it matters"]}
  rows={[
    {
      cells: [
        "Availability",
        "`openbao_cluster_ready_replicas` and cluster conditions such as `Available` or `Degraded`",
        "This tells you whether the cluster is actually serving, not just whether Pods exist.",
      ],
      emphasis: "recommended",
    },
    {
      cells: [
        "Backup freshness",
        "`openbao_backup_last_success_timestamp`, `openbao_backup_consecutive_failures`, and the backup status conditions",
        "Restore is only as real as the last backup you can prove succeeded.",
      ],
    },
    {
      cells: [
        "Upgrade safety",
        "`openbao_upgrade_in_progress`, `openbao_upgrade_failure_total`, and rollback counters",
        "Upgrades should be observable as controlled workflows, not silent StatefulSet churn.",
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
        - alert: OpenBaoReconcileErrors
          expr: rate(openbao_reconcile_errors_total[5m]) > 0.1
          for: 10m`}
>
  Keep the first alert set small. Availability, backup freshness, and sustained reconcile failure are the signals that change operator behavior fastest.
</CommandBlock>

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

The most useful dashboards show both surfaces together, but they should still make it obvious whether a failure is in the operator control plane or in the OpenBao workload itself.

</Callout>

<NextActions
  title="Keep the operational loop tight"
  items={[
    {
      label: "Configure backups",
      description: "Backups are the first place good observability pays off. Wire them before you depend on restore.",
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
