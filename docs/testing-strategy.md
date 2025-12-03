# Testing Strategy: OpenBao Supervisor Operator

This document expands on the high-level "Testing Strategy" section in `technical-design-document.md` and defines how we structure tests for the OpenBao Supervisor Operator.

The goals of this strategy are:

- To provide repeatable, layered test coverage from pure Go logic to full end-to-end behavior.
- To make tests easy to understand and extend by using table-driven tests wherever practical.
- To ensure tests enforce the non-functional requirements (reconcile latency, concurrency, and resource limits) as the project evolves.

-----

## 1. Testing Layers

We use three primary layers of tests:

- **Unit Tests (Pure Go / Small-scope):**
  - Focus on functions in `internal/` and other packages that do not require a running Kubernetes API server.
  - Use table-driven tests by default to cover multiple inputs and edge cases.

- **Controller / Integration Tests (envtest):**
  - Use `controller-runtime`'s `envtest` to run reconciliation logic against a real API server with in-memory etcd.
  - Validate interactions between controllers, CRDs, and Kubernetes resources without a full cluster.

- **End-to-End Tests (kind + KUTTL or Ginkgo):**
  - Run the Operator and a real OpenBao image in a kind cluster.
  - Validate real-world scenarios: cluster creation, upgrades, backup scheduling, multi-tenancy behavior.

Each layer has its own focus and trade-offs; higher layers cover fewer scenarios but with higher fidelity.

-----

## 2. Unit Tests and Table-Driven Patterns

Unit tests focus on deterministic, in-process logic with no external I/O. Typical targets include:

- TLS/PKI helpers (e.g., SAN generation, rotation-window calculations).
- Config rendering and merge functions for `config.hcl`.
- Static auto-unseal key management helpers.
- Upgrade state-machine helper functions that compute next actions from a given status.
- Backup naming and path construction functions.

### 2.1 Table-Driven Test Pattern

We standardize on table-driven tests for unit-level logic, following the pattern:

```go
func TestRenderConfig(t *testing.T) {
    tests := []struct {
        name      string
        spec      OpenBaoClusterSpec
        wantErr   bool
        wantMatch string
    }{
        {
            name:      "minimal config",
            spec:      minimalSpec(),
            wantErr:   false,
            wantMatch: `listener "tcp"`,
        },
        {
            name:    "protected stanza override rejected",
            spec:    specWithForbiddenConfig(),
            wantErr: true,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            cfg, err := renderConfig(tt.spec)
            if (err != nil) != tt.wantErr {
                t.Fatalf("renderConfig() error = %v, wantErr %v", err, tt.wantErr)
            }
            if !tt.wantErr && tt.wantMatch != "" && !strings.Contains(cfg, tt.wantMatch) {
                t.Fatalf("renderConfig() output did not contain %q", tt.wantMatch)
            }
        })
    }
}
```

Guidelines:

- Prefer small, focused tables per behavior rather than giant tables covering unrelated concerns.
- Use descriptive `name` values to make it clear what each case is asserting.
- Construct inputs via small helpers (e.g., `minimalSpec()`) to keep test cases readable.

### 2.2 What to Avoid in Unit Tests

- Avoid network calls, real Kubernetes clients, and real OpenBao APIs in unit tests.
- Avoid global shared state between tests; prefer fresh instances per test case.

-----

## 3. Controller / Integration Tests (envtest)

Controller-level tests validate reconciliation behavior using `envtest`:

- A real API server and etcd are started.
- CRDs for `OpenBaoCluster` and related resources are installed.
- Controllers run with `client.Client` against this API server.

### 3.1 Scenarios to Cover

At minimum, envtest suites should cover:

- **Cluster Creation:**
  - Creating a minimal `OpenBaoCluster` results in:
    - Expected Secrets (`<cluster>-tls-ca`, `<cluster>-tls-server`, `<cluster>-unseal-key`).
    - Expected ConfigMaps and StatefulSets.
    - Correct `Status.Phase`, `ReadyReplicas`, and `Conditions` (e.g., `ConditionAvailable=True`).

