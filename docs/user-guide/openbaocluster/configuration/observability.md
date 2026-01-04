# Observability

The OpenBao Operator exposes comprehensive metrics, structured logs, and health endpoints to integrate with your existing monitoring stack.

## Metrics

The Operator exposes Prometheus metrics on port `8080` by default (configurable via Helm).

### Enabling Metrics Scraping

=== "Prometheus Operator (ServiceMonitor)"

    The Helm chart can create a `ServiceMonitor` automatically:

    ```yaml
    # values.yaml
    metrics:
      enabled: true
      port: 8080
      serviceMonitor:
        enabled: true
        interval: 30s
        scrapeTimeout: 10s
    ```

=== "Prometheus Annotations"

    If not using the Prometheus Operator, annotate the service:

    ```yaml
    # values.yaml
    metrics:
      enabled: true
      annotations:
        prometheus.io/scrape: "true"
        prometheus.io/port: "8080"
        prometheus.io/path: "/metrics"
    ```

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
| `openbao_backup_total` | Counter | `namespace`, `name`, `type` | Backups attempted |
| `openbao_backup_success_total` | Counter | `namespace`, `name`, `type` | Successful backups |
| `openbao_backup_failure_total` | Counter | `namespace`, `name`, `type` | Failed backups |
| `openbao_backup_duration_seconds` | Histogram | `namespace`, `name`, `type` | Backup duration |
| `openbao_backup_last_success_timestamp` | Gauge | `namespace`, `name` | Unix timestamp of last success |
| `openbao_backup_size_bytes` | Gauge | `namespace`, `name` | Size of last backup |

The `type` label indicates the backup trigger:

- `scheduled` - Cron-scheduled backup
- `manual` - User-triggered backup
- `pre-upgrade` - Automatic backup before upgrade

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
| `openbao_upgrade_total` | Counter | `namespace`, `name`, `strategy` | Upgrades initiated |
| `openbao_upgrade_success_total` | Counter | `namespace`, `name`, `strategy` | Successful upgrades |
| `openbao_upgrade_failure_total` | Counter | `namespace`, `name`, `strategy` | Failed upgrades |
| `openbao_upgrade_rollback_total` | Counter | `namespace`, `name`, `strategy` | Rollbacks triggered |
| `openbao_upgrade_duration_seconds` | Histogram | `namespace`, `name`, `strategy` | Upgrade duration |

The `strategy` label is either `RollingUpdate` or `BlueGreen`.

!!! tip "Upgrade Rollback Monitoring"
    Track rollback frequency to identify problematic upgrades:

    ```promql
    increase(openbao_upgrade_rollback_total[7d]) > 0
    ```

#### Drift Detection Metrics

| Metric | Type | Labels | Description |
| :----- | :--- | :----- | :---------- |
| `openbao_drift_detected_total` | Counter | `namespace`, `name`, `resource_kind` | Drift events detected |
| `openbao_drift_corrected_total` | Counter | `namespace`, `name` | Drift events corrected |
| `openbao_drift_last_detected_timestamp` | Gauge | `namespace`, `name` | Last drift detection time |

!!! note "Drift Indicates External Modifications"
    High drift counts may indicate unauthorized changes or conflicting controllers.
    Investigate the `resource_kind` label to identify the source.

## Grafana Dashboard

A pre-built Grafana dashboard is included with the Operator.

### Installation

=== "Kubernetes ConfigMap"

    Apply the dashboard as a ConfigMap for Grafana sidecar discovery:

    ```bash
    kubectl apply -f config/grafana/
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
| **Drift** | Detection & Correction Rate |
| **TLS** | Certificate Expiry, Rotation Count |

## Logging

The Operator emits structured JSON logs with consistent fields for log aggregation.

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
    {namespace="openbao-operator"} 
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

The Operator exposes health endpoints for Kubernetes probes.

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
            summary: "OpenBao operator experiencing high reconciliation error rate"

        # Drift detection
        - alert: OpenBaoExcessiveDrift
          expr: rate(openbao_drift_detected_total[1h]) > 10
          for: 30m
          labels:
            severity: warning
          annotations:
            summary: "OpenBao cluster {{ $labels.name }} experiencing excessive configuration drift"
```

## OpenBao Server Metrics

In addition to Operator metrics, OpenBao itself exposes telemetry.

### Enabling OpenBao Telemetry

Configure telemetry in the cluster spec:

```yaml
apiVersion: openbao.org/v1alpha1
kind: OpenBaoCluster
metadata:
  name: prod-cluster
spec:
  telemetry:
    prometheusRetentionTime: "30s"
    disableHostname: true
```

This exposes OpenBao metrics at `/v1/sys/metrics` on the OpenBao pods.

!!! note "Separate Scrape Config"
    OpenBao server metrics require a separate scrape configuration targeting
    the OpenBao pods directly, not the Operator.

