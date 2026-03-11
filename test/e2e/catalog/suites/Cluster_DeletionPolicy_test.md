# Cluster Lifecycle: Deletion Policy

Source: `test/e2e/Cluster_DeletionPolicy_test.go`

Note: recorded checkpoints are best-effort extracts from literal `By(...)` calls visible to `ginkgo outline`.

## Cases

| Case ID | Spec | State | Covers | Labels |
| --- | --- | --- | --- | --- |
| `deletion-policy-delete-all` | deletes PVCs and Secrets when policy is DeleteAll | active | `deletion-policy`, `pvc-cleanup`, `recoverability-secret-cleanup`, `tls-secret-cleanup` | `lifecycle`, `cluster`, `deletion` |
| `deletion-policy-delete-pvcs` | deletes PVCs and Secrets when policy is DeletePVCs | active | `deletion-policy`, `pvc-cleanup`, `recoverability-secret-cleanup` | `lifecycle`, `cluster`, `deletion` |
| `deletion-policy-retain` | retains PVCs and recoverability secrets when policy is Retain | active | `deletion-policy`, `pvc-retention`, `recoverability-secret-retention` | `lifecycle`, `cluster`, `deletion` |

## `deletion-policy-delete-all`

Path: `Cluster Lifecycle: Deletion Policy > deletes PVCs and Secrets when policy is DeleteAll`

State: `active`

Generated fallback ID: `cluster-deletionpolicy-deletes-pvcs-and-secrets-when-policy-4358a35f`

Covers: `deletion-policy`, `pvc-cleanup`, `recoverability-secret-cleanup`, `tls-secret-cleanup`

Labels: `lifecycle`, `cluster`, `deletion`


## `deletion-policy-delete-pvcs`

Path: `Cluster Lifecycle: Deletion Policy > deletes PVCs and Secrets when policy is DeletePVCs`

State: `active`

Generated fallback ID: `cluster-deletionpolicy-deletes-pvcs-and-secrets-when-policy-8c40239c`

Covers: `deletion-policy`, `pvc-cleanup`, `recoverability-secret-cleanup`

Labels: `lifecycle`, `cluster`, `deletion`


## `deletion-policy-retain`

Path: `Cluster Lifecycle: Deletion Policy > retains PVCs and recoverability secrets when policy is Retain`

State: `active`

Generated fallback ID: `cluster-deletionpolicy-retains-pvcs-and-recoverability-secrets-when-cbc6acaa`

Covers: `deletion-policy`, `pvc-retention`, `recoverability-secret-retention`

Labels: `lifecycle`, `cluster`, `deletion`