- **Paused Semantics:**
  - When `spec.paused=true`, the reconciler:
    - Stops mutating StatefulSets, Secrets, and ConfigMaps.
    - Still honors finalizers and delete behavior.

- **DeletionPolicy Behavior:**
  - For each `DeletionPolicy` variant (`Retain`, `DeletePVCs`, `DeleteAll`), deletion of the `OpenBaoCluster`:
    - Cleans up or retains PVCs and backups as specified.
    - Clears finalizers and removes the CR.

- **Config Merge and Protected Stanzas:**
  - Valid user config fragments are merged.
  - Attempts to override protected stanzas are rejected or surfaced via Conditions.

Where individual envtest assertions involve small decision functions, we still prefer table-driven subtests to cover multiple variations per scenario.

### 3.2 Implementation Notes

- Use `testing.T` and Go subtests (`t.Run`) to group related scenarios.
- Use `Eventually`/`Consistently` patterns (from Gomega or similar) only where needed; prefer direct `client.Client` reads and explicit `timeouts`.
- Keep envtest suites focused; do not attempt to fully replicate E2E behavior.

-----

## 4. End-to-End Tests (kind + KUTTL or Ginkgo)

E2E tests validate real-system behavior using kind:

- kind cluster with Kubernetes versions from the support matrix.
- Deployed Operator image built from the current codebase.
- Real OpenBao image.
- Optional MinIO deployment for backup tests.

### 4.1 Core E2E Scenarios

- **Cluster Creation:**
  - Apply `OpenBaoCluster` manifests and verify:
    - StatefulSet pods become `Ready`.
    - OpenBao Raft peers form a cluster.
    - Status conditions report `Available=True`.

- **Raft-aware Upgrades:**
  - Bump `spec.version`/`image` and verify:
    - Pods roll one-by-one.
    - Leader is stepped down before upgrade.
    - No prolonged downtime and cluster remains healthy.

- **Backups:**
  - Configure `backup` settings pointing to MinIO.
  - Verify scheduled backups appear in object storage.
  - Confirm `Status.LastBackupTime` and metrics reflect successful runs.

- **Multi-Tenancy:**
  - Deploy multiple `OpenBaoCluster` resources across namespaces.
  - Verify:
    - No cross-tenant resource sharing (Secrets, ConfigMaps, PVCs).
    - Reconciliation of one cluster does not block others.

- **Performance / NFR Validation:**
  - Run at least 10 `OpenBaoCluster` instances.
  - Measure:
    - Reconcile latency (via `openbao_reconcile_duration_seconds`).
    - Operator CPU/memory.
  - Assert that reconciliation completes in approximately 30 seconds under normal load and that rate limiting prevents hot loops.

### 4.2 Tooling Choices

- KUTTL or Ginkgo may be used depending on the team’s preference:
  - KUTTL for declarative, YAML-based scenario descriptions.
  - Ginkgo for more programmatic flows requiring conditional logic.

-----

## 5. Backup Integration Testing with MinIO

As described in the TDD, we use MinIO to validate backup integration:

- Deploy MinIO as an S3-compatible endpoint within the test environment.
- Configure `BackupTarget` in `OpenBaoCluster` to point at MinIO.
- Validate:
  - Snapshots are streamed without buffering to disk.
  - Object names follow the expected naming convention (cluster name + timestamp + suffix).
  - Failures are reflected in `Status` and `openbao_backup_failure_total`.

These tests run in CI alongside other E2E scenarios.

-----

## 6. CI Integration and Test Execution

CI should:

- Run unit tests (`go test ./...`) on every change, enforcing table-driven tests for new internal logic.
- Run envtest-based controller tests on every PR to validate reconciliation behavior and CRD semantics.
- Run E2E suites (including MinIO backup tests and multi-tenant scenarios):
  - On main branch and/or nightly.
  - Across the supported Kubernetes versions listed in the compatibility matrix.

New functionality MUST include:

- Unit tests for new internal logic.
- envtest coverage where reconciliation behavior changes.
- E2E coverage for new workflows that affect cluster lifecycle, upgrades, or backups.

This layered, table-driven approach ensures that regressions are caught early and that the Operator maintains its intended behavior as the codebase evolves.

