# Project Conventions

Project-specific conventions for the OpenBao Operator beyond general Go and Kubernetes patterns.

## Type Safety

Go is strongly typed. We leverage this to prevent runtime crashes.

### No `interface{}` or `any`

The use of `interface{}` or `any` is **strictly prohibited** unless required by external libraries:

```go
// Bad
func process(data interface{}) error

// Good
func process(data ConfigData) error
```

If you must use `interface{}` (e.g., for `json.Unmarshal`), add a comment explaining why.

### Avoid Stringly-Typed Code

Use type definitions and constants instead of raw strings:

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

### Nil Pointer Safety

- Use pointers only for optional CRD fields (marked `+optional`)
- Never dereference without checking for nil
- Always initialize maps before writing

```go
// CRD optional field
Replicas *int `json:"replicas,omitempty"`

// Safe access
replicas := 3 // default
if cluster.Spec.Replicas != nil {
    replicas = *cluster.Spec.Replicas
}

// Map initialization
labels := make(map[string]string)  // Not: var labels map[string]string
labels["app"] = "openbao"
```

## DRY Guidelines

### Rule of Three

Do not abstract into a helper function until logic is repeated **three times**:

```go
// First occurrence - inline is fine
// Second occurrence - still inline
// Third occurrence - now extract to helper
```

### No "Common" or "Util" Packages

Avoid generic utility packages like `package util`. Instead:

```go
// Bad
package util  // Becomes a junk drawer

// Good
package slice     // internal/slice
package k8sutil   // pkg/k8sutil  
package tlsutil   // internal/tlsutil
```

### Test Helpers

DRY applies less strictly to tests. Prefer readable, slightly repetitive tests over complex test frameworks.

## Metrics Conventions

All per-cluster metrics must follow these conventions:

| Requirement | Example |
|-------------|---------|
| Prefix with `openbao_` | `openbao_cluster_replicas` |
| Label with `namespace` | `namespace="production"` |
| Label with `name` | `name="my-cluster"` |

```go
var clusterReplicas = prometheus.NewGaugeVec(
    prometheus.GaugeOpts{
        Name: "openbao_cluster_replicas",
        Help: "Number of replicas in the cluster",
    },
    []string{"namespace", "name"},
)
```

## Logging Conventions

Use structured logging with standard fields:

```go
log.Info("Reconciling cluster",
    "cluster_namespace", cluster.Namespace,
    "cluster_name", cluster.Name,
    "controller", "openbaocluster",
    "reconcile_id", reconcileID,
)
```

## Reconciliation Patterns

### Use Controller-Runtime Rate Limiting

Do **not** implement custom retry loops with `time.Sleep`:

```go
// Bad
for {
    if err := doWork(); err != nil {
        time.Sleep(5 * time.Second)
        continue
    }
    break
}

// Good - return with requeue
if err := doWork(); err != nil {
    return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
}
```

### Context Requirements

All external calls must:

- Accept `context.Context` as first parameter
- Use appropriate timeouts
- Respect context cancellation

## CRD Evolution

Breaking changes to the `OpenBaoCluster` API require:

1. **New CRD version** (e.g., `v1beta1`)
2. **Documented migration path** in design docs
3. **User-facing documentation** updates

Prefer additive, backwards-compatible changes over modifications.

## Testing Requirements

| Change Type | Required Tests |
|-------------|----------------|
| Logic in `internal/` | Table-driven unit tests |
| HCL generation | Updated golden files |
| Reconciliation flows | At least one envtest integration test |
| Upgrade/backup changes | At least one E2E scenario update |

### Golden Files

Changes to HCL generation require updating golden files:

```sh
make test-update-golden
```

Review the changes in `internal/config/testdata/*.hcl` carefully before committing.
