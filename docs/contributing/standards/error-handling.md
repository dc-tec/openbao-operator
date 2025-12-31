# Error Handling

Error handling is the most critical aspect of Go programming. The OpenBao Operator follows strict patterns to ensure errors are properly propagated and debuggable.

## Error Wrapping

**Always wrap errors** when passing them up the stack to add context:

```go
// Bad - loses context
return err

// Good - adds context
return fmt.Errorf("failed to generate CA certificate: %w", err)
```

Use `%w` for wrapping to preserve the error chain for `errors.Is()` and `errors.As()`.

## Error Checking

Check errors immediately. Use guard clauses instead of nested else blocks:

```go
// Bad - success logic nested in else
if err == nil {
    // do work
} else {
    return err
}

// Good - guard clause
if err != nil {
    return fmt.Errorf("failed to fetch secret: %w", err)
}
// do work
```

## Sentinel Errors

For domain-specific errors that callers need to check, define exported sentinel errors:

```go
var (
    ErrPodNotFound     = errors.New("pod not found")
    ErrClusterNotReady = errors.New("cluster not ready")
    ErrQuorumLost      = errors.New("raft quorum lost")
)
```

Callers can then use:

```go
if errors.Is(err, ErrPodNotFound) {
    // handle missing pod
}
```

## Panics

**NEVER panic** in the Controller or Internal packages.

Panics are only acceptable during:

- `init()` functions
- `main()` startup
- Programmer errors that should never occur

If a reconciler panics, it crashes the entire manager, affecting all clusters.

```go
// Bad - panics in reconciler
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    if config == nil {
        panic("config cannot be nil") // DON'T DO THIS
    }
}

// Good - return error
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    if config == nil {
        return ctrl.Result{}, fmt.Errorf("internal error: config is nil")
    }
}
```

## Error Messages

Write error messages that are:

- **Lowercase** (errors can be wrapped and combined)
- **Specific** (what operation failed)
- **Actionable** (hint at the cause when possible)

```go
// Bad
return errors.New("Error occurred")

// Good
return fmt.Errorf("failed to read TLS secret %s/%s: %w", namespace, name, err)
```

## Kubernetes Client Errors

For Kubernetes client errors, check for common cases:

```go
import apierrors "k8s.io/apimachinery/pkg/api/errors"

if err := r.Client.Get(ctx, key, &secret); err != nil {
    if apierrors.IsNotFound(err) {
        // Resource doesn't exist - may need to create it
        return r.createSecret(ctx, ...)
    }
    // Other errors should be propagated
    return fmt.Errorf("failed to get secret %s: %w", key, err)
}
```
