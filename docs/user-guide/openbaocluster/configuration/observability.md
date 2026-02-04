# Observability

The OpenBao Operator exposes comprehensive metrics, structured logs, and health endpoints to integrate with your existing monitoring stack.

## Metrics

The OpenBao Operator exposes Prometheus metrics on port `8443` by default (configurable via Helm values or Kustomize overlays). The endpoint is served over HTTPS and is protected via Kubernetes authentication and authorization (RBAC).

### Enabling Metrics Scraping

!!! note "RBAC-protected endpoint"
    The `/metrics` endpoint uses Kubernetes authn/authz. Your scraper must run with a ServiceAccount token and have permission to `get` the non-resource URL `/metrics`.

=== "Prometheus Operator (ServiceMonitor)"

    Create a `ServiceMonitor` to scrape the metrics Services.

    **Helm**

    The Helm chart can create `ServiceMonitor` resources automatically:

    ```yaml
    # values.yaml
    metrics:
      enabled: true
      port: 8443
      serviceMonitor:
        enabled: true
        interval: 30s
        scrapeTimeout: 10s
        tlsConfig:
          insecureSkipVerify: true
    ```

    Grant your Prometheus ServiceAccount permission to GET `/metrics`:

    ```yaml
    # values.yaml
    metrics:
      rbac:
        enabled: true
        subjects:
          - name: prometheus-k8s
            namespace: monitoring
    ```

    **YAML manifests**

    Apply a `ServiceMonitor` (example for the controller metrics Service):

    ```yaml
    apiVersion: monitoring.coreos.com/v1
    kind: ServiceMonitor
    metadata:
      name: openbao-operator-controller
      namespace: monitoring
    spec:
      namespaceSelector:
        matchNames:
          - openbao-operator-system
      selector:
        matchLabels:
          app.kubernetes.io/name: openbao-operator
          app.kubernetes.io/component: controller
      endpoints:
        - port: https
          path: /metrics
          scheme: https
          bearerTokenFile: /var/run/secrets/kubernetes.io/serviceaccount/token
          tlsConfig:
            insecureSkipVerify: true
    ```

    If you run the provisioner (multi-tenant mode), create a second `ServiceMonitor` with `app.kubernetes.io/component: provisioner`.

    Grant access to `/metrics` (replace the ServiceAccount as needed):

    ```bash
    kubectl create clusterrolebinding openbao-operator-metrics-reader-prometheus \
      --clusterrole=openbao-operator-metrics-reader \
      --serviceaccount=monitoring:prometheus-k8s
    ```

    If you deploy the OpenBao Operator via Kustomize, use the opt-in overlay `config/overlays/metrics-scrape-rbac` (update the subject first):

    ```bash
    kubectl apply -k config/overlays/metrics-scrape-rbac
    ```

    If you manage the OpenBao Operator via Kustomize, you can also enable the bundled examples by uncommenting `../prometheus` in `config/default/kustomization.yaml` and applying `kubectl apply -k config/default`.

=== "VictoriaMetrics Operator (VMServiceScrape)"

    Create a `VMServiceScrape` to scrape the metrics Services.

    **Helm**

    The Helm chart can create `VMServiceScrape` resources automatically:

    ```yaml
    # values.yaml
    metrics:
      enabled: true
      port: 8443
      victoriaMetrics:
        enabled: true
        interval: 30s
        scrapeTimeout: 10s
        tlsConfig:
          insecureSkipVerify: true
      rbac:
        enabled: true
        subjects:
          - name: vmagent
            namespace: monitoring
    ```

    **YAML manifests**

    Apply a `VMServiceScrape` (example for the controller metrics Service):

    ```yaml
    apiVersion: operator.victoriametrics.com/v1beta1
    kind: VMServiceScrape
    metadata:
      name: openbao-operator-controller
      namespace: monitoring
    spec:
      namespaceSelector:
        matchNames:
          - openbao-operator-system
      selector:
        matchLabels:
          app.kubernetes.io/name: openbao-operator
          app.kubernetes.io/component: controller
      endpoints:
        - port: https
          path: /metrics
          scheme: https
          bearerTokenFile: /var/run/secrets/kubernetes.io/serviceaccount/token
          tlsConfig:
            insecureSkipVerify: true
    ```

    If you run the provisioner (multi-tenant mode), create a second `VMServiceScrape` with `app.kubernetes.io/component: provisioner`.

    Grant access to `/metrics` (replace the ServiceAccount as needed):

    ```bash
    kubectl create clusterrolebinding openbao-operator-metrics-reader-vmagent \
      --clusterrole=openbao-operator-metrics-reader \
      --serviceaccount=monitoring:vmagent
    ```

    If you manage the OpenBao Operator via Kustomize, you can also enable the bundled examples by uncommenting `../victoriametrics` in `config/default/kustomization.yaml` and applying `kubectl apply -k config/default`.

