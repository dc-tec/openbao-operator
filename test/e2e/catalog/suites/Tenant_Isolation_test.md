# Tenant Isolation

Source: `test/e2e/Tenant_Isolation_test.go`

Note: recorded checkpoints are best-effort extracts from literal `By(...)` calls visible to `ginkgo outline`.

## Cases

| Case ID | Spec | State | Covers | Labels |
| --- | --- | --- | --- | --- |
| `tenant-isolation-allows-self-service-provisioning-within-own-67dc337d` | allows self-service provisioning within own namespace | active | _none_ | `security`, `tenant`, `tenancy`, `critical` |
| `tenant-isolation-blocks-cross-namespace-provisioning-attempts-15ee6ba1` | blocks cross-namespace provisioning attempts | active | _none_ | `security`, `tenant`, `tenancy`, `critical` |
| `tenant-isolation-creates-an-openbaocluster-without-openbaotenant-ffc0817d` | creates an OpenBaoCluster without OpenBaoTenant | active | _none_ | `security`, `tenant`, `tenancy`, `single-tenant` |
| `tenant-isolation-reacts-to-statefulset-deletion-via-event-0a45a642` | reacts to StatefulSet deletion via event-driven reconciliation | active | _none_ | `security`, `tenant`, `tenancy`, `single-tenant` |
| `tenant-isolation-reconciles-the-cluster-to-available-via-ef2fac83` | reconciles the cluster to Available via event-driven reconciliation | active | _none_ | `security`, `tenant`, `tenancy`, `single-tenant` |
| `tenant-isolation-sets-up-single-tenant-mode-environment-4b87e620` | sets up single-tenant mode environment | active | _none_ | `security`, `tenant`, `tenancy`, `single-tenant` |

## `tenant-isolation-allows-self-service-provisioning-within-own-67dc337d`

Path: `Tenant Isolation > Multi-Tenant: Self-Service (Confused Deputy Prevention) > allows self-service provisioning within own namespace`

State: `active`

Covers: _none_

Labels: `security`, `tenant`, `tenancy`, `critical`


## `tenant-isolation-blocks-cross-namespace-provisioning-attempts-15ee6ba1`

Path: `Tenant Isolation > Multi-Tenant: Self-Service (Confused Deputy Prevention) > blocks cross-namespace provisioning attempts`

State: `active`

Covers: _none_

Labels: `security`, `tenant`, `tenancy`, `critical`


## `tenant-isolation-creates-an-openbaocluster-without-openbaotenant-ffc0817d`

Path: `Tenant Isolation > Single-Tenant Mode > creates an OpenBaoCluster without OpenBaoTenant`

State: `active`

Covers: _none_

Labels: `security`, `tenant`, `tenancy`, `single-tenant`

Recorded checkpoints:
- creating OpenBaoCluster directly (no tenant required)


## `tenant-isolation-reacts-to-statefulset-deletion-via-event-0a45a642`

Path: `Tenant Isolation > Single-Tenant Mode > reacts to StatefulSet deletion via event-driven reconciliation`

State: `active`

Covers: _none_

Labels: `security`, `tenant`, `tenancy`, `single-tenant`

Recorded checkpoints:
- getting the current StatefulSet UID
- deleting the StatefulSet to test event-driven reconciliation
- waiting for StatefulSet to be recreated (event-driven, should be fast)


## `tenant-isolation-reconciles-the-cluster-to-available-via-ef2fac83`

Path: `Tenant Isolation > Single-Tenant Mode > reconciles the cluster to Available via event-driven reconciliation`

State: `active`

Covers: _none_

Labels: `security`, `tenant`, `tenancy`, `single-tenant`

Recorded checkpoints:
- waiting for StatefulSet to be created
- waiting for StatefulSet pods to be Ready
- waiting for cluster to become Available


## `tenant-isolation-sets-up-single-tenant-mode-environment-4b87e620`

Path: `Tenant Isolation > Single-Tenant Mode > sets up single-tenant mode environment`

State: `active`

Covers: _none_

Labels: `security`, `tenant`, `tenancy`, `single-tenant`

Recorded checkpoints:
- creating the single-tenant namespace
- applying the single-tenant ClusterRole
- creating RoleBinding in target namespace
- patching controller deployment with WATCH_NAMESPACE
- waiting for controller to restart with new configuration


