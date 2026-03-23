---
title: Error Handling
description: Error-handling rules for OpenBao Operator, including wrapping, guard clauses, Kubernetes typed errors, checkable sentinels, and panic avoidance.
pageType: concept
journey: contribute
---

<PageHero
  variant="compact"
  eyebrow="Contribute / Build & Change"
  title="Handle errors so callers keep context, reviewers can follow the failure path, and controllers stay alive."
  lede="Most error-handling issues in this repository are not about catching more failures. They are about returning failures with enough structure and context that the next layer can make the right decision without guessing. These rules keep that path predictable."
  actions={[
    {label: "Open Go style guide", to: "/contribute/standards/go-style", variant: "primary"},
    {label: "Open Kubernetes patterns", to: "/contribute/standards/kubernetes-patterns", variant: "secondary"},
  ]}
>
  <Checklist
    title="Use this page when you need to"
    items={[
      "wrap and classify errors without losing the underlying cause",
      "use Kubernetes typed errors correctly around the API server",
      "define checkable sentinel errors for internal state transitions",
      "avoid panic paths in controllers and internal logic packages",
    ]}
  />
</PageHero>

<DecisionTable
  title="Default error-handling rules"
  columns={["Rule", "Expected default", "Avoid"]}
  rows={[
    {
      cells: [
        "Wrap returned errors",
        "Add local context and preserve the chain with `%w`.",
        "Returning an upstream error with no explanation of what failed here.",
      ],
      emphasis: "recommended",
    },
    {
      cells: [
        "Use guard clauses",
        "Return early on failure and keep the happy path unindented.",
        "Nesting success logic inside `if err == nil` branches.",
      ],
    },
    {
      cells: [
        "Check Kubernetes error types explicitly",
        "Handle `NotFound` and similar typed API failures with `apierrors` helpers.",
        "String-matching Kubernetes errors.",
      ],
    },
    {
      cells: [
        "Define checkable sentinels only for real control flow",
        "Expose well-known errors when callers truly need `errors.Is` behavior.",
        "Creating exported errors for every failure string in the codebase.",
      ],
    },
    {
      cells: [
        "Do not panic in controllers",
        "Return an error or surface an internal invariant failure explicitly.",
        "Panics that can crash operator management for every cluster.",
      ],
      emphasis: "caution",
    },
  ]}
/>

## Wrapping and context

```go
if err := r.Client.Create(ctx, secret); err != nil {
    return fmt.Errorf("create bootstrap secret %s/%s: %w", secret.Namespace, secret.Name, err)
}
```

## Kubernetes typed errors

```go
if err := r.Client.Get(ctx, key, secret); err != nil {
    if apierrors.IsNotFound(err) {
        return r.createSecret(ctx, ...)
    }

    return fmt.Errorf("get secret %s: %w", key.Name, err)
}
```

## Checkable internal errors

Use exported sentinel errors only when the caller genuinely needs to branch on the result:

```go
var (
    ErrClusterLocked = errors.New("cluster is locked")
    ErrNoLeader = errors.New("no leader available")
)
```

<Callout type="danger" title="No panics in controller or internal logic paths">

If a nil or impossible state appears, return a contextual error and let the caller decide how to surface it. A panic in a reconcile worker can take down management for the entire operator process.

</Callout>

<NextActions
  title="Related coding rules"
  items={[
    {
      label: "Go style guide",
      description: "Return to naming, imports, and logging rules that sit next to error handling in normal implementation work.",
      to: "/contribute/standards/go-style",
    },
    {
      label: "Security practices",
      description: "Open the secure-coding guidance when the error path also handles secrets, external input, or filesystem state.",
      to: "/contribute/standards/security-practices",
    },
    {
      label: "Testing strategy",
      description: "Use the testing guide when a failure path needs explicit unit, integration, or E2E coverage.",
      to: "/contribute/testing",
    },
  ]}
/>