=== "Plain Prometheus"

    If you are not using an operator, add scrape jobs targeting the metrics Services.
    Ensure the Prometheus process runs with a ServiceAccount token that has `get` permission on `/metrics`
    (see `metrics.rbac` above), and configure TLS appropriately.

    !!! note "Metrics Service names"
        - **Helm**: `<release>-openbao-operator-controller-metrics` and `<release>-openbao-operator-provisioner-metrics`
        - **YAML manifests**: `openbao-operator-controller-metrics-service` and `openbao-operator-provisioner-metrics-service`

    ```yaml
    scrape_configs:
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
              - <provisioner-metrics-service>.<operator-namespace>.svc:8443
    ```

    !!! note "NetworkPolicy"
        If you have `networkPolicy.enabled: true`, ensure your Prometheus namespace has the label(s) configured in `networkPolicy.metricsAllowedNamespaceLabels` so it can reach the metrics Service.

!!! tip "TLS Verification (Production)"
    For production, prefer strict TLS verification:

    - Set `metrics.serviceMonitor.tlsConfig.insecureSkipVerify: false` and configure CA/certs.
    - Set `metrics.victoriaMetrics.tlsConfig.insecureSkipVerify: false` and configure trusted CA in your scraping stack.

    See the production checklist for guidance.

### Available Metrics

#### Reconciliation Metrics

| Metric | Type | Labels | Description |
| :----- | :--- | :----- | :---------- |
| `openbao_reconcile_duration_seconds` | Histogram | `namespace`, `name`, `controller` | Duration of reconciliation loops |
| `openbao_reconcile_errors_total` | Counter | `namespace`, `name`, `controller`, `reason` | Total reconciliation errors |

!!! tip "Alerting on Reconciliation Errors"
    Alert when the error rate exceeds a threshold:

    ```promql
    rate(openbao_reconcile_errors_total[5m]) > 0.1
    ```

#### Cluster State Metrics

| Metric | Type | Labels | Description |
| :----- | :--- | :----- | :---------- |
| `openbao_cluster_ready_replicas` | Gauge | `namespace`, `name` | Number of ready replicas |
| `openbao_cluster_phase` | Gauge | `namespace`, `name`, `phase` | Current cluster phase (1 = active) |

The `phase` label takes one of these values:

- `Initializing` - Cluster is starting up
- `Running` - Cluster is healthy
- `Upgrading` - Upgrade in progress
- `BackingUp` - Backup in progress
- `Failed` - Cluster is in a failed state

!!! warning "Cluster Availability"
    Alert when ready replicas drop below expected:

    ```promql
    openbao_cluster_ready_replicas < 3
    ```

#### Backup Metrics

| Metric | Type | Labels | Description |
| :----- | :--- | :----- | :---------- |
| `openbao_backup_success_total` | Counter | `namespace`, `name` | Successful backups |
| `openbao_backup_failure_total` | Counter | `namespace`, `name` | Failed backups |
| `openbao_backup_consecutive_failures` | Gauge | `namespace`, `name` | Consecutive backup failures |
| `openbao_backup_in_progress` | Gauge | `namespace`, `name` | Backup in progress (1/0) |
| `openbao_backup_last_success_timestamp` | Gauge | `namespace`, `name` | Unix timestamp of last successful backup |
| `openbao_backup_last_duration_seconds` | Gauge | `namespace`, `name` | Duration of the last backup in seconds |
| `openbao_backup_last_size_bytes` | Gauge | `namespace`, `name` | Size of the last backup in bytes |
| `openbao_backup_retention_deleted_total` | Counter | `namespace`, `name` | Backups deleted by retention policy |

