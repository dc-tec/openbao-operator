# Contributing: OpenBao Operator

This guide helps developers get started with contributing to the OpenBao Operator project. It covers development environment setup, build instructions, and testing strategy.

## 1. Development Environment

### 1.1 Prerequisites

- **Go:** version v1.25.5+ (check with `go version`)
- **Docker:** version 28.3.3+ (check with `docker version`)
- **kubectl:** version v1.33+ (check with `kubectl version --client`)
- **Access to a Kubernetes cluster:** v1.33+ (for testing; see `docs/compatibility.md`)
- **Kind:** For local E2E testing (optional but recommended)
- **Make:** For running build targets

You should also have:

- Permissions to install CRDs and cluster-scoped RBAC
- A default `StorageClass` for StatefulSet PVCs

### 1.2 Clone the Repository

```sh
git clone https://github.com/openbao/openbao-operator.git
cd openbao-operator
```

## 2. Build & Run

### 2.1 Local Development

**Build the operator binary:**

```sh
make build
```

This compiles the operator binary to `bin/manager`.

**Run the operator locally:**

```sh
make run
```

This runs the operator locally using `kubectl` to connect to your configured Kubernetes cluster. The operator will use your local kubeconfig (typically `~/.kube/config`).

**Note:** Running locally requires:
- A Kubernetes cluster accessible via your kubeconfig
- CRDs installed (see below)
- Proper RBAC permissions

### 2.2 Install CRDs

Before running the operator, install the Custom Resource Definitions:

```sh
make install
```

This installs the `OpenBaoCluster` CRD into your cluster.

### 2.3 Deploy the Operator

**Build and push a container image:**

```sh
make docker-build docker-push IMG=<your-registry>/openbao-operator:tag
```

**Deploy to the cluster:**

```sh
make deploy IMG=<your-registry>/openbao-operator:tag
```

**NOTE:** If you encounter RBAC errors, you may need to grant yourself cluster-admin privileges or be logged in as admin.

### 2.4 Uninstall

**Delete the instances (CRs) from the cluster:**

```sh
kubectl delete -k config/samples/
```

**Delete the APIs (CRDs) from the cluster:**

```sh
make uninstall
```

**UnDeploy the controller from the cluster:**

```sh
make undeploy
```

### 2.5 Build Targets

Run `make help` to see all available make targets:

```sh
make help
```

Common targets include:

- `make build` - Build the operator binary
- `make run` - Run the operator locally
- `make install` - Install CRDs
- `make uninstall` - Uninstall CRDs
- `make deploy` - Deploy the operator to the cluster
- `make docker-build` - Build the container image
- `make docker-push` - Push the container image
- `make manifests` - Generate manifests (CRDs, RBAC, etc.)
- `make generate` - Generate code (deepcopy, clientset, etc.)
- `make test` - Run unit tests
- `make test-integration` - Run integration tests
- `make lint` - Run linters

## 3. Testing Strategy

To ensure correctness and avoid regressions, we adopt a layered testing strategy:

- **Unit Tests (Pure Go / Small-scope):** Validate functions in `internal/` and other packages that do not require a running Kubernetes API server.
- **Controller / Integration Tests (envtest):** Use `controller-runtime`'s `envtest` to run reconciliation logic against a real API server with in-memory etcd.
- **End-to-End Tests (kind + KUTTL or Ginkgo):** Run the Operator and a real OpenBao image in a kind cluster.

Each layer has its own focus and trade-offs; higher layers cover fewer scenarios but with higher fidelity.

### 3.1 Unit Tests

Unit tests focus on deterministic, in-process logic with no external I/O. Typical targets include:

- TLS/PKI helpers (e.g., SAN generation, rotation-window calculations).
- Config rendering and merge functions for `config.hcl`.
- Static auto-unseal key management helpers.
- Initialization helpers (e.g., parsing health responses, checking container status).
- Upgrade state-machine helper functions that compute next actions from a given status.
- Backup naming and path construction functions.

