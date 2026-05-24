"use strict";(self.webpackChunkwebsite=self.webpackChunkwebsite||[]).push([["6222"],{73424(e,r,t){t.r(r),t.d(r,{metadata:()=>o,default:()=>p,frontMatter:()=>i,contentTitle:()=>n,toc:()=>c,assets:()=>l});var o=JSON.parse('{"id":"user-guide/openbaocluster/configuration/observability","title":"Observability","description":"Configure operator metrics, cluster telemetry, dashboards, alerts, and logs for routine operations and incident response.","source":"@site/../docs/user-guide/openbaocluster/configuration/observability.md","sourceDirName":"user-guide/openbaocluster/configuration","slug":"/user-guide/openbaocluster/configuration/observability","permalink":"/openbao-operator/docs/next/user-guide/openbaocluster/configuration/observability","draft":false,"unlisted":false,"editUrl":"https://github.com/dc-tec/openbao-operator/edit/main/docs/user-guide/openbaocluster/configuration/observability.md","tags":[],"version":"current","lastUpdatedBy":"Roel de Cort","lastUpdatedAt":1779640660000,"frontMatter":{"title":"Observability","hide_title":true,"pageType":"task","journey":"configure","description":"Configure operator metrics, cluster telemetry, dashboards, alerts, and logs for routine operations and incident response."},"sidebar":"operatorDocs","previous":{"title":"Resources and storage","permalink":"/openbao-operator/docs/next/user-guide/openbaocluster/configuration/resources-storage"},"next":{"title":"Air-gapped and private registries","permalink":"/openbao-operator/docs/next/user-guide/openbaocluster/configuration/air-gapped"}}'),a=t(74848),s=t(28453);let i={title:"Observability",hide_title:!0,pageType:"task",journey:"configure",description:"Configure operator metrics, cluster telemetry, dashboards, alerts, and logs for routine operations and incident response."},n,l={},c=[{value:"Wire operator metrics",id:"wire-operator-metrics",level:2},{value:"Enable OpenBao workload telemetry deliberately",id:"enable-openbao-workload-telemetry-deliberately",level:2},{value:"Dashboards, logs, and health",id:"dashboards-logs-and-health",level:2}];function d(e){let r={code:"code",h2:"h2",p:"p",...(0,s.R)(),...e.components},{Callout:t,CommandBlock:o,DecisionTable:i,NextActions:n,PageHeader:l,TabItem:c,Tabs:d}=r;return t||u("Callout",!0),o||u("CommandBlock",!0),i||u("DecisionTable",!0),n||u("NextActions",!0),l||u("PageHeader",!0),c||u("TabItem",!0),d||u("Tabs",!0),(0,a.jsxs)(a.Fragment,{children:[(0,a.jsx)(l,{title:"Observability for operator and workload",lede:"OpenBao Operator has two observability layers: the operator control plane itself, and the OpenBao workload it renders. Use this page to wire both layers into your monitoring stack, choose the scrape model your platform already supports, and focus on the signals that matter for upgrades, backups, and recovery."}),"\n",(0,a.jsx)(i,{title:"Observe the right surface",columns:["Surface","Configure it through","Use it for","Watch for"],rows:[{cells:["Operator metrics","Helm or Kustomize settings on the operator installation","Controller and provisioner reconcile health, errors, and platform-level backup or upgrade counters.","The endpoint is HTTPS and RBAC-protected. Your scraper needs both network reachability and permission to GET `/metrics`."],emphasis:"recommended"},{cells:["OpenBao workload telemetry","The `spec.observability.metrics` block on each `OpenBaoCluster`, plus optional `spec.telemetry` overrides.","Application-level metrics from the OpenBao Pods themselves.","Configure this separately from operator metrics. Enabling one layer does not configure the other."]},{cells:["Logs and health probes","Operator install values such as log level and health probe settings.","Fast incident triage when the issue is not obvious from metrics alone.","Use debug logging intentionally and temporarily, then return to the normal log level."]},{cells:["Dashboards and alerts","Grafana assets under `config/grafana/` and your own Prometheus or Alertmanager rules.","A small, repeatable operator cockpit for upgrades, backups, and cluster readiness.","Use dashboards for context and alerts for time-sensitive failures."]}]}),"\n",(0,a.jsx)(r.h2,{id:"wire-operator-metrics",children:"Wire operator metrics"}),"\n",(0,a.jsxs)(d,{groupId:"operator-observability-prometheus-vmagent-plain",children:[(0,a.jsx)(c,{value:"prometheus-operator",label:"Prometheus Operator",children:(0,a.jsx)(o,{language:"yaml",label:"configure",title:"Enable operator metrics with ServiceMonitor resources",code:`metrics:
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
    insecureSkipVerify: true`,children:(0,a.jsx)(r.p,{children:"This is the cleanest path when Prometheus Operator is already your cluster standard. It creates ServiceMonitors for the controller and, in multi-tenant mode, the provisioner metrics Services."})})}),(0,a.jsx)(c,{value:"victoria-metrics",label:"VictoriaMetrics Operator",children:(0,a.jsx)(o,{language:"yaml",label:"configure",title:"Enable operator metrics with VMServiceScrape resources",code:`metrics:
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
    insecureSkipVerify: true`,children:(0,a.jsx)(r.p,{children:"Use this when VictoriaMetrics is your standard scrape controller. The same HTTPS and RBAC constraints still apply to the metrics endpoint."})})}),(0,a.jsx)(c,{value:"plain-prometheus",label:"Plain Prometheus",children:(0,a.jsx)(o,{language:"yaml",label:"configure",title:"Scrape the operator metrics services directly",code:`scrape_configs:
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
        - <provisioner-metrics-service>.<operator-namespace>.svc:8443`,children:(0,a.jsxs)(r.p,{children:["Use this path when you do not run a scrape operator. Keep the ServiceAccount permission to GET ",(0,a.jsx)(r.code,{children:"/metrics"})," and the TLS assumptions explicit."]})})})]}),"\n",(0,a.jsx)(t,{type:"note",title:"Network policy still applies to scrapers",children:(0,a.jsxs)(r.p,{children:["If operator network policy is enabled, the monitoring namespace must carry the labels expected by ",(0,a.jsx)(r.code,{children:"networkPolicy.metricsAllowedNamespaceLabels"})," so the scraper can actually reach the HTTPS metrics service."]})}),"\n",(0,a.jsx)(r.h2,{id:"enable-openbao-workload-telemetry-deliberately",children:"Enable OpenBao workload telemetry deliberately"}),"\n",(0,a.jsx)(o,{language:"yaml",label:"configure",title:"Turn on workload telemetry for an OpenBaoCluster",code:`apiVersion: openbao.org/v1alpha1
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
      scrapeTimeout: "10s"`,children:(0,a.jsxs)(r.p,{children:["This enables the OpenBao telemetry stanza with safe defaults and creates an active-node metrics Service plus a Prometheus Operator ServiceMonitor when that is the scrape model you use. Reach for ",(0,a.jsx)(r.code,{children:"spec.telemetry"})," when you need lower-level OpenBao telemetry tuning."]})}),"\n",(0,a.jsx)(o,{language:"yaml",label:"configure",title:"Configure authenticated workload scraping",code:`apiVersion: openbao.org/v1alpha1
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
          key: ca.crt`,children:(0,a.jsxs)(r.p,{children:["Use this shape for production Prometheus Operator scraping. The Secret should contain a scoped OpenBao token that can read ",(0,a.jsx)(r.code,{children:"sys/metrics"}),", and the CA reference should validate the OpenBao serving certificate."]})}),"\n",(0,a.jsx)(o,{language:"yaml",label:"configure",title:"Scrape every OpenBao node through a metrics-only listener",code:`apiVersion: openbao.org/v1alpha1
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
        release: kube-prometheus-stack`,children:(0,a.jsx)(r.p,{children:"Use this profile when you need standby-node and per-node Raft visibility. The operator renders a dedicated metrics listener, a headless metrics Service, and ServiceMonitor relabeling for the pod and node names."})}),"\n",(0,a.jsxs)(r.p,{children:["The default ",(0,a.jsx)(r.code,{children:"Active"})," scrape profile targets the active OpenBao pod on the API\nlistener using ",(0,a.jsx)(r.code,{children:"/v1/sys/metrics?format=prometheus"}),". The ",(0,a.jsx)(r.code,{children:"AllNodes"})," profile\ntargets every OpenBao pod through the metrics-only listener. Keep the metrics\nService reachable only from your monitoring namespace with NetworkPolicy when\nunauthenticated metrics access is enabled."]}),"\n",(0,a.jsx)(i,{kind:"reference",title:"Promote a small set of signals first",columns:["Concern","What to watch","Why it matters"],rows:[{cells:["Availability","`openbao_cluster_ready_replicas` and cluster conditions such as `Available` or `Degraded`","This tells you whether the cluster is serving traffic rather than only whether Pods exist."],emphasis:"recommended"},{cells:["Steady read pool","`openbao_cluster_read_replicas_desired`, `_ready`, `_registered`, `_healthy`, plus read-replica conditions such as `ReadServingAvailable` and `ReadReplicasAutopilotHealthy`","This tells you whether the steady read tier exists, has actually joined, and is still healthy enough for the topology you placed it in."]},{cells:["Backup freshness","`openbao_backup_last_success_timestamp`, `openbao_backup_consecutive_failures`, and the backup status conditions","These signals show whether the snapshots you plan to restore are current and successful."]},{cells:["Upgrade safety","`openbao_upgrade_in_progress`, `openbao_upgrade_failure_total`, and rollback counters","These signals distinguish orchestrated upgrade activity from normal steady state."]},{cells:["Controller health","`openbao_reconcile_errors_total` and sustained reconcile duration spikes","This exposes control-plane failures before they become cluster-wide drift or stalled operations."]}]}),"\n",(0,a.jsx)(o,{language:"yaml",label:"apply",title:"Start with focused alert rules",code:`apiVersion: monitoring.coreos.com/v1
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
        for: 10m`,children:(0,a.jsx)(r.p,{children:"Keep the first alert set small. Availability, backup freshness, sustained read-pool degradation, and sustained reconcile failure are the highest-value starting signals."})}),"\n",(0,a.jsx)(r.h2,{id:"dashboards-logs-and-health",children:"Dashboards, logs, and health"}),"\n",(0,a.jsx)(o,{language:"bash",label:"apply",title:"Install the bundled Grafana dashboards",code:"kubectl apply -k config/grafana -n monitoring",children:(0,a.jsxs)(r.p,{children:["The per-feature dashboards under ",(0,a.jsx)(r.code,{children:"config/grafana/dashboards/"})," are the better starting point. The old monolithic dashboard still exists, but it is no longer the recommended default."]})}),"\n",(0,a.jsx)(o,{language:"yaml",label:"configure",title:"Raise log detail temporarily during investigation",code:`controller:
extraArgs:
  - --zap-log-level=debug
  - --zap-stacktrace-level=error`,children:(0,a.jsx)(r.p,{children:"Use debug logging only long enough to capture the behavior you need. Reset to the normal log level once the incident or rollout check is complete."})}),"\n",(0,a.jsx)(t,{type:"tip",title:"Keep operator metrics and workload telemetry separate in your dashboards",children:(0,a.jsx)(r.p,{children:"Build dashboards that show both surfaces together and still make it obvious whether a failure is in the operator control plane or in the OpenBao workload itself."})}),"\n",(0,a.jsx)(t,{type:"note",title:"The overview dashboard now includes the steady read pool",children:(0,a.jsxs)(r.p,{children:[(0,a.jsx)(r.code,{children:"config/grafana/dashboards/overview.json"})," now shows desired, ready, registered, and Autopilot-healthy read-replica counts next to the existing cluster-level signals. Use that view for the first operational pass, then build more topology-specific dashboards if your placement strategy needs them."]})}),"\n",(0,a.jsx)(n,{title:"Keep the operational loop tight",items:[{label:"Configure backups",description:"Configure backup telemetry before restore depends on it.",docId:"user-guide/openbaocluster/operations/backups"},{label:"Plan upgrades",description:"Use upgrade metrics and rollback signals to make rollout behavior observable before the first version change.",docId:"user-guide/openbaocluster/operations/upgrades"},{label:"Troubleshoot the cluster",description:"Move from baseline telemetry into symptom-driven incident routing when the service stops behaving normally.",docId:"user-guide/openbaocluster/operations/troubleshooting"}]})]})}function p(e={}){let{wrapper:r}={...(0,s.R)(),...e.components};return r?(0,a.jsx)(r,{...e,children:(0,a.jsx)(d,{...e})}):d(e)}function u(e,r){throw Error("Expected "+(r?"component":"object")+" `"+e+"` to be defined: you likely forgot to import, pass, or provide it.")}},28453(e,r,t){t.d(r,{R:()=>i,x:()=>n});var o=t(96540);let a={},s=o.createContext(a);function i(e){let r=o.useContext(s);return o.useMemo(function(){return"function"==typeof e?e(r):{...r,...e}},[r,e])}function n(e){let r;return r=e.disableParentContext?"function"==typeof e.components?e.components(a):e.components||a:i(e.components),o.createElement(s.Provider,{value:r},e.children)}}}]);