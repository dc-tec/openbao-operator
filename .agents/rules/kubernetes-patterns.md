---
trigger: always_on
glob: "internal/controller/**/*.go"
description: Kubernetes operator patterns for the OpenBao Operator
---

# Kubernetes Operator Patterns

See [project standards](../../website/content/contribute/standards.md).

## Reconciler Rules

1. **Idempotency**: Reconcile may be called multiple times for same state
2. **Check Before Acting**: Always check if resource exists before creating
3. **Deterministic Names**: Resource names from parent CR, not random
4. **Context**: Pass `ctx` as first param, respect cancellation
5. **No Goroutines**: Controller-runtime handles concurrency
6. **No Shared State**: Don't store request state in Reconciler struct

## Spec vs Status

- **Spec**: Read-only desired state (user input)
- **Status**: Observed state; in controller `Reconcile`, delegate status persistence to focused helpers/facades (avoid direct `r.Status().Update/Patch/Apply` there)

## Controller Boundary

- Keep controllers as observe/delegate/patch-requeue plumbing
- Delegate orchestration/domain behavior to `internal/app/*`
- Avoid direct domain package orchestration in controller `Reconcile`

## Logging

Use structured logging with standard fields:

```go
log.Info("Reconciling",
    "cluster_namespace", cluster.Namespace,
    "cluster_name", cluster.Name,
)
```

NEVER log secrets, tokens, or keys.

## Rate Limiting

- Do NOT use `time.Sleep()` in reconcilers
- Return `ctrl.Result{RequeueAfter: duration}` instead
- Use controller-runtime's built-in rate limiting

## Status Conditions

Use typed conditions from `api/v1alpha1/`:

- `Available`, `Degraded`, `TLSReady`, `ProductionReady`, etc.
