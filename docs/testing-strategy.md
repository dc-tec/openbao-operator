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
- Initialization helpers (e.g., parsing health responses, checking container status).
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

- **Initialization Flow:**
  - StatefulSet starts with 1 replica when `Status.Initialized` is false.
  - After initialization, `Status.Initialized` becomes true.
  - Once initialized, StatefulSet scales to `Spec.Replicas`.
  - Root token Secret (`<cluster>-root-token`) is created during initialization.

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
  - Attempts to override protected stanzas are rejected at admission via CRD-level CEL validation (and, in later phases, a validating webhook), or surfaced via Status Conditions when detected during reconciliation.

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

- **Cluster Creation and Initialization:**
  - Apply `OpenBaoCluster` manifests and verify:
    - StatefulSet starts with 1 replica for initialization.
    - InitManager initializes the cluster and creates `<cluster>-root-token` Secret.
    - `Status.Initialized` becomes `true`.
    - StatefulSet scales to desired replicas.
    - OpenBao Raft peers form a cluster.
    - Status conditions report `Available=True`.

- **Raft-aware Upgrades:**
  - Bump `spec.version`/`image` and verify:
    - Pods roll one-by-one in reverse ordinal order.
    - Leader is stepped down before upgrade (verify via OpenBao API logs).
    - `Status.Upgrade` tracks progress (TargetVersion, CurrentPartition, CompletedPods).
    - `Status.Phase` transitions: Running -> Upgrading -> Running.
    - No prolonged downtime and cluster remains healthy.
  - Upgrade resumption:
    - Kill Operator mid-upgrade and restart.
    - Verify upgrade resumes from `Status.Upgrade.CurrentPartition`.
  - Downgrade rejection:
    - Attempt to set `spec.version` lower than `status.currentVersion`.
    - Verify `Degraded=True` with `Reason=DowngradeBlocked`.
  - Pre-upgrade backup:
    - Enable `spec.backup.preUpgradeSnapshot`.
    - Verify backup is created before pods are updated.

- **Backups:**
  - Configure `backup` settings pointing to MinIO.
  - Verify scheduled backups appear in object storage.
  - Confirm `Status.Backup` fields are updated:
    - `LastBackupTime`, `LastBackupSize`, `LastBackupDuration`, `LastBackupName`.
    - `NextScheduledBackup` is calculated correctly.
  - Verify backup naming convention: `<prefix>/<namespace>/<cluster>/<timestamp>-<uuid>.snap`.
  - Retention policy enforcement:
    - Configure `spec.backup.retention.maxCount: 3`.
    - Trigger multiple backups and verify older backups are deleted.
  - Backup metrics:
    - `openbao_backup_success_total` increments on success.
    - `openbao_backup_failure_total` increments on failure.
    - `openbao_backup_last_success_timestamp` is updated.
  - Backup during upgrade:
    - Verify scheduled backups are skipped when `Status.Upgrade != nil`.

- **Self-Init Backup Authentication:**
  - Deploy self-init cluster without backup token.
  - Verify backups are skipped with `Reason=NoBackupToken`.
  - Add backup token via self-init requests.
  - Verify backups succeed with configured token.

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
  - Object names follow the expected naming convention: `<prefix>/<namespace>/<cluster>/<timestamp>-<uuid>.snap`.
  - Failures are reflected in `Status.Backup.LastFailureReason` and `openbao_backup_failure_total`.

### 5.1 Backup Test Scenarios

| Scenario | Expected Behavior |
|----------|-------------------|
| Scheduled backup | Backup appears in MinIO at scheduled time |
| Manual trigger (future) | Backup created immediately when annotation is set |
| Large backup (>100MB) | Multipart upload completes successfully |
| Retention by count | Old backups deleted when MaxCount exceeded |
| Retention by age | Old backups deleted when MaxAge exceeded |
| Invalid credentials | Backup fails with clear error in Status |
| MinIO unavailable | Backup fails, ConsecutiveFailures increments |
| Backup during upgrade | Backup skipped, no error |

### 5.2 Credentials Testing

- **Static credentials**: Test with `accessKeyId` and `secretAccessKey` in Secret.
- **Session tokens**: Test with temporary credentials including `sessionToken`.
- **Missing credentials**: Verify appropriate error when credentials Secret is missing.
- **Invalid credentials**: Verify error is surfaced in `Status.Backup.LastFailureReason`.

These tests run in CI alongside other E2E scenarios.

-----

## 5.3 Upgrade Testing Scenarios

| Scenario | Expected Behavior |
|----------|-------------------|
| Patch upgrade (2.4.0 -> 2.4.1) | Smooth rolling update, no warnings |
| Minor upgrade (2.4.0 -> 2.5.0) | Smooth rolling update |
| Major upgrade (2.x -> 3.x) | Upgrade proceeds with warning in Status |
| Downgrade attempt | Upgrade blocked, `Degraded=True` with reason |
| Upgrade with leader on pod-1 | Pod-2 updated first, then step-down, then pod-1, then pod-0 |
| Upgrade paused mid-way | `spec.paused=true` halts upgrade, resume on unpause |
| Operator restart during upgrade | Upgrade resumes from `Status.Upgrade.CurrentPartition` |
| Degraded cluster | Upgrade blocked until quorum is healthy |
| Pre-upgrade backup failure | Upgrade blocked with `Degraded=True` |
| Step-down timeout | Upgrade halts, `Degraded=True` with timeout reason |

### 5.4 Upgrade Unit Test Coverage

Unit tests for the UpgradeManager (`internal/upgrade/*_test.go`):

- **Version validation**: Table-driven tests for semver parsing and comparison.
- **State machine transitions**: Test each phase transition with mocked Kubernetes client.
- **Timeout handling**: Verify context deadline enforcement.
- **Resume logic**: Test that upgrade resumes correctly from various `Status.Upgrade` states.

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
