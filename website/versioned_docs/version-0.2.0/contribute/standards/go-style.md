---
title: Go Style Guide
description: Go coding style for OpenBao Operator, covering naming, error wrapping, structured logging, reconciler concurrency, imports, and constants.
pageType: concept
journey: contribute
---

<PageHeader
  title="Repository Go style defaults"
  lede="Naming, error handling, logging, reconciler-safe concurrency, imports, and constants used across the repository."
/>

<DecisionTable
  title="Default coding choices"
  columns={["Area", "Expected default", "Avoid"]}
  rows={[
    {
      cells: [
        "Naming",
        "Keep acronyms consistently cased and use direct Go-style getter names.",
        "Mixed-case acronym spellings such as `ServeHttp` or getters such as `GetStatus()`.",
      ],
      emphasis: "recommended",
    },
    {
      cells: [
        "Errors",
        "Wrap returned errors with `%w` so callers keep context and error identity.",
        "Returning opaque upstream errors with no explanation.",
      ],
    },
    {
      cells: [
        "Logging",
        "Use structured `logr` key-value logging.",
        "`fmt.Printf`, `log.Println`, or string-built log lines that lose context.",
      ],
    },
    {
      cells: [
        "Reconcilers",
        "Keep work synchronous and requeue explicitly when you need another pass.",
        "Unmanaged goroutines or `time.Sleep` in reconcile code.",
      ],
      emphasis: "caution",
    },
    {
      cells: [
        "Imports and constants",
        "Use stable import grouping and name magic values explicitly.",
        "Ad hoc import ordering and raw literals with unclear meaning.",
      ],
    },
  ]}
/>

## Naming

- Keep acronyms consistently cased: `ServeHTTP`, `ParseURL`, `userID`.
- Do not prefix getters with `Get`.
- Prefer `-er` names for single-method interfaces when the interface describes capability.

## Errors and logging

Use wrapped errors and structured logging together. Most reviewer comments in this area come from missing context, not from complex logic.

```go
if err := syncSecret(ctx, obj); err != nil {
    return fmt.Errorf("sync secret for %s/%s: %w", obj.Namespace, obj.Name, err)
}

log.Info("reconciling cluster",
    "cluster_namespace", req.Namespace,
    "cluster_name", req.Name,
)
```

<Callout type="danger" title="Never log secrets">

Do not log tokens, keys, passwords, or raw Secret data, even in debug-only paths.

</Callout>

## Reconcile-safe concurrency

- Do not spawn unmanaged goroutines in reconcile code.
- Do not block workers with `time.Sleep`.
- Use `RequeueAfter` when the controller should check again later.

## Imports and constants

Group imports into three blocks:

1. standard library
2. third-party dependencies
3. local `github.com/dc-tec/openbao-operator/...` packages

Name important values explicitly instead of leaving raw numbers or suffix strings in logic.

<NextActions
  title="Related standards"
  items={[
    {
      label: "Error handling",
      description: "Open the error-specific rules when the question is how to preserve context, classify failure, or avoid panics.",
      to: "/contribute/standards/error-handling",
    },
    {
      label: "Kubernetes operator patterns",
      description: "Use the controller-specific guidance when the change lives in reconciliation, status, or app-layer delegation.",
      to: "/contribute/standards/kubernetes-patterns",
    },
    {
      label: "Project conventions",
      description: "Go back to the repository-level rules when the problem is scope, boundaries, or generated output.",
      to: "/contribute/standards/project-conventions",
    },
  ]}
/>
