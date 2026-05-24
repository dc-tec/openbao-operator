# Prometheus Operator manifests (v0.91.0)

This directory vendors the Prometheus Operator ServiceMonitor CRD for use in local and CI testing.

- Version: `v0.91.0`
- Source: https://github.com/prometheus-operator/prometheus-operator/blob/v0.91.0/example/prometheus-operator-crd/monitoring.coreos.com_servicemonitors.yaml
- Used by: `envtest` suites that validate operator-managed ServiceMonitor admission behavior

## Contents

- `crds/monitoring.coreos.com_servicemonitors.yaml`: ServiceMonitor CRD copied from the upstream Prometheus Operator release.

## Updating

If you bump the Prometheus Operator CRD version:

1. Replace `crds/monitoring.coreos.com_servicemonitors.yaml` with the matching upstream release file.
2. Update envtest suites that reference this path.
3. Run `go test -tags=integration ./test/integration` to validate envtest still boots and ServiceMonitor resources can be created.
