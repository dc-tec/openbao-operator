# Component Design

The Operator runs a controller manager process hosting multiple controller-runtime controllers. The primary API (`OpenBaoCluster`) is reconciled by three dedicated controllers to keep responsibilities clear and avoid status write contention.

## Controller Overview

| Controller | Responsibility |
|------------|----------------|
| `openbaocluster-workload` | Workload reconciliation (certs, infra, init) + Sentinel fast-path drift correction |
| `openbaocluster-adminops` | Admin operations (upgrades, backups) + periodic/forced full reconciles after Sentinel fast-path |
| `openbaocluster-status` | Single-writer for `status.conditions` + finalizers + status/phase aggregation |
| [`openbaorestore`](restore-manager.md) | Reconcile `OpenBaoRestore` jobs and restore lifecycle |
| `openbao-provisioner` | Reconcile `OpenBaoTenant` and provision tenant-scoped RBAC |

## Manager Overview (OpenBaoCluster)

| Manager | Responsibility |
|---------|----------------|
| [Cert Manager](cert-manager.md) | Bootstrap PKI and rotate certificates |
| [Infrastructure Manager](infra-manager.md) | Render configuration and maintain StatefulSet |
| [Init Manager](init-manager.md) | Automate initial cluster initialization |
| [Upgrade Manager](upgrade-manager.md) | Safe rolling and blue/green updates |
| [Backup Manager](backup-manager.md) | Schedule and execute Raft snapshots |

## Pausing Reconciliation

All `OpenBaoCluster` controllers MUST honor `spec.paused`:

- When `OpenBaoCluster.Spec.Paused == true`, reconcilers SHOULD short-circuit early and avoid mutating cluster resources, allowing safe manual maintenance (e.g., manual restore).
- Finalizers and deletion handling MAY still proceed to ensure cleanup when the CR is deleted.

## Manager Details

### Cert Manager (TLS Lifecycle)

Supports three TLS modes: `OperatorManaged`, `External`, and `ACME`. Handles certificate generation, rotation, and hot-reload signaling.

[Read more →](cert-manager.md)

### Infrastructure Manager (Config & StatefulSet)

Generates `config.hcl` dynamically, manages the StatefulSet, handles auto-unseal configuration, and performs image verification.

[Read more →](infra-manager.md)

### Init Manager (Cluster Initialization)

Handles single-pod bootstrap, cluster initialization via HTTP API, and root token storage.

[Read more →](init-manager.md)

### Upgrade Manager (Rolling & Blue/Green)

Implements StatefulSet partitioning for rolling updates and a 10-phase state machine for blue/green upgrades with automatic rollback.

[Read more →](upgrade-manager.md)

### Backup Manager (Snapshots)

Creates backup Jobs that stream Raft snapshots directly to object storage with cron scheduling and retention management.

[Read more →](backup-manager.md)
