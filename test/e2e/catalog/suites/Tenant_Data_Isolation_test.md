# Tenant Data Isolation

Source: `test/e2e/Tenant_Data_Isolation_test.go`

Note: recorded checkpoints are best-effort extracts from literal `By(...)` calls visible to `ginkgo outline`.

## Cases

| Case ID | Spec | State | Covers | Labels |
| --- | --- | --- | --- | --- |
| `tenant-data-plane-isolation` | isolates tenant data plane access across namespaces | active | `tenant-isolation`, `data-plane-isolation`, `network-isolation` | `security`, `tenant`, `tenancy` |

## `tenant-data-plane-isolation`

Path: `Tenant Data Isolation > isolates tenant data plane access across namespaces`

State: `active`

Generated fallback ID: `tenant-data-isolation-isolates-tenant-data-plane-access-across-eb142963`

Covers: `tenant-isolation`, `data-plane-isolation`, `network-isolation`

Labels: `security`, `tenant`, `tenancy`

Recorded checkpoints:
- writing tenant-specific secrets through each tenant's own data plane
- verifying each tenant can still reach and read its own cluster
- verifying a labeled pod in tenant A still cannot reach tenant B's data plane
- verifying the same labeled access path works inside tenant B
