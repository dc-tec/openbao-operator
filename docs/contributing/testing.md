# Testing Strategy

To ensure correctness and avoid regressions, we adopt a layered testing strategy:

- **Unit Tests (Pure Go / Small-scope):** Validate functions in `internal/` and other packages that do not require a running Kubernetes API server.
- **Controller / Integration Tests (envtest):** Use `controller-runtime`'s `envtest` to run reconciliation logic against a real API server with in-memory etcd.
- **End-to-End Tests (kind + KUTTL or Ginkgo):** Run the Operator and a real OpenBao image in a kind cluster.

Each layer has its own focus and trade-offs; higher layers cover fewer scenarios but with higher fidelity.

## 1. Unit Tests

Unit tests focus on deterministic, in-process logic with no external I/O. Typical targets include:

- TLS/PKI helpers (e.g., SAN generation, rotation-window calculations).
- Config rendering and merge functions for `config.hcl`.
- Static auto-unseal key management helpers.
- Initialization helpers (e.g., parsing health responses, checking container status).
- Upgrade state-machine helper functions that compute next actions from a given status.
- Backup naming and path construction functions.

### 1.1 Table-Driven Test Pattern

We standardize on table-driven tests for unit-level logic:

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

**What to Avoid in Unit Tests:**

- Avoid network calls, real Kubernetes clients, and real OpenBao APIs in unit tests.
- Avoid global shared state between tests; prefer fresh instances per test case.

**Run unit tests:**

```sh
go test ./...
```

Or for a specific package:

```sh
go test ./internal/certs/...
```

### 1.2 Golden File Testing for HCL Generation

The HCL configuration generation tests (`internal/config/builder_test.go`) use golden files to ensure exact output matching. Golden files are stored in `internal/config/testdata/` and contain the expected HCL output for each test scenario.

**When to update golden files:**

- After modifying `internal/config/builder.go` or related HCL generation logic
- When the expected HCL output changes due to feature additions or bug fixes
- When refactoring config generation that affects output formatting

**To update golden files:**

```sh
make test-update-golden
```

Or manually:

```sh
UPDATE_GOLDEN=true go test ./internal/config/... -v
```

This will regenerate all golden files based on the current implementation. Review the changes in the golden files carefully before committing, as they represent the actual HCL configuration that will be generated for OpenBao clusters.

## 2. Controller / Integration Tests (envtest)

Controller-level tests validate reconciliation behavior using `envtest`:

- A real API server and etcd are started.
- CRDs for `OpenBaoCluster` and related resources are installed.
- Controllers run with `client.Client` against this API server.

### 2.1 Scenarios to Cover

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
  - Attempts to override protected stanzas are rejected at admission via CRD-level CEL validation and ValidatingAdmissionPolicy, or surfaced via Status Conditions when detected during reconciliation.

- **Sentinel Drift Detection:**
  - Sentinel Deployment is created when `spec.sentinel.enabled` is true.
  - Sentinel ServiceAccount, Role, and RoleBinding are created with correct permissions.
  - Sentinel Deployment is deleted when `spec.sentinel.enabled` is false.
  - Sentinel trigger status (`status.sentinel.triggerID`) triggers fast-path reconciliation.
  - Fast-path mode skips UpgradeManager and BackupManager when trigger is present and unhandled.
  - Trigger is marked handled after successful reconciliation (`status.sentinel.lastHandledTriggerID`).

Where individual envtest assertions involve small decision functions, we still prefer table-driven subtests to cover multiple variations per scenario.

### 2.2 Implementation Notes

- Use `testing.T` and Go subtests (`t.Run`) to group related scenarios.
- Use `Eventually`/`Consistently` patterns (from Gomega or similar) only where needed; prefer direct `client.Client` reads and explicit `timeouts`.
- Keep envtest suites focused; do not attempt to fully replicate E2E behavior.

**Run integration tests:**

```sh
make test-integration
```

Or for a specific test:

```sh
go test ./internal/controller/openbaocluster/... -v
```

## 3. End-to-End Tests (kind + KUTTL or Ginkgo)

E2E tests validate real-system behavior using kind:

- kind cluster with Kubernetes versions from the compatibility matrix (see `docs/reference/compatibility.md`).
- Deployed Operator image built from the current codebase.
- Real OpenBao image.
- Optional MinIO deployment for backup tests.

### 3.0 Running the E2E Suite

The e2e tests live under `test/e2e` and are implemented with Ginkgo.

**Environment variables**

Images:

- `E2E_OPERATOR_IMAGE` (default: `example.com/openbao-operator:v0.0.1`)  
  Image used for the operator (`projectImage` in `test/e2e/e2e_suite_test.go`).