#### Table-Driven Test Pattern

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

### 3.2 Controller / Integration Tests (envtest)

Controller-level tests validate reconciliation behavior using `envtest`:

- A real API server and etcd are started.
- CRDs for `OpenBaoCluster` and related resources are installed.
- Controllers run with `client.Client` against this API server.

#### Scenarios to Cover

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

Where individual envtest assertions involve small decision functions, we still prefer table-driven subtests to cover multiple variations per scenario.

#### Implementation Notes

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

### 3.3 End-to-End Tests (kind + KUTTL or Ginkgo)

E2E tests validate real-system behavior using kind:

- kind cluster with Kubernetes versions from the compatibility matrix (see `docs/compatibility.md`).
- Deployed Operator image built from the current codebase.
- Real OpenBao image.
- Optional MinIO deployment for backup tests.

#### Core E2E Scenarios

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

#### Tooling Choices

- KUTTL or Ginkgo may be used depending on the team's preference:
  - KUTTL for declarative, YAML-based scenario descriptions.
  - Ginkgo for more programmatic flows requiring conditional logic.

### 3.4 Backup Integration Testing with MinIO

As described in the architecture documentation, we use MinIO to validate backup integration:

- Deploy MinIO as an S3-compatible endpoint within the test environment.
- Configure `BackupTarget` in `OpenBaoCluster` to point at MinIO.
- Validate:
  - Snapshots are streamed without buffering to disk.
  - Object names follow the expected naming convention: `<prefix>/<namespace>/<cluster>/<timestamp>-<uuid>.snap`.
  - Failures are reflected in `Status.Backup.LastFailureReason` and `openbao_backup_failure_total`.

#### Backup Test Scenarios

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

#### Credentials Testing

- **Static credentials:** Test with `accessKeyId` and `secretAccessKey` in Secret.
- **Session tokens:** Test with temporary credentials including `sessionToken`.
- **Missing credentials:** Verify appropriate error when credentials Secret is missing.
- **Invalid credentials:** Verify error is surfaced in `Status.Backup.LastFailureReason`.

These tests run in CI alongside other E2E scenarios.

### 3.5 Upgrade Testing Scenarios

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

### 3.6 Upgrade Unit Test Coverage

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
  - Across the supported Kubernetes versions listed in the compatibility matrix (`docs/compatibility.md`).

New functionality MUST include:

- Unit tests for new internal logic.
- envtest coverage where reconciliation behavior changes.
- E2E coverage for new workflows that affect cluster lifecycle, upgrades, or backups.

This layered, table-driven approach ensures that regressions are caught early and that the Operator maintains its intended behavior as the codebase evolves.

## 6. Coding Standards

This section outlines the coding standards, architectural patterns, and idioms for the OpenBao Operator. **All contributors must adhere strictly to these guidelines.**

**Guiding Principle:** "Clear is better than clever." We prioritize readability, maintainability, and explicit error handling over terse or "magical" code.

### 6.1 General Go Philosophy

