---
trigger: always_on
glob: "**/*.go"
description: Error handling patterns for the OpenBao Operator
---

# Error Handling

See [Error Handling](docs/contributing/standards/error-handling.md).

## Wrapping Errors

ALWAYS wrap errors with context:

```go
// Bad
return err

// Good
return fmt.Errorf("failed to create TLS secret: %w", err)
```

Use `%w` to preserve error chain for `errors.Is()` / `errors.As()`.

## Guard Clauses

Check errors immediately with guard clauses:

```go
// Bad
if err == nil {
    // do work
}

// Good
if err != nil {
    return fmt.Errorf("context: %w", err)
}
// do work
```

## Sentinel Errors

Define exported sentinels for errors callers need to check:

```go
var (
    ErrPodNotFound     = errors.New("pod not found")
    ErrClusterNotReady = errors.New("cluster not ready")
)
```

## Kubernetes Errors

```go
import apierrors "k8s.io/apimachinery/pkg/api/errors"

if apierrors.IsNotFound(err) {
    // Handle missing resource
}
```

## NEVER Panic

- No `panic()` in controllers or internal packages
- Panics crash the entire manager
- OK only in `init()` or `main()` for programmer errors
