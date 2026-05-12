# Cluster Runtime Controls

Source: `test/e2e/Cluster_Runtime_Controls_test.go`

Note: recorded checkpoints are best-effort extracts from literal `By(...)` calls visible to `ginkgo outline`.

## Cases

| Case ID | Spec | State | Covers | Labels |
| --- | --- | --- | --- | --- |
| `cluster-ingress-host-san` | creates ingress resources and includes the ingress host in the server certificate | active | `ingress`, `external-service`, `tls-san` | `lifecycle`, `cluster`, `runtime` |
| `cluster-oci-plugin-install` | registers an OCI plugin with a writable plugin directory | active | `plugin-auto-download`, `plugin-directory` | `lifecycle`, `cluster`, `runtime` |
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


## `cluster-oci-plugin-install`

Path: `Cluster Runtime Controls > registers an OCI plugin with a writable plugin directory`

State: `active`

Generated fallback ID: `cluster-runtime-controls-registers-an-oci-plugin-with-a-b5786be4`

Covers: `plugin-auto-download`, `plugin-directory`

Labels: `lifecycle`, `cluster`, `runtime`

Recorded checkpoints:
- waiting for the OpenBao pod to become ready
- verifying the StatefulSet mounts a writable plugin directory
- verifying OpenBao registered the declarative OCI plugin


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