- `E2E_CONFIG_INIT_IMAGE` (default: `openbao-config-init:dev`)  
  Image used by the OpenBao config-init container.
- `E2E_SENTINEL_IMAGE` (default: `ghcr.io/dc-tec/openbao-operator-sentinel:v0.0.0`)  
  Image used by the Sentinel deployment.
- `E2E_BACKUP_EXECUTOR_IMAGE` (default: `openbao/backup-executor:dev`)  
  Image used by backup jobs.
- `E2E_UPGRADE_EXECUTOR_IMAGE` (default: `openbao/upgrade-executor:dev`)  
  Image used by upgrade jobs.

OpenBao versions:

- `E2E_OPENBAO_VERSION` (fallback: `defaultOpenBaoVersion` from `test/e2e/e2e_versions.go`)  
  Base OpenBao version used for most tests.
- `E2E_OPENBAO_IMAGE` (optional)  
  Overrides the OpenBao image; if unset, it is derived from `E2E_OPENBAO_VERSION`.
- `E2E_UPGRADE_FROM_VERSION` / `E2E_UPGRADE_TO_VERSION`  
  Control the starting and target versions for upgrade tests (rolling/blue-green).  
  CI **must** set these explicitly; the defaults in `e2e_versions.go` are for local/development only.

Gateway API manifests:

- `E2E_GATEWAY_API_STANDARD_MANIFEST`  
  Path or URL to the standard Gateway API install manifest. Recommended: local YAML under `test/manifests/gateway-api/` for hermetic CI.
- `E2E_GATEWAY_API_EXPERIMENTAL_MANIFEST`  
  Path or URL to the experimental Gateway API manifest.

If these Gateway API env vars are unset, the e2e helpers fall back to the upstream GitHub release URLs.

Other useful flags:

- `CERT_MANAGER_INSTALL_SKIP=true`  
  Skip automatic cert-manager installation if it is already present.
- `E2E_SKIP_CLEANUP=true`  
  Preserve the cluster state after the suite for debugging (namespaces and CRDs are not removed).

**Running the suite**

From the repository root:

```sh
go test ./test/e2e/... -tags=e2e -v
```

or use the project’s dedicated Make target if defined (for example):

```sh
make test-e2e
```

### 3.1 Label-based E2E selection

E2E tests use Ginkgo v2 labels (`Label(...)`) to slice suites by domain and criticality (for example `smoke`, `critical`, `security`, `upgrade`, `backup`, `requires-rustfs`).

You can run only tests matching a label expression via the Makefile:

- Run smoke/critical tests only:

  ```sh
  make test-e2e E2E_LABEL_FILTER='smoke && critical'
  ```

- Run only upgrade-related tests:

  ```sh
  make test-e2e E2E_LABEL_FILTER='upgrade'
  ```

- Run slow backup/restore tests that require the RustFS S3-compatible endpoint:

  ```sh
  make test-e2e E2E_LABEL_FILTER='backup && requires-rustfs'
  ```

`E2E_LABEL_FILTER` is passed through to Ginkgo’s `--label-filter` (or `-ginkgo.label-filter` when using `go test`). You can still use `E2E_FOCUS` for name-based filtering; when both are set, Ginkgo requires tests to satisfy **both** the focus pattern and the label filter.

### 3.1 Core E2E Scenarios

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

- **Sentinel Drift Detection:**
  - Enable Sentinel (`spec.sentinel.enabled: true`) and verify:
    - Sentinel Deployment is created with correct image, resources, and environment variables.
    - Sentinel ServiceAccount, Role, and RoleBinding are created.
    - Sentinel health endpoint (`/healthz`) returns 200 OK.
  - Drift detection:
    - Manually modify a managed ConfigMap (e.g., change `config.hcl` content).
    - Verify Sentinel detects the change and patches `OpenBaoCluster` status (`status.sentinel.triggerID`).
    - Verify operator enters fast-path mode (skips Upgrade and Backup managers).
    - Verify operator corrects the drift and marks the trigger handled (`status.sentinel.lastHandledTriggerID`).
  - Debouncing:
    - Rapidly modify multiple resources (StatefulSet, Service, ConfigMap) within the debounce window.
    - Verify only one trigger is emitted (not multiple).
  - Actor filtering:
    - Operator updates a StatefulSet (e.g., during normal reconciliation).
    - Verify Sentinel does NOT trigger (ignores operator updates).
  - Secret safety:
    - Update unseal key Secret metadata (e.g., add annotation) without changing data.
    - Verify Sentinel does NOT trigger (hash comparison prevents false positives).
  - VAP enforcement:
    - Attempt to use Sentinel ServiceAccount to modify `OpenBaoCluster.Spec` (should be blocked by VAP).
    - Attempt to modify operator-owned `OpenBaoCluster.Status` fields (conditions/phase/etc.) (should be blocked by VAP).
    - Verify Sentinel can only modify `status.sentinel` trigger fields.
  - Cleanup:
    - Disable Sentinel (`spec.sentinel.enabled: false`).
    - Verify Sentinel Deployment, ServiceAccount, Role, and RoleBinding are deleted.
    - `openbao_backup_last_success_timestamp` is updated.
  - Backup during upgrade:
    - Verify scheduled backups are skipped when `Status.Upgrade != nil`.

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

