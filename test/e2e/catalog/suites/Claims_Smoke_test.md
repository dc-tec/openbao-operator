# Claims Smoke

Source: `test/e2e/Claims_Smoke_test.go`

Note: recorded checkpoints are best-effort extracts from literal `By(...)` calls visible to `ginkgo outline`.

## Cases

| Case ID | Spec | State | Covers | Labels |
| --- | --- | --- | --- | --- |
| `claims-smoke-offering-secret-bootstrap` | binds a stable service offering to a ready same-cluster claim with secret-backed bootstrap | active | `service-offering-binding`, `secret-bootstrap-projection`, `same-cluster-materialization`, `same-cluster-connection` | `claims`, `claims-smoke`, `critical` |
| `claims-smoke-cleanup` | deletes the claim and cleans up local materialization artifacts | active | `claim-deletion`, `same-cluster-cleanup` | `claims`, `claims-smoke`, `critical` |

## `claims-smoke-offering-secret-bootstrap`

Path: `Claims Smoke > binds a stable service offering to a ready same-cluster claim with secret-backed bootstrap`

State: `active`

Generated fallback ID: `claims-smoke-binds-a-stable-service-offering-to-30b8c3cf`

Covers: `service-offering-binding`, `secret-bootstrap-projection`, `same-cluster-materialization`, `same-cluster-connection`

Labels: `claims`, `claims-smoke`, `critical`

Recorded checkpoints:
- waiting for the claim to bind the selected offering to one immutable service profile
- waiting for the claim to reach Ready
- waiting for the same-cluster materialization to point at a concrete local OpenBaoCluster
- waiting for the local OpenBaoCluster to report Running
- waiting for the claim-owned connection Secret to be published


## `claims-smoke-cleanup`

Path: `Claims Smoke > deletes the claim and cleans up local materialization artifacts`

State: `active`

Generated fallback ID: `claims-smoke-deletes-the-claim-and-cleans-up-f678d349`

Covers: `claim-deletion`, `same-cluster-cleanup`

Labels: `claims`, `claims-smoke`, `critical`

Recorded checkpoints:
- deleting the claim
- waiting for the claim to be removed
- waiting for the local OpenBaoCluster to be removed
- waiting for the claim-owned connection Secret to be removed
