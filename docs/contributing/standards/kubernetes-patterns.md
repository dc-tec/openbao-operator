# Kubernetes Operator Patterns

Patterns and conventions specific to Kubernetes operator development.

## Idempotency

The `Reconcile` function may be called multiple times for the same state.

### Key Principles

- **Do not** assume `Reconcile` is only called on changes
- **Check before acting:** Before creating a resource, check if it already exists
- **Update if needed:** If a resource exists, check if it needs updating
- **Deterministic naming:** Resource names must be deterministic based on the parent CR

```go
// Resource names are deterministic
secretName := fmt.Sprintf("%s-tls", cluster.Name)

// Check before creating
var existing corev1.Secret
if err := r.Client.Get(ctx, types.NamespacedName{
    Name:      secretName,
    Namespace: cluster.Namespace,
}, &existing); err != nil {
    if apierrors.IsNotFound(err) {
        // Create new resource
        return r.createSecret(ctx, cluster)
    }
    return err
}
// Resource exists - check if update needed
```

## Context Usage

- **Always pass `ctx context.Context`** as the first argument to functions performing I/O
- **Respect context cancellation** - do not use `time.Sleep()`

```go
// Bad
time.Sleep(5 * time.Second)

// Good
select {
case <-ctx.Done():
    return ctx.Err()
case <-time.After(5 * time.Second):
}
```

For blocking operations, use controller-runtime's rate limiting instead of manual sleeps.

## Logging

Use the structured logger provided by controller-runtime:

```go
log := log.FromContext(ctx)

// Good - structured with key-value pairs
log.Info("Reconciling OpenBaoCluster",
    "namespace", req.Namespace,
    "name", req.Name,
    "phase", cluster.Status.Phase,
)

// Log at appropriate levels
log.V(1).Info("Debug: checking TLS certificates")  // Debug level
log.Error(err, "Failed to update status")          // Error level
```

### Standard Log Fields

Use consistent field names:

| Field | Description |
|-------|-------------|
| `cluster_namespace` | Namespace of the OpenBaoCluster |
| `cluster_name` | Name of the OpenBaoCluster |
| `controller` | Controller name |
| `reconcile_id` | Unique reconciliation ID |

### Never Log Sensitive Data

**NEVER** log secrets, unseal keys, tokens, or other sensitive material:

```go
// NEVER DO THIS
log.Info("Created secret", "data", secret.Data)

// Instead, log metadata only
log.Info("Created secret", "name", secret.Name, "type", secret.Type)
```

## Spec vs Status

- **Spec:** The desired state (user input). Treat as **read-only** in reconcilers.
- **Status:** The observed state. Update via `r.Status().Update()` or `Patch`.

```go
// Update Spec or Metadata (rare)
if err := r.Update(ctx, cluster); err != nil {
    return ctrl.Result{}, err
}

// Update Status (common)
cluster.Status.Phase = v1alpha1.PhaseRunning
if err := r.Status().Update(ctx, cluster); err != nil {
    return ctrl.Result{}, err
}
```

## RBAC (Zero Trust Model)

**We do NOT use standard Kubebuilder RBAC annotations** for the OpenBaoCluster controller.

### Why?

The operator follows a Zero Trust model:

- Controller does **not** have cluster-wide permissions
- RBAC is dynamically granted per-namespace by the Provisioner
- This prevents privilege escalation

### What This Means

- Do **not** add `// +kubebuilder:rbac` annotations to `internal/controller/openbaocluster/`
- New permissions must be added to `internal/provisioner/rbac.go`
- Update `internal/provisioner/rbac_test.go` to lock the expected permissions

## Concurrency

### No Unmanaged Goroutines

Do not spawn unmanaged goroutines in the Reconcile loop. The controller-runtime manager handles concurrency:

```go
// Bad - unmanaged goroutine
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    go r.doSomething()  // DON'T DO THIS
}

// Good - let controller-runtime handle concurrency
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    if err := r.doSomething(ctx); err != nil {
        return ctrl.Result{}, err
    }
    return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}
```

### No Shared Request State

The Reconciler struct is shared across requests. Do not store request-scoped state in struct fields:

```go
// Bad - stores state in reconciler
type Reconciler struct {
    CurrentPods []corev1.Pod  // DON'T DO THIS
}

// Good - use local variables
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    var pods corev1.PodList  // Local to this reconciliation
    if err := r.Client.List(ctx, &pods, ...); err != nil {
        return ctrl.Result{}, err
    }
}
```
