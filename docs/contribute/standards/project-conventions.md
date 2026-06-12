---
title: Project Conventions
description: Repository-specific implementation conventions for OpenBao Operator, including type safety, review scope, architecture boundaries, observability, and testing expectations.
pageType: concept
journey: contribute
---

<PageHeader
  title="Repository project conventions"
  lede="These are the project-specific rules that go beyond generic Go style. They keep controller code predictable, reviews focused, architecture boundaries enforceable, and generated output aligned with the code that owns it."
/>

<DecisionTable
  title="Core project conventions"
  columns={["Convention", "What it means in practice", "Why it matters"]}
  rows={[
    {
      cells: [
        "Prefer strict types",
        "Avoid `any` or `interface{}` in core logic except where an external library leaves no alternative.",
        "Compile-time guarantees beat runtime assertions in controller code.",
      ],
      emphasis: "recommended",
    },
    {
      cells: [
        "Delay abstraction until the rule of three",
        "Do not create shared helpers the first or second time a pattern appears.",
        "This keeps the codebase from filling with speculative utility layers.",
      ],
    },
    {
      cells: [
        "Keep PRs single-theme",
        "A change should primarily be a feature, bug fix, refactor, or doc update, not several unrelated cleanups at once.",
        "Smaller review surfaces keep maintainers effective.",
      ],
    },
    {
      cells: [
        "Keep architecture boundaries aligned with policy",
        "Update the boundary policy before adding new top-level internal packages, controller packages, or service-level service or adapter dependencies.",
        "The repository enforces these rules automatically, so design intent and CI stay aligned.",
      ],
    },
    {
      cells: [
        "Always update the generated or test artifacts a change implies",
        "If APIs, manifests, golden files, or rules change, regenerate them in the same PR.",
        "Generated drift should never be left for the next contributor to discover.",
      ],
    },
  ]}
/>

<Callout type="warning" title="Avoid type erasure in core logic">

Reserve `any` and `interface{}` for external API boundaries. Helpers that only work because they erase types usually need a narrower contract.

</Callout>

## Type safety and package shape

- Use defined types for enum-like status and phase values instead of raw strings.
- Avoid junk-drawer package names such as `util`, `common`, or `shared`.
- Prefer package names that describe the actual boundary or job, such as `k8sutil`, `config`, or `schema`.
- Reuse shared platform contracts such as `internal/platform/resourceidentity`, `internal/platform/resourceapply`, and `internal/platform/resourceownership` instead of copying names, labels, selectors, provenance checks, or generic apply flow into another service.
- Keep `config.hcl` semantics behind `internal/service/configuration` instead of letting workload bootstrap and upgrade flows render around each other.

## Review hygiene

- Keep a PR to one main theme.
- Avoid drive-by reformatting of unrelated files.
- If a change touches `api/`, update the generated artifacts in the same branch.
- If a service needs a new adapter import, update `serviceBoundaries` and regenerate the ast-grep rules in the same branch.

<CommandBlock
  language="bash"
  label="verify"
  title="Boundary policy regeneration and verification"
  code={`make generate-ast-rules
make verify-arch-policy
make test-ast
make lint-ast`}
>
  Run this when you add a new top-level `internal/*` package, introduce a new controller package, or change the boundary policy itself.
</CommandBlock>

<DecisionTable
  title="Observability and testing expectations"
  columns={["Area", "Repository expectation", "Primary entry point"]}
  rows={[
    {
      cells: [
        "Metrics",
        "Use the `openbao_` prefix and stable label sets.",
        "The metrics conventions in the existing controllers plus review.",
      ],
    },
    {
      cells: [
        "Logging",
        "Prefer structured, context-rich logging with consistent field names.",
        "[Kubernetes operator patterns](/contribute/standards/kubernetes-patterns).",
      ],
    },
    {
      cells: [
        "Testing",
        "Match the verification depth to the change, and update golden files when renderer output changes.",
        "[Testing strategy](/contribute/testing).",
      ],
      emphasis: "recommended",
    },
    {
      cells: [
        "CRD evolution",
        "Prefer additive API changes and document migrations when a breaking shape change is unavoidable.",
        "API review plus [Reference](/docs/reference).",
      ],
    },
  ]}
/>

<NextActions
  title="Related build-and-change guides"
  items={[
    {
      label: "Go style guide",
      description: "Open the lower-level coding rules for naming, imports, logging, and reconcilers.",
      to: "/contribute/standards/go-style",
    },
    {
      label: "Generated artifacts",
      description: "Generated artifacts maps emitted files back to their owning commands.",
      to: "/contribute/standards/generated-artifacts",
    },
    {
      label: "Kubernetes operator patterns",
      description: "Review the controller-specific patterns that keep reconciliation logic idempotent and boundary-aware.",
      to: "/contribute/standards/kubernetes-patterns",
    },
  ]}
/>
