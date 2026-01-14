# Error Handling

Error handling is critical for the stability and debuggability of the OpenBao Operator. We follow strict patterns to ensure errors are propagated with context and handled gracefully.

## 1. Wrapping Errors

**Always** wrap errors when passing them up the stack. This preserves the original error type (for checking) while adding valuable context (for debugging).

=== ":material-check: Good Pattern"
    ```go
    // Adds context and preserves the chain
    return fmt.Errorf("failed to create secret: %w", err)
    ```

=== ":material-close: Bad Pattern"
    ```go
    // Loses the "where" and "why"
    return err
    ```

## 2. Guard Clauses

Handle errors immediately. Check for failure first, return early, and keep the "happy path" unindented.

=== ":material-check: Good Pattern"
    ```go
    if err := r.Client.Get(ctx, key, obj); err != nil {
        return fmt.Errorf("failed to fetch object: %w", err)
    }

    // Do work on obj...
    ```

=== ":material-close: Bad Pattern"
    ```go
    if err := r.Client.Get(ctx, key, obj); err == nil {
        // Do work on obj...
    } else {
        return err
    }
    ```

## 3. Kubernetes Errors

When interacting with the Kubernetes API, check for specific error types using `apierrors`.

=== ":material-check: Good Pattern"
    ```go
    import apierrors "k8s.io/apimachinery/pkg/api/errors"

    if err := r.Client.Get(ctx, key, secret); err != nil {
        if apierrors.IsNotFound(err) {
            // Resource missing is expected here, create it
            return r.createSecret(ctx, ...)
        }
        // Unexpected error, bubble it up
        return fmt.Errorf("failed to get secret: %w", err)
    }
    ```

## 4. Checkable Errors

Define exported, well-known errors for implementation-specific conditions that callers might need to handle.

```go
var (
    // ErrClusterLocked indicates the cluster is in a frozen state.
    ErrClusterLocked = errors.New("cluster is locked")
    
    // ErrNoLeader indicates the Raft cluster has lost quorum.
    ErrNoLeader      = errors.New("no leader available")
)
```

**Usage:**

```go
if errors.Is(err, ErrClusterLocked) {
    // Handle lock specifically
}
```

## 5. Panics

!!! danger "No Panics Allowed"
    **NEVER** panic in controllers or internal logic packages.

    A panic in a single reconciler thread can crash the entire operator process, taking down management for ALL clusters.

=== ":material-check: Good Pattern"
    ```go
    if config == nil {
        return fmt.Errorf("internal error: config is nil")
    }
    ```

=== ":material-close: Bad Pattern"
    ```go
    if config == nil {
        panic("config is nil") // CRASHES THE OPERATOR
    }
    ```
