# OpenBao Operator Grafana Dashboard

This directory contains a sample Grafana dashboard for monitoring OpenBao Operator metrics.

## Dashboard Overview

The dashboard provides monitoring of OpenBao Operator operations across several key areas:

1. **Overview/Summary** - Key metrics at a glance (upgrade status, backup status, ready replicas)
2. **Reconciliation Performance** - Duration and error rates for reconciliation operations
3. **Upgrade Operations** - Upgrade duration, step-down operations, and per-pod upgrade metrics
4. **Backup Operations** - Backup duration, size, success/failure rates, and retention metrics
5. **TLS/Certificates** - Certificate expiry and rotation metrics
6. **Cluster Health** - Ready replicas and cluster phase

## Installation

### Option 1: Import via Grafana UI

1. Open Grafana and navigate to **Dashboards** → **Import**
2. Upload the `dashboard.json` file or paste its contents
3. Select your Prometheus data source
4. Click **Import**

### Option 2: Deploy via ConfigMap (Kubernetes)

```bash
kubectl create configmap openbao-operator-dashboard \
  --from-file=dashboard.json=config/grafana/dashboard.json \
  -n grafana

# If using Grafana Operator, label it appropriately
kubectl label configmap openbao-operator-dashboard \
  grafana_dashboard=1 \
  -n grafana
```

## Configuration

The dashboard uses template variables for filtering:

- **DS_PROMETHEUS**: Prometheus data source (auto-detected)
- **namespace**: Kubernetes namespace filter (populated from metrics)
- **cluster**: OpenBaoCluster name filter (populated from metrics)

These variables allow you to filter the dashboard to a specific cluster or view all clusters.

## Metrics Coverage

### Currently Implemented Metrics

The following metrics are implemented and will display data:

- **Upgrade Metrics:**
  - `openbao_upgrade_status`
  - `openbao_upgrade_duration_seconds`
  - `openbao_upgrade_pod_duration_seconds`
  - `openbao_upgrade_stepdown_total`
  - `openbao_upgrade_stepdown_failures_total`
  - `openbao_upgrade_in_progress`
  - `openbao_upgrade_pods_completed`
  - `openbao_upgrade_pods_total`

- **Backup Metrics:**
  - `openbao_backup_last_success_timestamp`
  - `openbao_backup_last_duration_seconds`
  - `openbao_backup_last_size_bytes`
  - `openbao_backup_success_total`
  - `openbao_backup_failure_total`
  - `openbao_backup_consecutive_failures`
  - `openbao_backup_in_progress`
  - `openbao_backup_retention_deleted_total`
  - `openbao_cluster_ready_replicas`
  - `openbao_cluster_phase`
  - `openbao_reconcile_duration_seconds`
  - `openbao_reconcile_errors_total`
  - `openbao_tls_cert_expiry_timestamp`
  - `openbao_tls_rotation_total`

## Customization

The dashboard can be customized to fit your needs:

1. **Time Range**: Default is 6 hours, adjust in the time picker
2. **Refresh Interval**: Default is 30 seconds, adjust in dashboard settings
3. **Panel Layout**: Panels can be rearranged, resized, or removed
4. **Thresholds**: Alert thresholds can be adjusted in panel settings
5. **Colors**: Color schemes can be customized per panel

## Troubleshooting

### No Data Showing

1. **Verify Prometheus is scraping the operator:**
   ```bash
   kubectl get servicemonitor -n openbao-operator-system
   ```

2. **Check metrics endpoint:**
   ```bash
   kubectl port-forward -n openbao-operator-system deployment/openbao-operator-controller-manager 8443:8443
   curl -k https://localhost:8443/metrics | grep openbao
   ```

3. **Verify data source connection:**
   - Ensure Prometheus data source is correctly configured in Grafana
   - Check that Prometheus is scraping the operator's metrics endpoint

4. **Check template variables:**
   - Ensure namespace and cluster variables are populated
   - Try selecting "All" if available, or manually enter values

### Missing Metrics

If certain metrics show "No data", they may not be implemented yet. Refer to the "Metrics Coverage" section above to see which metrics are currently available.

## References

- [Architecture Document](../docs/architecture.md) - Section 5 (Observability & Metrics)
- [Prometheus Integration](../config/prometheus/) - ServiceMonitor configuration
- [Grafana Dashboard Documentation](https://grafana.com/docs/grafana/latest/dashboards/)



