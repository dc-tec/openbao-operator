# Cluster Runtime Controls

Source: `test/e2e/Cluster_Runtime_Controls_test.go`

Note: recorded checkpoints are best-effort extracts from literal `By(...)` calls visible to `ginkgo outline`.

## Cases

| Case ID | Spec | State | Covers | Labels |
| --- | --- | --- | --- | --- |
| `cluster-ingress-host-san` | creates ingress resources and includes the ingress host in the server certificate | active | `ingress`, `external-service`, `tls-san` | `lifecycle`, `cluster`, `runtime` |
| `cluster-paused-resume` | pauses reconciliation until spec.paused is cleared | active | `paused-reconcile`, `paused-status` | `lifecycle`, `cluster`, `runtime`, `lower-layer-covered` |
| `cluster-telemetry-rendering` | renders telemetry configuration when observability metrics are enabled | active | `telemetry`, `observability-metrics` | `lifecycle`, `cluster`, `runtime`, `lower-layer-covered` |
| `cluster-restart-at-rolls-pod` | rolls the OpenBao pod when runtime.restartAt changes | active | `restart-at`, `pod-rollout` | `lifecycle`, `cluster`, `runtime` |

## `cluster-ingress-host-san`

Path: `Cluster Runtime Controls > creates ingress resources and includes the ingress host in the server certificate`

State: `active`

Generated fallback ID: `cluster-runtime-controls-creates-ingress-resources-and-includes-the-11208c05`

Covers: `ingress`, `external-service`, `tls-san`

Labels: `lifecycle`, `cluster`, `runtime`

Recorded checkpoints:
- verifying the ingress and public service are created for external access
- verifying the operator-managed server certificate includes the ingress host


## `cluster-paused-resume`

Path: `Cluster Runtime Controls > pauses reconciliation until spec.paused is cleared`

State: `active`

Generated fallback ID: `cluster-runtime-controls-pauses-reconciliation-until-spec-paused-is-7e97d27d`

Covers: `paused-reconcile`, `paused-status`

Labels: `lifecycle`, `cluster`, `runtime`, `lower-layer-covered`

Recorded checkpoints:
- waiting for paused status conditions to be reported
- verifying workload resources are not created while reconciliation is paused
- clearing spec.paused so normal reconciliation resumes


## `cluster-telemetry-rendering`

Path: `Cluster Runtime Controls > renders telemetry configuration when observability metrics are enabled`

State: `active`

Generated fallback ID: `cluster-runtime-controls-renders-telemetry-configuration-when-observability-metrics-b6a0c999`

Covers: `telemetry`, `observability-metrics`

Labels: `lifecycle`, `cluster`, `runtime`, `lower-layer-covered`

Recorded checkpoints:
- verifying the rendered config includes the telemetry stanza


## `cluster-restart-at-rolls-pod`

Path: `Cluster Runtime Controls > rolls the OpenBao pod when runtime.restartAt changes`

State: `active`

Generated fallback ID: `cluster-runtime-controls-rolls-the-openbao-pod-when-runtime-c0619ab5`

Covers: `restart-at`, `pod-rollout`

Labels: `lifecycle`, `cluster`, `runtime`

Recorded checkpoints:
- setting spec.runtime.restartAt to trigger a rolling restart
- waiting for the StatefulSet pod template to carry the restart annotation
- waiting for the original pod to be replaced


