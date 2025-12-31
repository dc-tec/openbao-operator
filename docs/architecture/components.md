# Component Design

The Operator runs a single `OpenBaoCluster` controller which delegates to five specific internal "Managers" to separate concerns.

## Manager Overview

| Manager | Responsibility |
|---------|----------------|
| [Cert Manager](cert-manager.md) | Bootstrap PKI and rotate certificates |
| [Infrastructure Manager](infra-manager.md) | Render configuration and maintain StatefulSet |
| [Init Manager](init-manager.md) | Automate initial cluster initialization |
| [Upgrade Manager](upgrade-manager.md) | Safe rolling and blue/green updates |
| [Backup Manager](backup-manager.md) | Schedule and execute Raft snapshots |

## Pausing Reconciliation

All controllers MUST honor `spec.paused`:

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
