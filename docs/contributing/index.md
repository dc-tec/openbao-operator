# Contributing: OpenBao Operator

This guide helps developers get started with contributing to the OpenBao Operator project. It covers development environment setup, build instructions, and testing strategy.

## 1. Submitting Changes

1. **Fork the repository** and create a feature branch.
2. **Make your changes** following the coding standards.
3. **Add tests** for new functionality (unit, integration, and E2E as appropriate).
4. **Update golden files** if you modified HCL generation logic (`internal/config/builder.go`):

   ```sh
   make test-update-golden
   ```

   Review the changes in `internal/config/testdata/*.hcl` files carefully before committing.
5. **Run tests locally** to ensure everything passes:

   ```sh
   make test
   make lint
   ```

6. **Update documentation** if your changes affect user-facing behavior.
7. **Commit your changes** with clear, descriptive commit messages.
8. **Push to your fork** and open a pull request.

## 2. Coding Standards

This section outlines the coding standards, architectural patterns, and idioms for the OpenBao Operator. **All contributors must adhere strictly to these guidelines.**

**Guiding Principle:** "Clear is better than clever." We prioritize readability, maintainability, and explicit error handling over terse or "magical" code.

### 2.1 General Go Philosophy

- **Formatting:** All code must be formatted with `gofmt` (or `goimports`).
- **Linting:** The code must pass `golangci-lint` with the default configuration provided in the repository.
- **Idioms:** Follow [Effective Go](https://go.dev/doc/effective_go) and [Go Code Review Comments](https://github.com/golang/go/wiki/CodeReviewComments).

### 2.2 Project Structure & Organization

We adhere to the standard Kubebuilder scaffolding.

- `api/v1alpha1/`: Contains CRD structs (`_types.go`) and simple validation methods. **No complex logic here.**
- `internal/controller/`: Contains the Reconciliation logic. Each controller should delegate complex business logic to internal packages.
- `internal/`: Contains the core business logic (e.g., PKI generation, Raft state machine, Backup streaming).
  - *Why?* This prevents other projects from importing our logic as a library, allowing us to change internal APIs freely.
- `cmd/`: Main entry points.

### 2.3 Naming Conventions

- **PascalCase** for exported (public) variables, structs, and functions.
- **camelCase** for unexported (private) variables and functions.
- **Acronyms:** Keep acronyms consistent case (e.g., `ServeHTTP`, `ID`, `URL` — **not** `ServeHttp`, `Id`, `Url`).
- **Getters:** Do **not** use `Get` prefixes for getters.
  - *Bad:* `obj.GetOwner()`
  - *Good:* `obj.Owner()`
- **Interfaces:** Interfaces with one method should be named with an `-er` suffix (e.g., `Reader`, `Writer`, `CertGenerator`).

### 2.4 Error Handling (Strict)

Error handling is the most critical aspect of Go programming.

- **Wrapping:** Always wrap errors when passing them up the stack to add context. Use `%w`.

  ```go
  // Bad
  return err
  
  // Good
  return fmt.Errorf("failed to generate CA certificate: %w", err)
  ```

- **Checking:** Check errors immediately. Do not nest success logic deep inside `else` blocks. Guard clauses are preferred.

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

- **Sentinel Errors:** For domain-specific errors that callers need to check (e.g., `ErrPodNotFound`), define them as exported variables.

- **Panics:** **NEVER** panic in the Controller or Internal packages. Panics are only acceptable during `init()` or `main()` startup. If a reconciler panics, it crashes the whole manager.

### 2.5 Kubernetes Operator Patterns

#### 2.5.1 Idempotency

The `Reconcile` function may be called multiple times for the same state.

- **Do not** assume `Reconcile` is only called on changes.
- **Check before Acting:** Before creating a resource (e.g., a Secret), check if it already exists. If it exists, check if it needs updating.
- **Deterministic Naming:** Resource names should be deterministic based on the Parent CR (e.g., `fmt.Sprintf("%s-tls", cluster.Name)`).

#### 2.5.2 Context Usage

- Always pass `ctx context.Context` as the first argument to functions performing I/O or Kubernetes API calls.
- Respect context cancellation. Do not use `time.Sleep()`; use `select { case <-ctx.Done(): ... }`.

#### 2.5.3 Logging

- Use the structured logger provided by `controller-runtime` (`logr.Logger`).
- **Do not** use `fmt.Printf` or global standard `log`.
- **Sensitive Data:** NEVER log Secrets, Keys, or Tokens.

  ```go
  // Good
  log.Info("Reconciling OpenBaoCluster", "namespace", req.Namespace, "name", req.Name)
  ```

#### 2.5.4 Spec vs. Status

- **Spec:** The source of truth (User input). Treat as Read-Only in the Reconciler logic.
- **Status:** The observation of the world.
- **Updates:**
  - Use `r.Update()` for changing the `Spec` or Metadata (rare in reconciliation).
  - Use `r.Status().Update()` (or `Patch`) for updating `Status`.

#### 2.5.5 RBAC Annotations

When creating or managing Kubernetes resources that require RBAC permissions, you **must** add the corresponding `// +kubebuilder:rbac` annotation to the controller file (`internal/controller/openbaocluster/controller.go`).

- **Format:** `// +kubebuilder:rbac:groups=<apiGroup>,resources=<resource>,verbs=<verb1>;<verb2>;...`
- **Location:** Place annotations directly above the `Reconcile` function or the reconciler struct definition.
- **Verbs:** Use appropriate verbs: `get`, `list`, `watch`, `create`, `update`, `patch`, `delete`.
- **Examples:**

  ```go
  // +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterrolebindings,verbs=get;list;watch;create;update;patch;delete
  // +kubebuilder:rbac:groups="",resources=endpoints,verbs=get;list;watch
  ```

- **After adding annotations:**
  1. Run `make manifests` to regenerate `config/rbac/role.yaml`
  2. Verify the generated permissions match your code's requirements
  3. Commit both the code changes and the updated `config/rbac/role.yaml`

- **Common resources that need annotations:**
  - ClusterRoleBindings (cluster-scoped RBAC)
  - RoleBindings (namespace-scoped RBAC)
  - Any new Kubernetes resources created by the operator

### 2.6 Concurrency

- **Goroutines:** Do not spawn unmanaged goroutines in the Reconciler loop. The Controller Manager handles concurrency for you.
- **Shared State:** The Reconciler struct is shared across requests. Do not store request-scoped state (like "current pod list") in the struct fields. Store them in local variables within the `Reconcile` method.

### 2.7 Separation of Concerns (SoC)

We strictly decouple the **Orchestration** (Controller) from the **Implementation** (Business Logic). This ensures the Operator logic is testable without a running Kubernetes cluster.

- **The Reconciler's Job:** The `Reconcile()` method in `internal/controller/` has only three responsibilities:
  1. **Fetch** the Resource (CR) from the Kubernetes API.
  2. **Observe** the current state of the world.
  3. **Delegate** decisions to `internal/` packages.

- **The Internal Package's Job:** All complex logic (e.g., parsing OpenBao configuration, executing Raft joins, generating TLS chains) must reside in `internal/`.
  - *Rule:* Logic in `internal/` should ideally not depend on `controller-runtime` types if possible, making it pure Go logic.

- **API Types are "Dumb":** Structs in `api/v1alpha1` must strictly be data containers. Do not attach business logic methods (like `.ConnectToOpenBao()`) to CRD structs. Only simple data helpers (like `.GetCondition()`) are permitted.

- **No "God Objects":** Do not pass the entire Reconciler struct into helper functions. Pass only the `context`, the `Logger`, and the specific data required (e.g., the `Spec` or the `Client`).

### 2.8 Strict Type Safety

Go is a strongly typed language; we must leverage this to prevent runtime crashes. As this project is built by AI, explicit types help maintain context.

- **No `interface{}` or `any`:** The use of `interface{}` or `any` is **strictly prohibited** unless interacting with a library that forces it (e.g., `json.Unmarshal`). If you must use it, you must add a comment explaining why a concrete type was impossible.
- **Avoid Stringly-Typed Code:** Do not use raw strings for status phases or condition types. Use `type` definitions and constants.

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

- **Nil Pointer Safety:**
  - **Optional Fields:** In CRDs, use pointers (`*int`, `*string`) *only* for optional fields (marked `+optional`).
  - **Dereferencing:** Never dereference a pointer without checking for `nil` first, or using a safe helper function.
  - **Maps:** Always initialize maps before writing to them. `var m map[string]int` is nil; writing to it causes a panic. Use `m := make(map[string]int)`.

- **Kubernetes Objects:** Do not assume a field in a fetched K8s object is populated. Always verify existence.

### 2.9 DRY (Don't Repeat Yourself) & Go Nuance

While we aim to reduce redundancy, we adhere to the Go proverb: **"A little copying is better than a little dependency."**

- **Rule of Three:** Do not abstract logic into a helper function or shared package until it has been repeated **three times**. Premature abstraction creates rigid code that is hard to refactor later.
- **Avoid "Common" or "Util" Packages:** Do not create a generic `package util`. This becomes a "junk drawer" of unrelated functions (e.g., string manipulation mixed with Kubernetes logic).
  - *Better:* Create specific packages like `pkg/k8sutil`, `pkg/tlsutil`, or `internal/slice`.
- **Test Helpers:** DRY applies less strictly to tests. It is better to have slightly repetitive, readable tests than a complex test framework that hides what is actually being tested.
- **Boilerplate is Okay:** Kubernetes Controllers naturally have error-handling boilerplate (`if err != nil`). **Do not** try to hide this control flow behind complex macros or error-handling wrappers. Explicit control flow is preferred.

### 2.10 Security Best Practices

- **File Permissions:** When writing files (even in tests), use `0600` for secrets and `0644` for public config.
- **Crypto:** Use `crypto/rand` for key generation, never `math/rand`.
- **Input Sanitization:** Validate all inputs from the CRD before using them in shell commands (though we should avoid shell commands in favor of native Go calls) or file paths.

### 2.11 Project-Specific Conventions

#### 2.11.1 Design & Documentation Alignment

- Any non-trivial change (new controllers, new CRD fields, or major behavior changes) must be reflected in:
  - `./docs/architecture.md`
  - `./docs/security.md`
  - `./docs/user-guide/README.md`
  - `./docs/contributing.md`
- Docs and code must remain in sync; do not introduce behavior that contradicts the current design without updating the design first.

#### 2.11.2 Metrics & Logging Conventions

- **Metrics:**
  - All per-cluster metrics MUST use the `openbao_` prefix.
  - Metrics MUST be labeled with at least `namespace` and `name` for the `OpenBaoCluster`.
- **Logging:**
  - Use structured logs with the following standard fields where applicable:
    - `cluster_namespace`
    - `cluster_name`
    - `controller`
    - `reconcile_id`
  - Never log Secrets, unseal keys, tokens, or other sensitive material.

#### 2.11.3 Reconciliation & Backoff Patterns

- Use controller-runtime rate limiting; do **not** implement custom retry loops with `time.Sleep`.
- All external calls (Kubernetes API, OpenBao API, object storage) must:
  - Accept `context.Context`.
  - Use appropriate timeouts and respect context cancellation.

#### 2.11.4 CRD Evolution & Compatibility

- Breaking changes to the `OpenBaoCluster` API (removal/rename of fields, changed semantics) require:
  - Updating CRD versions (e.g., introducing `v1beta1`).
  - A documented migration path in the design docs and user-facing documentation.
- Avoid changing existing field behavior silently; prefer additive, backwards-compatible changes.

#### 2.11.5 Testing Expectations Per Change

- New logic in `internal/` packages MUST have table-driven unit tests.
- Changes to HCL generation (`internal/config/builder.go`) MUST update golden files by running `make test-update-golden` and reviewing the changes.
- New reconciliation flows MUST have at least one envtest-based integration test.
- Changes to upgrade or backup behavior SHOULD be accompanied by at least one E2E scenario update (kind + KUTTL/Ginkgo).

#### 2.11.6 No Shelling Out to Cluster or Cloud Tools

- Controllers and internal packages MUST NOT shell out to tools like `kubectl`, `helm`, or cloud CLIs.
- Always use Kubernetes client-go/controller-runtime clients and official SDKs for cloud/object storage APIs.

## 3. Additional Resources

- [Kubebuilder Documentation](https://book.kubebuilder.io/introduction.html)
- [Go Code Review Comments](https://github.com/golang/go/wiki/CodeReviewComments)
- [Effective Go](https://go.dev/doc/effective_go)
- Project documentation in `docs/` directory
- [Documentation Style Guide](style-guide.md): Guidelines for writing and formatting documentation.
- `AGENTS.md` (at repo root): Complete coding standards and guidelines for AI agents