* **Formatting:** All code must be formatted with `gofmt` (or `goimports`).
* **Linting:** The code must pass `golangci-lint` with the default configuration provided in the repository.
* **Idioms:** Follow [Effective Go](https://go.dev/doc/effective_go) and [Go Code Review Comments](https://github.com/golang/go/wiki/CodeReviewComments).

### 6.2 Project Structure & Organization

We adhere to the standard Kubebuilder scaffolding.

* `api/v1alpha1/`: Contains CRD structs (`_types.go`) and simple validation methods. **No complex logic here.**
* `internal/controller/`: Contains the Reconciliation logic. Each controller should delegate complex business logic to internal packages.
* `internal/`: Contains the core business logic (e.g., PKI generation, Raft state machine, Backup streaming).
  * *Why?* This prevents other projects from importing our logic as a library, allowing us to change internal APIs freely.
* `cmd/`: Main entry points.

### 6.3 Naming Conventions

* **PascalCase** for exported (public) variables, structs, and functions.
* **camelCase** for unexported (private) variables and functions.
* **Acronyms:** Keep acronyms consistent case (e.g., `ServeHTTP`, `ID`, `URL` — **not** `ServeHttp`, `Id`, `Url`).
* **Getters:** Do **not** use `Get` prefixes for getters.
  * *Bad:* `obj.GetOwner()`
  * *Good:* `obj.Owner()`
* **Interfaces:** Interfaces with one method should be named with an `-er` suffix (e.g., `Reader`, `Writer`, `CertGenerator`).

### 6.4 Error Handling (Strict)

Error handling is the most critical aspect of Go programming.

* **Wrapping:** Always wrap errors when passing them up the stack to add context. Use `%w`.
  ```go
  // Bad
  return err
  
  // Good
  return fmt.Errorf("failed to generate CA certificate: %w", err)
  ```
* **Checking:** Check errors immediately. Do not nest success logic deep inside `else` blocks. Guard clauses are preferred.
  ```go
  // Bad
  if err == nil {
      // do work
  }
  
  // Good
  if err != nil {
      return err
  }
  // do work
  ```
* **Sentinel Errors:** For domain-specific errors that callers need to check (e.g., `ErrPodNotFound`), define them as exported variables.
* **Panics:** **NEVER** panic in the Controller or Internal packages. Panics are only acceptable during `init()` or `main()` startup. If a reconciler panics, it crashes the whole manager.

### 6.5 Kubernetes Operator Patterns

#### 6.5.1 Idempotency

The `Reconcile` function may be called multiple times for the same state.

* **Do not** assume `Reconcile` is only called on changes.
* **Check before Acting:** Before creating a resource (e.g., a Secret), check if it already exists. If it exists, check if it needs updating.
* **Deterministic Naming:** Resource names should be deterministic based on the Parent CR (e.g., `fmt.Sprintf("%s-tls", cluster.Name)`).

#### 6.5.2 Context Usage

* Always pass `ctx context.Context` as the first argument to functions performing I/O or Kubernetes API calls.
* Respect context cancellation. Do not use `time.Sleep()`; use `select { case <-ctx.Done(): ... }`.

#### 6.5.3 Logging

* Use the structured logger provided by `controller-runtime` (`logr.Logger`).
* **Do not** use `fmt.Printf` or global standard `log`.
* **Sensitive Data:** NEVER log Secrets, Keys, or Tokens.
  ```go
  // Good
  log.Info("Reconciling OpenBaoCluster", "namespace", req.Namespace, "name", req.Name)
  ```

#### 6.5.4 Spec vs. Status

* **Spec:** The source of truth (User input). Treat as Read-Only in the Reconciler logic.
* **Status:** The observation of the world.
* **Updates:**
  * Use `r.Update()` for changing the `Spec` or Metadata (rare in reconciliation).
  * Use `r.Status().Update()` (or `Patch`) for updating `Status`.

#### 6.5.5 RBAC Annotations

When creating or managing Kubernetes resources that require RBAC permissions, you **must** add the corresponding `// +kubebuilder:rbac` annotation to the controller file (`internal/controller/openbaocluster/controller.go`).

* **Format:** `// +kubebuilder:rbac:groups=<apiGroup>,resources=<resource>,verbs=<verb1>;<verb2>;...`
* **Location:** Place annotations directly above the `Reconcile` function or the reconciler struct definition.
* **Verbs:** Use appropriate verbs: `get`, `list`, `watch`, `create`, `update`, `patch`, `delete`.
* **Examples:**
  ```go
  // +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterrolebindings,verbs=get;list;watch;create;update;patch;delete
  // +kubebuilder:rbac:groups="",resources=endpoints,verbs=get;list;watch
  ```
* **After adding annotations:**
  1. Run `make manifests` to regenerate `config/rbac/role.yaml`
  2. Verify the generated permissions match your code's requirements
  3. Commit both the code changes and the updated `config/rbac/role.yaml`
* **Common resources that need annotations:**
  * ClusterRoleBindings (cluster-scoped RBAC)
  * RoleBindings (namespace-scoped RBAC)
  * Any new Kubernetes resources created by the operator

### 6.6 Concurrency

* **Goroutines:** Do not spawn unmanaged goroutines in the Reconcile loop. The Controller Manager handles concurrency for you.
* **Shared State:** The Reconciler struct is shared across requests. Do not store request-scoped state (like "current pod list") in the struct fields. Store them in local variables within the `Reconcile` method.

### 6.7 Separation of Concerns (SoC)

We strictly decouple the **Orchestration** (Controller) from the **Implementation** (Business Logic). This ensures the Operator logic is testable without a running Kubernetes cluster.

* **The Reconciler's Job:** The `Reconcile()` method in `internal/controller/` has only three responsibilities:
  1. **Fetch** the Resource (CR) from the Kubernetes API.
  2. **Observe** the current state of the world.
  3. **Delegate** decisions to `internal/` packages.

* **The Internal Package's Job:** All complex logic (e.g., parsing OpenBao configuration, executing Raft joins, generating TLS chains) must reside in `internal/`.
  * *Rule:* Logic in `internal/` should ideally not depend on `controller-runtime` types if possible, making it pure Go logic.

* **API Types are "Dumb":** Structs in `api/v1alpha1` must strictly be data containers. Do not attach business logic methods (like `.ConnectToOpenBao()`) to CRD structs. Only simple data helpers (like `.GetCondition()`) are permitted.

* **No "God Objects":** Do not pass the entire Reconciler struct into helper functions. Pass only the `context`, the `Logger`, and the specific data required (e.g., the `Spec` or the `Client`).

### 6.8 Strict Type Safety

Go is a strongly typed language; we must leverage this to prevent runtime crashes. As this project is built by AI, explicit types help maintain context.

* **No `interface{}` or `any`:** The use of `interface{}` or `any` is **strictly prohibited** unless interacting with a library that forces it (e.g., `json.Unmarshal`). If you must use it, you must add a comment explaining why a concrete type was impossible.
* **Avoid Stringly-Typed Code:** Do not use raw strings for status phases or condition types. Use `type` definitions and constants.
  ```go
  // Bad
  cr.Status.Phase = "Running"

  // Good
  type Phase string
  const (
      PhaseRunning Phase = "Running"
      PhaseFailed  Phase = "Failed"
  )
  cr.Status.Phase = PhaseRunning
  ```
* **Nil Pointer Safety:**
  * **Optional Fields:** In CRDs, use pointers (`*int`, `*string`) *only* for optional fields (marked `+optional`).
  * **Dereferencing:** Never dereference a pointer without checking for `nil` first, or using a safe helper function.
  * **Maps:** Always initialize maps before writing to them. `var m map[string]int` is nil; writing to it causes a panic. Use `m := make(map[string]int)`.
* **Kubernetes Objects:** Do not assume a field in a fetched K8s object is populated. Always verify existence.

### 6.9 DRY (Don't Repeat Yourself) & Go Nuance

While we aim to reduce redundancy, we adhere to the Go proverb: **"A little copying is better than a little dependency."**

* **Rule of Three:** Do not abstract logic into a helper function or shared package until it has been repeated **three times**. Premature abstraction creates rigid code that is hard to refactor later.
* **Avoid "Common" or "Util" Packages:** Do not create a generic `package util`. This becomes a "junk drawer" of unrelated functions (e.g., string manipulation mixed with Kubernetes logic).
  * *Better:* Create specific packages like `pkg/k8sutil`, `pkg/tlsutil`, or `internal/slice`.
* **Test Helpers:** DRY applies less strictly to tests. It is better to have slightly repetitive, readable tests than a complex test framework that hides what is actually being tested.
* **Boilerplate is Okay:** Kubernetes Controllers naturally have error-handling boilerplate (`if err != nil`). **Do not** try to hide this control flow behind complex macros or error-handling wrappers. Explicit control flow is preferred.

### 6.10 Security Best Practices

* **File Permissions:** When writing files (even in tests), use `0600` for secrets and `0644` for public config.
* **Crypto:** Use `crypto/rand` for key generation, never `math/rand`.
* **Input Sanitization:** Validate all inputs from the CRD before using them in shell commands (though we should avoid shell commands in favor of native Go calls) or file paths.

### 6.11 Project-Specific Conventions

#### 6.11.1 Design & Documentation Alignment

* Any non-trivial change (new controllers, new CRD fields, or major behavior changes) must be reflected in:
  * `./docs/architecture.md`
  * `./docs/security.md`
  * `./docs/user-guide/README.md`
  * `./docs/contributing.md`
* Docs and code must remain in sync; do not introduce behavior that contradicts the current design without updating the design first.

#### 6.11.2 Metrics & Logging Conventions

* **Metrics:**
  * All per-cluster metrics MUST use the `openbao_` prefix.
  * Metrics MUST be labeled with at least `namespace` and `name` for the `OpenBaoCluster`.
* **Logging:**
  * Use structured logs with the following standard fields where applicable:
    * `cluster_namespace`
    * `cluster_name`
    * `controller`
    * `reconcile_id`
  * Never log Secrets, unseal keys, tokens, or other sensitive material.

#### 6.11.3 Reconciliation & Backoff Patterns

* Use controller-runtime rate limiting; do **not** implement custom retry loops with `time.Sleep`.
* All external calls (Kubernetes API, OpenBao API, object storage) must:
  * Accept `context.Context`.
  * Use appropriate timeouts and respect context cancellation.

#### 6.11.4 CRD Evolution & Compatibility

* Breaking changes to the `OpenBaoCluster` API (removal/rename of fields, changed semantics) require:
  * Updating CRD versions (e.g., introducing `v1beta1`).
  * A documented migration path in the design docs and user-facing documentation.
* Avoid changing existing field behavior silently; prefer additive, backwards-compatible changes.

#### 6.11.5 Testing Expectations Per Change

* New logic in `internal/` packages MUST have table-driven unit tests.
* New reconciliation flows MUST have at least one envtest-based integration test.
* Changes to upgrade or backup behavior SHOULD be accompanied by at least one E2E scenario update (kind + KUTTL/Ginkgo).

#### 6.11.6 No Shelling Out to Cluster or Cloud Tools

* Controllers and internal packages MUST NOT shell out to tools like `kubectl`, `helm`, or cloud CLIs.
* Always use Kubernetes client-go/controller-runtime clients and official SDKs for cloud/object storage APIs.

## 7. Submitting Changes

1. **Fork the repository** and create a feature branch.
2. **Make your changes** following the coding standards.
3. **Add tests** for new functionality (unit, integration, and E2E as appropriate).
4. **Run tests locally** to ensure everything passes:
   ```sh
   make test
   make lint
   ```
5. **Update documentation** if your changes affect user-facing behavior.
6. **Commit your changes** with clear, descriptive commit messages.
7. **Push to your fork** and open a pull request.

## 8. Additional Resources

- [Kubebuilder Documentation](https://book.kubebuilder.io/introduction.html)
- [Go Code Review Comments](https://github.com/golang/go/wiki/CodeReviewComments)
- [Effective Go](https://go.dev/doc/effective_go)
- Project documentation in `docs/` directory
- [AGENTS.md](../AGENTS.md): Complete coding standards and guidelines (reference for AI agents and detailed conventions)
