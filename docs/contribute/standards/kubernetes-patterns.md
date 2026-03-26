---
title: Kubernetes Operator Patterns
description: Kubernetes-native controller patterns for OpenBao Operator, including level-triggered reconciliation, idempotency, app-layer delegation, context handling, and structured logging.
pageType: concept
journey: contribute
---

<PageHeader
  title="Use these patterns to keep controller code Kubernetes-native instead of drifting into ad hoc orchestration."
  lede="The operator relies on predictable reconcile loops, explicit controller boundaries, and safe interaction with the API server. These patterns are the default shape of controller work in this repository, especially when app-layer orchestration and manager boundaries are involved."
/>

<DiagramFrame
  title="Level-triggered reconciliation"
  caption="Controllers reconcile the current world against desired state. They do not assume a single event arrives only once."
  code={`graph TD
    Trigger["Event or scheduled requeue"] --> Fetch["Fetch current custom resource"]
    Fetch --> Exists{"Object exists?"}
    Exists -- No --> Stop["Stop or finalize"]
    Exists -- Yes --> Observe["Fetch child resources and status"]
    Observe --> Diff{"State drift?"}
    Diff -- No --> Status["Patch status if needed"]
    Diff -- Yes --> Act["Create, update, or delete resources"]
    Act --> Status
    Status --> End["Return and requeue if needed"]`}
/>

<DecisionTable
  title="Controller patterns to keep"
  columns={["Pattern", "What it means", "Why it matters"]}
  rows={[
    {
      cells: [
        "Level-triggered reconciliation",
        "Always reconcile from current observed state rather than assuming a single edge event is authoritative.",
        "Controllers may run repeatedly for the same change and still need to converge safely.",
      ],
      emphasis: "recommended",
    },
    {
      cells: [
        "Idempotent fetch-check-act flow",
        "Fetch existing state, compare with desired state, and act only when drift exists.",
        "Blind create or blind update paths become noisy and brittle on the second reconcile.",
      ],
    },
    {
      cells: [
        "Controller boundary stays thin",
        "Controllers should observe, delegate, patch status, and requeue, not encode deep orchestration directly.",
        "This keeps the app layer and manager boundaries testable and enforceable.",
      ],
    },
    {
      cells: [
        "No unmanaged concurrency",
        "Use the context and lifecycle controller-runtime already provides.",
        "Background goroutines lose error handling, ordering, and shutdown semantics.",
      ],
      emphasis: "caution",
    },
    {
      cells: [
        "Structured logging with context",
        "Attach stable keys such as cluster namespace, name, and phase to log entries.",
        "Logs stay queryable and preserve reconcile context across the workflow.",
      ],
    },
  ]}
/>

<Callout type="note" title="Boundary rules are policy-enforced">

Architecture boundaries are generated from `.ast-grep/policy/architecture-boundaries.yml` and enforced in CI. If a new controller or internal package boundary is required, change the policy first and regenerate the rules in the same branch.

</Callout>

```go
// Controller path: observe, delegate, then return result and error.
result, err := appopenbaocluster.ReconcileAdminOps(ctx, r.Client, req, log, deps)
if err != nil {
    return ctrl.Result{}, err
}
return result, nil
```

<NextActions
  title="Related implementation guides"
  items={[
    {
      label: "Architecture",
      description: "Use the architecture section when the next question is which manager or controller owns a responsibility.",
      to: "/docs/architecture",
    },
    {
      label: "Go style guide",
      description: "Return to the language-level rules for naming, errors, and imports.",
      to: "/contribute/standards/go-style",
    },
    {
      label: "Error handling",
      description: "Open the error-specific guidance when the reconcile path needs clearer failure contracts.",
      to: "/contribute/standards/error-handling",
    },
  ]}
/>
