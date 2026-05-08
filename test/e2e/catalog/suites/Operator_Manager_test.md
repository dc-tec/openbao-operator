# Manager

Source: `test/e2e/Operator_Manager_test.go`

Note: recorded checkpoints are best-effort extracts from literal `By(...)` calls visible to `ginkgo outline`.

## Cases

| Case ID | Spec | State | Covers | Labels |
| --- | --- | --- | --- | --- |
| `operator-manager-metrics-endpoint` | should ensure the metrics endpoint is serving metrics | active | `manager-metrics-endpoint` | `manager`, `critical`, `smoke` |

## `operator-manager-metrics-endpoint`

Path: `Manager > Manager > should ensure the metrics endpoint is serving metrics`

State: `active`

Generated fallback ID: `operator-manager-should-ensure-the-metrics-endpoint-is-71a02c09`

Covers: `manager-metrics-endpoint`

Labels: `manager`, `critical`, `smoke`

Recorded checkpoints:
- locating the controller pod
- validating that the metrics service is available
- ensuring the controller pod is ready
- verifying that the controller manager is serving the metrics server
- verifying that the controller metrics endpoint returns data