### 3.2 Tooling Choices

- KUTTL or Ginkgo may be used depending on the team's preference:
  - KUTTL for declarative, YAML-based scenario descriptions.
  - Ginkgo for more programmatic flows requiring conditional logic.

### 3.3 Backup Integration Testing with MinIO

As described in the architecture documentation, we use MinIO to validate backup integration:

- Deploy MinIO as an S3-compatible endpoint within the test environment.
- Configure `BackupTarget` in `OpenBaoCluster` to point at MinIO.
- Validate:
  - Snapshots are streamed without buffering to disk.
  - Object names follow the expected naming convention: `<prefix>/<namespace>/<cluster>/<timestamp>-<uuid>.snap`.
  - Failures are reflected in `Status.Backup.LastFailureReason` and `openbao_backup_failure_total`.

#### 3.3.1 Backup Test Scenarios

| Scenario | Expected Behavior |
| :--- | :--- |
| Scheduled backup | Backup appears in MinIO at scheduled time |
| Manual trigger (future) | Backup created immediately when annotation is set |
| Large backup (>100MB) | Multipart upload completes successfully |
| Retention by count | Old backups deleted when MaxCount exceeded |
| Retention by age | Old backups deleted when MaxAge exceeded |
| Invalid credentials | Backup fails with clear error in Status |
| MinIO unavailable | Backup fails, ConsecutiveFailures increments |
| Backup during upgrade | Backup skipped, no error |

#### 3.3.2 Credentials Testing

- **Static credentials:** Test with `accessKeyId` and `secretAccessKey` in Secret.
- **Session tokens:** Test with temporary credentials including `sessionToken`.
- **Missing credentials:** Verify appropriate error when credentials Secret is missing.
- **Invalid credentials:** Verify error is surfaced in `Status.Backup.LastFailureReason`.

These tests run in CI alongside other E2E scenarios.

### 3.4 Upgrade Testing Scenarios

| Scenario | Expected Behavior |
| :--- | :--- |
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

### 3.5 Upgrade Unit Test Coverage

Unit tests for the UpgradeManager (`internal/upgrade/*_test.go`):

- **Version validation:** Table-driven tests for semver parsing and comparison.
- **State machine transitions:** Test each phase transition with mocked Kubernetes client.
- **Timeout handling:** Verify context deadline enforcement.
- **Resume logic:** Test that upgrade resumes correctly from various `Status.Upgrade` states.

## 4. Code Quality

### 4.1 Linting

Run the linter:

```sh
make lint
```

Or using golangci-lint directly:

```sh
golangci-lint run
```

The project uses `golangci-lint` with the configuration in `.golangci.yml`.

### 4.2 Code Formatting

All code must be formatted with `gofmt` (or `goimports`):

```sh
go fmt ./...
```

Or:

```sh
goimports -w .
```

### 4.3 Code Generation

Generate code (deepcopy, clientset, etc.):

```sh
make generate
```

Generate manifests (CRDs, RBAC, etc.):

```sh
make manifests
```

**Important:** After modifying API types (`api/v1alpha1/*.go`), you must run `make generate` and `make manifests` to regenerate the code and manifests.

## 5. CI Integration and Test Execution

CI should:

- Run unit tests (`go test ./...`) on every change, enforcing table-driven tests for new internal logic.
- Run envtest-based controller tests on every PR to validate reconciliation behavior and CRD semantics.
- Run E2E suites (including MinIO backup tests and multi-tenant scenarios):
  - On main branch and/or nightly.
  - Across the supported Kubernetes versions listed in the compatibility matrix (`docs/reference/compatibility.md`).

New functionality MUST include:

- Unit tests for new internal logic.
- envtest coverage where reconciliation behavior changes.
- E2E coverage for new workflows that affect cluster lifecycle, upgrades, or backups.

This layered, table-driven approach ensures that regressions are caught early and that the Operator maintains its intended behavior as the codebase evolves.