!!! danger "Backup Staleness Alert"
    Alert if backups are older than 24 hours:

    ```promql
    time() - openbao_backup_last_success_timestamp > 86400
    ```

#### Restore Metrics

| Metric | Type | Labels | Description |
| :----- | :--- | :----- | :---------- |
| `openbao_restore_total` | Counter | `namespace`, `name` | Restore operations attempted |
| `openbao_restore_success_total` | Counter | `namespace`, `name` | Successful restores |
| `openbao_restore_failure_total` | Counter | `namespace`, `name` | Failed restores |
| `openbao_restore_duration_seconds` | Histogram | `namespace`, `name` | Restore duration |

#### Upgrade Metrics

| Metric | Type | Labels | Description |
| :----- | :--- | :----- | :---------- |
| `openbao_upgrade_status` | Gauge | `namespace`, `name` | Upgrade status (0=none, 1=running, 2=success, 3=failed) |
| `openbao_upgrade_in_progress` | Gauge | `namespace`, `name` | Upgrade in progress (1/0) |
| `openbao_upgrade_total` | Counter | `namespace`, `name`, `strategy` | Upgrades initiated |
| `openbao_upgrade_success_total` | Counter | `namespace`, `name`, `strategy` | Successful upgrades |
| `openbao_upgrade_failure_total` | Counter | `namespace`, `name`, `strategy` | Failed upgrades |
| `openbao_upgrade_rollback_total` | Counter | `namespace`, `name`, `strategy` | Rollbacks triggered |
| `openbao_upgrade_duration_seconds` | Histogram | `namespace`, `name`, `from_version`, `to_version` | Total upgrade duration |
| `openbao_upgrade_pod_duration_seconds` | Histogram | `namespace`, `name`, `pod` | Per-pod upgrade duration |
| `openbao_upgrade_stepdown_total` | Counter | `namespace`, `name` | Leader step-down operations during upgrades |
| `openbao_upgrade_stepdown_failures_total` | Counter | `namespace`, `name` | Failed leader step-down operations |
| `openbao_upgrade_pods_total` | Gauge | `namespace`, `name` | Total pods to upgrade |
| `openbao_upgrade_pods_completed` | Gauge | `namespace`, `name` | Pods upgraded so far |
| `openbao_upgrade_partition` | Gauge | `namespace`, `name` | Current StatefulSet partition during rolling upgrades |

The `strategy` label is either `RollingUpdate` or `BlueGreen`.

!!! tip "Upgrade Rollback Monitoring"
    Track rollback frequency to identify problematic upgrades:

    ```promql
    increase(openbao_upgrade_rollback_total[7d]) > 0
    ```

## Grafana Dashboard

A pre-built Grafana dashboard is included with the OpenBao Operator.

### Installation

=== "Kubernetes ConfigMap"

    Apply the dashboard as a ConfigMap for Grafana sidecar discovery:

    ```bash
    kubectl apply -k config/grafana
    ```

=== "Manual Import"

    1. Open Grafana and navigate to **Dashboards > Import**.
    2. Upload `config/grafana/dashboard.json`.
    3. Select your Prometheus data source.

### Dashboard Panels

The dashboard includes:

| Section | Panels |
| :------ | :----- |
| **Overview** | Upgrade Status, Backup Status, Ready Replicas |
| **Reconciliation** | Duration (p50/p95/p99), Error Rate by Controller |
| **Backups** | Success/Failure Rate, Duration, Size, Last Success |
| **Upgrades** | Duration, Step-Down Operations, Progress |
| **Restores** | Success/Failure Rate, Duration |
| **TLS** | Certificate Expiry, Rotation Count |

## Logging

The OpenBao Operator emits structured JSON logs with consistent fields for log aggregation.

### Log Format

```json
{
  "level": "info",
  "ts": "2024-01-15T10:30:00.000Z",
  "logger": "openbaocluster",
  "msg": "Reconciliation complete",
  "cluster_name": "prod-cluster",
  "cluster_namespace": "vault",
  "controller": "openbaocluster",
  "reconcileID": "abc123"
}
```

