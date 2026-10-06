# Restore across independent Kubernetes clusters

Source: `test/e2e/restore_cross_cluster_test.go`

Note: recorded checkpoints are best-effort extracts from literal `By(...)` calls visible to `ginkgo outline`.

## Cases

| Case ID | Spec | State | Covers | Labels |
| --- | --- | --- | --- | --- |
| `restore-cross-cluster-disposable` | confirms and cleans a disposable restore while the source Kubernetes cluster is stopped | active | _none_ | `restore-cross-cluster`, `dr`, `restore`, `slow` |
| `restore-cross-cluster-retain` | retains restored data and resumes after administrator repair of destination JWT trust | active | _none_ | `restore-cross-cluster`, `dr`, `restore`, `slow` |

## `restore-cross-cluster-disposable`

Path: `Restore across independent Kubernetes clusters > confirms and cleans a disposable restore while the source Kubernetes cluster is stopped`

State: `active`

Generated fallback ID: `restore-cross-cluster-confirms-and-cleans-a-disposable-restore-67f9ae83`

Covers: _none_

Labels: `restore-cross-cluster`, `dr`, `restore`, `slow`


## `restore-cross-cluster-retain`

Path: `Restore across independent Kubernetes clusters > retains restored data and resumes after administrator repair of destination JWT trust`

State: `active`

Generated fallback ID: `restore-cross-cluster-retains-restored-data-and-resumes-after-0daa6d35`

Covers: _none_

Labels: `restore-cross-cluster`, `dr`, `restore`, `slow`

Recorded checkpoints:
- showing Resume retains the hold while restored source trust rejects the destination controller
- repairing issuer, signing-key trust, controller audience, and lifecycle subjects through independent administrator access
- confirming normal management keeps the resumed Pod and service registration works
- reading the restored data after managed restart with the source still unavailable
