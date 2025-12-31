---
trigger: always_on
glob: "internal/**/*.go"
description: Architecture and separation of concerns for the OpenBao Operator
---

# Architecture Rules

See [Architecture Documentation](docs/architecture/index.md).

## Separation of Concerns

### Controller Layer (`internal/controller/`)

Reconcilers have THREE responsibilities only:

1. **Fetch** the CR from the API
2. **Observe** current state
3. **Delegate** to internal packages

NO business logic in controllers.

### Internal Packages (`internal/`)

All complex logic lives here:

- `internal/infra/` — Kubernetes resource management (StatefulSet, Service, etc.)
- `internal/config/` — HCL configuration generation
- `internal/backup/` — Backup streaming to object storage
- `internal/upgrade/` — Rolling and blue/green upgrades
- `internal/pki/` — Certificate generation
- `internal/provisioner/` — RBAC and namespace management

### API Types (`api/v1alpha1/`)

CRD structs are DATA CONTAINERS only:

- No business logic methods
- Only simple helpers like `.GetCondition()`
- No methods that connect to external services

## Manager Pattern

Each domain has a Manager (e.g., `InfraManager`, `UpgradeManager`):

- Owns reconciliation logic for that domain
- Called by the controller
- Returns structured results

## No God Objects

Do NOT pass entire Reconciler to helpers:

```go
// Bad
func helper(r *Reconciler, cluster *v1alpha1.OpenBaoCluster)

// Good
func helper(ctx context.Context, client client.Client, cluster *v1alpha1.OpenBaoCluster)
```