### Key Log Fields

| Field | Description |
| :---- | :---------- |
| `cluster_name` | Name of the OpenBaoCluster |
| `cluster_namespace` | Namespace of the cluster |
| `controller` | Controller processing the event |
| `reconcileID` | Unique ID for correlating log entries |

### Log Levels

Configure the log level via Helm:

```yaml
# values.yaml
controller:
  args:
    - --zap-log-level=info  # debug, info, error
```

!!! tip "Debug Logging"
    Enable debug logging temporarily for troubleshooting:

    ```yaml
    controller:
      args:
        - --zap-log-level=debug
        - --zap-stacktrace-level=error
    ```

### Example Log Queries

=== "Loki (LogQL)"

    ```logql
    {namespace="openbao-operator-system"}
    | json
    | cluster_name="prod-cluster"
    | level="error"
    ```

=== "Elasticsearch"

    ```json
    {
      "query": {
        "bool": {
          "must": [
            { "match": { "cluster_name": "prod-cluster" } },
            { "match": { "level": "error" } }
          ]
        }
      }
    }
    ```

## Health Probes

The OpenBao Operator exposes health endpoints for Kubernetes probes.

### Endpoints

| Endpoint | Purpose | Port |
| :------- | :------ | :--- |
| `/healthz` | Liveness probe | 8081 |
| `/readyz` | Readiness probe | 8081 |

### Configuring Probes

```yaml
# values.yaml
healthProbes:
  port: 8081
  livenessInitialDelaySeconds: 15
  livenessPeriodSeconds: 20
  readinessInitialDelaySeconds: 5
  readinessPeriodSeconds: 10
```

## Recommended Alerts

Here are production-ready alert rules for the OpenBao Operator:

```yaml
apiVersion: monitoring.coreos.com/v1
kind: PrometheusRule
metadata:
  name: openbao-operator-alerts
spec:
  groups:
    - name: openbao-operator
      rules:
        # Cluster availability
        - alert: OpenBaoClusterDegraded
          expr: openbao_cluster_ready_replicas < 3
          for: 5m
          labels:
            severity: warning
          annotations:
            summary: "OpenBao cluster {{ $labels.name }} has fewer than 3 ready replicas"

        - alert: OpenBaoClusterDown
          expr: openbao_cluster_ready_replicas == 0
          for: 1m
          labels:
            severity: critical
          annotations:
            summary: "OpenBao cluster {{ $labels.name }} has no ready replicas"

        # Backup health
        - alert: OpenBaoBackupStale
          expr: time() - openbao_backup_last_success_timestamp > 86400
          for: 1h
          labels:
            severity: warning
          annotations:
            summary: "OpenBao cluster {{ $labels.name }} has not had a successful backup in 24+ hours"

        - alert: OpenBaoBackupFailing
          expr: rate(openbao_backup_failure_total[1h]) > 0
          for: 30m
          labels:
            severity: warning
          annotations:
            summary: "OpenBao cluster {{ $labels.name }} backups are failing"

        # Reconciliation health
        - alert: OpenBaoReconcileErrors
          expr: rate(openbao_reconcile_errors_total[5m]) > 0.5
          for: 10m
          labels:
            severity: warning
          annotations:
            summary: "OpenBao Operator experiencing high reconciliation error rate"
```

## OpenBao Server Metrics

In addition to Operator metrics, OpenBao itself exposes telemetry.

### Enabling OpenBao Telemetry

Configure telemetry in the cluster spec using the simplified `observability` block:

```yaml
apiVersion: openbao.org/v1alpha1
kind: OpenBaoCluster
metadata:
  name: prod-cluster
spec:
  observability:
    metrics:
      enabled: true
```

This automatically injects a `telemetry` stanza into the OpenBao configuration with safe defaults (prometheus retention, disabled hostname).
You can still provide fine-grained overrides via `spec.telemetry` if needed.

This exposes OpenBao metrics at `/v1/sys/metrics` on the OpenBao pods.

!!! note "Separate Scrape Config"
    OpenBao server metrics require a separate scrape configuration targeting
    the OpenBao pods directly, not the OpenBao Operator.
