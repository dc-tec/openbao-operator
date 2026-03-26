---
title: Generated Artifacts
description: Generated-artifact rules for OpenBao Operator, including what generates CRDs, API docs, Helm sync output, golden files, and architecture policy rules.
pageType: reference
journey: contribute
---

<PageHeader
  title="Use this page when a change should have emitted files and you need to know which command owns them."
  lede="Generated output is part of the source tree contract in this repository. If an API, policy, renderer, or chart input changes, the matching generated artifacts must change in the same PR. Do not edit generated files directly."
/>

<Callout type="warning" title="Do not edit generated files manually">

Edit the source input and rerun the owning command. Manual edits to generated output will be overwritten and usually hide the real source of truth from the next reviewer.

</Callout>

<DecisionTable
  title="Quick reference"
  kind="reference"
  columns={["If you changed", "Regenerate with", "Main CI verifier"]}
  rows={[
    {
      cells: [
        "`api/v1alpha1/*.go`",
        "`make manifests generate`",
        "`verify-generated`",
      ],
      emphasis: "recommended",
    },
    {
      cells: [
        "`api/v1alpha1/*.go` comments or API shapes that affect docs",
        "`make api-reference`",
        "`verify-generated`",
      ],
    },
    {
      cells: [
        "CRD, admission, or RBAC inputs that feed the chart",
        "`make helm-sync`",
        "`verify-helm`",
      ],
    },
    {
      cells: [
        "`.ast-grep/policy/architecture-boundaries.yml`",
        "`make generate-ast-rules`",
        "`verify-arch-policy`",
      ],
    },
    {
      cells: [
        "`internal/adapter/config/*.go` renderer behavior",
        "`make test-update-golden`",
        "`test`",
      ],
    },
  ]}
/>

## Artifact ownership

- Kubernetes CRDs and `zz_generated.deepcopy.go` come from `api/v1alpha1/*.go`.
- API reference docs come from API types and comments.
- Helm chart sync output comes from CRD, policy, and RBAC source material.
- Golden HCL files come from renderer behavior in `internal/adapter/config/`.
- Ast-grep boundary rules come from `.ast-grep/policy/architecture-boundaries.yml`.

<CommandBlock
  language="bash"
  label="verify"
  title="Safe regeneration sweep"
  code={`make manifests generate
make api-reference
make helm-sync
make generate-ast-rules
make test-update-golden`}
>
  Use this when several change areas overlap or when CI tells you generated output drifted but the failing surface is not obvious yet.
</CommandBlock>

<NextActions
  title="After regeneration"
  items={[
    {
      label: "Testing strategy",
      description: "Run the right validation layer after generation so output drift does not hide a deeper logic problem.",
      to: "/contribute/testing",
    },
    {
      label: "Project conventions",
      description: "Return to the repository conventions when the question is scope, review hygiene, or architecture policy ownership.",
      to: "/contribute/standards/project-conventions",
    },
    {
      label: "Continuous integration",
      description: "Open the CI guide if you need to understand which workflow enforces a particular generator or verifier.",
      to: "/contribute/ci",
    },
  ]}
/>
