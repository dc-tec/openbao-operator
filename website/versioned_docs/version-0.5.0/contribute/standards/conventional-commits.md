---
title: Conventional Commits
description: Conventional commit rules for OpenBao Operator, including the required format, supported types and scopes, breaking-change signaling, and CI enforcement.
pageType: reference
journey: contribute
---

<PageHeader
  title="Use Conventional Commits so history, changelog generation, and release tooling all agree on what changed."
  lede="The repository uses Conventional Commits to keep PR titles and commit history predictable. This matters both for human review and for the release automation that turns merged changes into versioned output."
/>

<CommandBlock
  language="text"
  label="configure"
  title="Commit format"
  code={`<type>(<scope>): <description>

[optional body]

[optional footer(s)]`}
>
  Use the same shape for PR titles because the project relies on squash merge and changelog automation.
</CommandBlock>

<DecisionTable
  title="Common commit types"
  kind="reference"
  columns={["Type", "Use it for", "Release effect"]}
  rows={[
    {cells: ["`feat`", "new user-visible or contributor-visible functionality", "minor"]},
    {cells: ["`fix`", "bug fixes and behavior corrections", "patch"], emphasis: "recommended"},
    {cells: ["`docs`", "documentation-only changes", "patch"]},
    {cells: ["`refactor`", "code changes that do not add a feature or fix a bug", "patch"]},
    {cells: ["`test`", "test additions or corrections", "patch"]},
    {cells: ["`build`, `ci`, `chore`", "build-system, workflow, or maintenance changes", "patch"]},
    {cells: ["`revert`", "reversal of a previous change", "patch"]},
  ]}
/>

<DecisionTable
  title="Common scopes in this repository"
  kind="reference"
  columns={["Scope", "Area it maps to"]}
  rows={[
    {cells: ["`api`, `controller`, `infra`, `config`, `security`, `rbac`", "core controller and platform areas"]},
    {cells: ["`backup`, `restore`, `upgrade`, `bluegreen`", "manager and lifecycle features"]},
    {cells: ["`charts`, `manifests`, `deps`, `build`, `ci`, `docs`, `ai`", "tooling, artifacts, workflows, and docs"]},
    {cells: ["`test(unit)`, `test(integration)`, `test(e2e)`", "test-only changes with explicit layer scoping"]},
  ]}
/>

<Callout type="note" title="Breaking changes">

Mark a breaking change with `!` after the type or scope, or use a `BREAKING CHANGE:` footer when the description needs more room.

</Callout>

## CI enforcement

The repository validates Conventional Commit format in CI for:

- PR titles, which are required for the squash-merge workflow
- commit-subject checks, which remain informational rather than blocking

<NextActions
  title="Related contributor pages"
  items={[
    {
      label: "Release management",
      description: "Open the release flow when the next question is how commit semantics feed the published release process.",
      to: "/contribute/release-management",
    },
    {
      label: "Distribution",
      description: "Use the distribution page for how release outputs are published after the change is merged.",
      to: "/contribute/distribution",
    },
    {
      label: "Coding standards",
      description: "Return to the build-and-change landing page for implementation rules beyond commit formatting.",
      to: "/contribute/standards",
    },
  ]}
/>
