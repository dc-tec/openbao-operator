# OpenBao Operator - Coding Standards & Guidelines

This document outlines the coding standards, architectural patterns, and idioms for the OpenBao Operator. **All AI agents and human contributors must adhere strictly to these guidelines.**

**Guiding Principle:** "Clear is better than clever." We prioritize readability, maintainability, and explicit error handling over terse or "magical" code.

---

## 1. General Go Philosophy

* **Formatting:** All code must be formatted with `gofmt` (or `goimports`).
* **Linting:** The code must pass `golangci-lint` with the default configuration provided in the repository.
* **Idioms:** Follow [Effective Go](https://go.dev/doc/effective_go) and [Go Code Review Comments](https://github.com/golang/go/wiki/CodeReviewComments).

---

## 2. Project Structure & Organization

We adhere to a modified Kubebuilder scaffolding that promotes internal encapsulation.

* `api/v1alpha1/`: Contains CRD structs (`_types.go`) and simple validation methods. **No complex logic here.**
* `cmd/`: Main entry points.
    * `cmd/controller/`: The Operator Controller binary.
    * `cmd/provisioner/`: The Provisioner binary (handles namespace creation and initial RBAC).
* `internal/controller/`: Contains the Reconciliation logic (e.g., `openbaocluster/`). Each controller should delegate complex business logic to specific internal packages.
* `internal/`: Contains the core business logic (e.g., `infra/` for K8s resources, `config/` for HCL generation, `backup/` for streaming).
    * *Why?* This prevents other projects from importing our logic as a library, allowing us to change internal APIs freely.

---

## 3. Naming Conventions

* **PascalCase** for exported (public) variables, structs, and functions.
* **camelCase** for unexported (private) variables and functions.
* **Acronyms:** Keep acronyms consistent case (e.g., `ServeHTTP`, `ID`, `URL` — **not** `ServeHttp`, `Id`, `Url`).
* **Getters:** Do **not** use `Get` prefixes for getters.
    * *Bad:* `obj.GetOwner()`
    * *Good:* `obj.Owner()`
* **Interfaces:** Interfaces with one method should be named with an `-er` suffix (e.g., `Reader`, `Writer`, `CertGenerator`).

---

## 4. Error Handling (Strict)

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

---

## 5. Kubernetes Operator Patterns

### 5.1 Idempotency
The `Reconcile` function may be called multiple times for the same state.
* **Do not** assume `Reconcile` is only called on changes.
* **Check before Acting:** Before creating a resource (e.g., a Secret), check if it already exists. If it exists, check if it needs updating.
* **Deterministic Naming:** Resource names should be deterministic based on the Parent CR (e.g., `fmt.Sprintf("%s-tls", cluster.Name)`).

### 5.2 Context Usage
* Always pass `ctx context.Context` as the first argument to functions performing I/O or Kubernetes API calls.
* Respect context cancellation. Do not use `time.Sleep()`; use `select { case <-ctx.Done(): ... }`.

### 5.3 Logging
* Use the structured logger provided by `controller-runtime` (`logr.Logger`).
* **Do not** use `fmt.Printf` or global standard `log`.
* **Sensitive Data:** NEVER log Secrets, Keys, or Tokens.
    ```go
    // Good
    log.Info("Reconciling OpenBaoCluster", "namespace", req.Namespace, "name", req.Name)
    ```

### 5.4 Spec vs. Status
* **Spec:** The source of truth (User input). Treat as Read-Only in the Reconciler logic.
* **Status:** The observation of the world.
* **Updates:**
    * Use `r.Update()` for changing the `Spec` or Metadata (rare in reconciliation).
    * Use `r.Status().Update()` (or `Patch`) for updating `Status`.

### 5.5 RBAC & Zero Trust (Crucial)
**We DO NOT use standard Kubebuilder RBAC annotations (`// +kubebuilder:rbac`) for the OpenBaoCluster controller.**

* **Why?** This operator follows a Zero Trust model. The Controller does **not** have cluster-wide list/watch permissions on Secrets or ConfigMaps.
* **Mechanism:** RBAC is manually maintained via the `Provisioner` component.
    * The Provisioner creates a specific `Role` and `RoleBinding` in the tenant namespace when a new tenant is onboarded.
    * The Controller operates *only* within these explicit permissions.
* **Action:** Do **not** add `+kubebuilder:rbac` annotations to `internal/controller/openbaocluster/controller.go`. If new permissions are needed, they must be added to the Provisioner logic (`internal/provisioner/rbac.go`).

---

## 6. Concurrency

* **Goroutines:** Do not spawn unmanaged goroutines in the Reconcile loop. The Controller Manager handles concurrency for you.
* **Shared State:** The Reconciler struct is shared across requests. Do not store request-scoped state (like "current pod list") in the struct fields. Store them in local variables within the `Reconcile` method.

---

## 7. Testing Standards

* **Table-Driven Tests:** Use table-driven tests for unit logic (`internal/`).
    ```go
    func TestCertGeneration(t *testing.T) {
        tests := []struct{
            name    string
            input   Config
            wantErr bool
        }{
            {"valid config", validConfig, false},
            {"missing host", invalidConfig, true},
        }
        // ... loop ...
    }
    ```
* **Golden File Testing:** HCL configuration generation tests use golden files stored in `internal/config/testdata/` to ensure exact output matching. When modifying `internal/config/builder.go` or related HCL generation logic, you **must** update golden files by running `UPDATE_GOLDEN=true go test ./internal/config/... -v`. Review the generated golden files carefully before committing.
* **EnvTest:** Use `envtest` (part of controller-runtime) for integration tests to verify CRD interactions.
* **Mocks:** Prefer defining small interfaces in your consumer package and mocking those, rather than mocking entire external libraries.

---

## 8. Specific Directives for AI Agents

When generating code:
1.  **Completeness:** Do not generate placeholders like `// TODO: Implement this` unless specifically asked to skeleton a file. Attempt to implement the logic.
2.  **Imports:** Ensure all imports are standard or part of the `go.mod`. Do not hallucinate external libraries. Group imports: Standard Lib, Third Party, Local.
3.  **Comments:**
    * Exported functions **must** have a GoDoc comment explaining *what* they do.
    * Complex logic blocks **must** have inline comments explaining *why* (not just what).
4.  **Literals:** Avoid magic numbers or string literals. Define constants in a `const` block at the top of the file or in the `internal/constants/` package.

---

## 9. Security Best Practices

* **File Permissions:** When writing files (even in tests), use `0600` for secrets and `0644` for public config.
* **Crypto:** Use `crypto/rand` for key generation, never `math/rand`.
* **Input Sanitization:** Validate all inputs from the CRD before using them in shell commands (though we should avoid shell commands in favor of native Go calls) or file paths.

---

## 10. Separation of Concerns (SoC)

We strictly decouple the **Orchestration** (Controller) from the **Implementation** (Business Logic).

  * **The Reconciler's Job:** The `Reconcile()` method in `internal/controller/` has only three responsibilities:
    1.  **Fetch** the Resource (CR) from the Kubernetes API.
    2.  **Observe** the current state of the world.
    3.  **Delegate** decisions to `internal/` packages (e.g., `inframanager`, `certmanager`).

  * **The Internal Package's Job:** All complex logic (e.g., parsing Vault configuration, executing Raft joins, generating TLS chains) must reside in `internal/`.
      * *Rule:* Logic in `internal/` should ideally not depend on `controller-runtime` types if possible, making it pure Go logic.

  * **API Types are "Dumb":** Structs in `api/v1alpha1` must strictly be data containers. Do not attach business logic methods (like `.ConnectToVault()`) to CRD structs. Only simple data helpers (like `.GetCondition()`) are permitted.

  * **No "God Objects":** Do not pass the entire Reconciler struct into helper functions. Pass only the `context`, the `Logger`, and the specific data required (e.g., the `Spec` or the `Client`).

---

## 11. Strict Type Safety

Go is a strongly typed language; we must leverage this to prevent runtime crashes.

  * **No `interface{}` or `any`:** The use of `interface{}` or `any` is **strictly prohibited** unless interacting with a library that forces it (e.g., `json.Unmarshal` or HCL cty conversion). If you must use it, you must add a comment explaining why.
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

---

## 12. DRY (Don't Repeat Yourself) & Go Nuance

While we aim to reduce redundancy, we adhere to the Go proverb: **"A little copying is better than a little dependency."**

  * **Rule of Three:** Do not abstract logic into a helper function or shared package until it has been repeated **three times**. Premature abstraction creates rigid code.
  * **Avoid "Common" or "Util" Packages:** Do not create a generic `package util`. This becomes a "junk drawer" of unrelated functions.
      * *Better:* Create specific packages like `internal/k8sutil` or `internal/tlsutil`.
  * **Test Helpers:** DRY applies less strictly to tests. It is better to have slightly repetitive, readable tests than a complex test framework that hides what is actually being tested.

---

## 13. Project-Specific Conventions

### 13.1 Design & Documentation Alignment

* Any non-trivial change (new controllers, new CRD fields, or major behavior changes) must be reflected in:
 * `docs/architecture.md`
 * `docs/user-guide/README.md`
* Docs and code must remain in sync.

### 13.2 Metrics & Logging Conventions

* **Metrics:**
  * All per-cluster metrics MUST use the `openbao_` prefix.
  * Metrics MUST be labeled with at least `namespace` and `name` for the `OpenBaoCluster`.
* **Logging:**
  * Use structured logs with the following standard fields where applicable:
    * `cluster_namespace`
    * `cluster_name`
    * `controller`
    * `reconcile_id`

### 13.3 Reconciliation & Backoff Patterns

* Use controller-runtime rate limiting; do **not** implement custom retry loops with `time.Sleep`.
* All external calls (Kubernetes API, OpenBao API, object storage) must accept `context.Context`.

### 13.4 CRD Evolution & Compatibility

* Breaking changes to the `OpenBaoCluster` API (removal/rename of fields, changed semantics) require:
  * Updating CRD versions (e.g., introducing `v1beta1`).
  * A documented migration path.
* Avoid changing existing field behavior silently; prefer additive, backwards-compatible changes.

### 13.5 Testing Expectations Per Change

* New logic in `internal/` packages MUST have table-driven unit tests.
* Changes to HCL generation MUST include updated Golden Files.
* New reconciliation flows MUST have at least one envtest-based integration test.

### 13.6 No Shelling Out

* Controllers and internal packages MUST NOT shell out to tools like `kubectl`, `helm`, or cloud CLIs.
* Always use Kubernetes client-go/controller-runtime clients and official SDKs.