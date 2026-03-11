# Manager

Source: `test/e2e/Operator_Manager_test.go`

Note: recorded checkpoints are best-effort extracts from literal `By(...)` calls visible to `ginkgo outline`.

## Cases

| Case ID | Spec | State | Covers | Labels |
| --- | --- | --- | --- | --- |
| `operator-manager-should-ensure-the-metrics-endpoint-is-71a02c09` | should ensure the metrics endpoint is serving metrics | active | _none_ | `manager`, `critical`, `smoke` |
| `operator-manager-should-run-successfully-620495ad` | should run successfully | active | _none_ | `manager`, `critical`, `smoke` |

## `operator-manager-should-ensure-the-metrics-endpoint-is-71a02c09`

Path: `Manager > Manager > should ensure the metrics endpoint is serving metrics`

State: `active`

Covers: _none_

Labels: `manager`, `critical`, `smoke`

Recorded checkpoints:
- validating that the metrics service is available
- ensuring the controller pod is ready
- verifying that the controller manager is serving the metrics server
- verifying that the controller metrics endpoint returns data


## `operator-manager-should-run-successfully-620495ad`

Path: `Manager > Manager > should run successfully`

State: `active`

Covers: _none_

Labels: `manager`, `critical`, `smoke`

Recorded checkpoints:
- validating that the controller-manager pod is running as expected


