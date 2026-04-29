# ACME TLS (OpenBao native ACME client)

Source: `test/e2e/Cluster_TLS_ACME_test.go`

Note: recorded checkpoints are best-effort extracts from literal `By(...)` calls visible to `ginkgo outline`.

## Cases

| Case ID | Spec | State | Covers | Labels |
| --- | --- | --- | --- | --- |
| `cluster-tls-acme-creates-an-acme-tls-cluster-and-4caefad8` | creates an ACME TLS cluster and becomes Ready (no TLS secrets mounted) | active | _none_ | `tls`, `security`, `slow` |
| `cluster-tls-acme-provisions-tenant-rbac-via-openbaotenant-cbd56667` | provisions tenant RBAC via OpenBaoTenant | active | _none_ | `tls`, `security`, `slow` |

## `cluster-tls-acme-creates-an-acme-tls-cluster-and-4caefad8`

Path: `ACME TLS (OpenBao native ACME client) > creates an ACME TLS cluster and becomes Ready (no TLS secrets mounted)`

State: `active`

Covers: _none_

Labels: `tls`, `security`, `slow`

Recorded checkpoints:
- setting up ACME service and domain
- creating transit token secret for auto-unseal (include TLS CA for transit and PKI CA for ACME)
- verifying transit token secret can access infra-bao transit key
- waiting for OpenBaoCluster to be observed by the API server
- verifying TLS secrets are NOT created (ACME mode)
- checking for prerequisite resources (ConfigMap)
- waiting for StatefulSet to be created
- waiting for the first ACME pod to become Ready after self-init
- waiting for StatefulSet pods to reach the desired replica count
- waiting for the ACME shared cache PVC to become Bound
- verifying documented ACME readiness conditions
- validating that the config contains ACME parameters
- verifying the ACME-issued certificate is trusted by the PKI CA


## `cluster-tls-acme-provisions-tenant-rbac-via-openbaotenant-cbd56667`

Path: `ACME TLS (OpenBao native ACME client) > provisions tenant RBAC via OpenBaoTenant`

State: `active`

Covers: _none_

Labels: `tls`, `security`, `slow`

Recorded checkpoints:
- verifying OpenBaoTenant is provisioned


