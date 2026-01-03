# Project Conventions

Specific conventions for the OpenBao Operator codebase that go beyond standard Go idioms.

## 1. Type Safety

We leverage Go's strong typing to prevent runtime errors.

### Strict Prohibitions

!!! failure "No `any` or `interface{}`"
    The use of `interface{}` or `any` is **strictly prohibited** in core logic.
    It defeats compile-time safety and requires runtime type assertions.

    **Exception:** Interaction with external libraries that require it (e.g., `json.Unmarshal`).

=== ":material-close: Bad Pattern"
    ```go
    func process(data any) error {
        // Runtime crash risk!
        return data.(string)
    }
    ```

=== ":material-check: Good Pattern"
    ```go
    func process(data ConfigType) error {
        // Compile-time safe
        return nil
    }
    ```

### Enum Constants

Avoid "stringly-typed" code. Use defined types for status fields.

```go
type Phase string

const (
    PhaseRunning Phase = "Running"
    PhaseFailed  Phase = "Failed"
)
```

## 2. Code Organization (DRY)

### The Rule of Three

!!! note "Don't Abstract Too Early"
    Do not create a helper function or shared package until logic is repeated **three times**.

    1.  **First time:** Write it inline.
    2.  **Second time:** Copy-paste (yes, really).
    3.  **Third time:** Refactor into a shared helper.

### Package Naming

Avoid generic names that become "junk drawers".

| :material-close: Avoid | :material-check: Prefer |
| :--- | :--- |
| `util`, `common`, `shared` | `slice`, `pointer`, `k8sutil` |
| `types`, `models` | `api`, `config`, `schema` |

## 3. Review Hygiene

To keep code reviews high-signal, adhere to these scoping rules:

- [ ] **One Theme**: A PR should fix a bug OR add a feature OR refactor. Not all three.
- [ ] **No Drive-By Changes**: Do not reformat unrelated files.
- [ ] **Update Generated Files**: If you change `api/`, run `make manifests generate`.

## 4. Observability Standards

### Metrics

Must use the `openbao_` prefix and standard labels.

| Type | Name | Labels |
| :--- | :--- | :--- |
| **Gauge** | `openbao_cluster_replicas` | `namespace`, `name` |
| **Counter** | `openbao_reconcile_errors_total` | `controller`, `type` |

### Logging

See [Kubernetes Patterns](kubernetes-patterns.md#4-structured-logging) for detailed logging rules.

## 5. Testing Requirements

Every change requires verification.

| Change Type | Required Test | Command |
| :--- | :--- | :--- |
| **Business Logic** | Unit Test (Table-driven) | `make test` |
| **HCL Config** | Golden File Update | `make test-update-golden` |
| **Controller Logic** | EnvTest Integration | `make test-integration` |
| **Critical Path** | End-to-End Test | `make test-e2e` |

!!! tip "Golden Files"
    Changes to `internal/config/builder.go` will often break tests.
    Run `make test-update-golden` to update the expected output in `internal/config/testdata/`.

## 6. CRD Evolution

Breaking changes to `OpenBaoCluster` are **expensive**.

1. **Prefer Additive Changes**: Add new optional fields; do not rename or remove existing ones.
2. **Versioning**: Significant changes require a new API version (e.g., `v1beta1`).
3. **Migration**: You must document how to migrate from the old version.
