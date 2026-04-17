---
title: OpenBao Operator Documentation Style Guide
description: Editorial and page-design standards for OpenBao Operator documentation, including voice, page types, approved components, navigation language, and validation steps.
pageType: reference
journey: contribute
---

<PageHeader
  title="Documentation style guide"
  lede="The docs are written for platform engineers, SREs, operators, maintainers, and contributors doing real work. This guide defines the voice, page contracts, navigation language, and shared components that keep the documentation product coherent as it grows."
/>

<DecisionTable
  title="Page types"
  kind="reference"
  columns={["Page type", "Use it for", "Expected shape"]}
  rows={[
    {
      cells: [
        "`landing`",
        "Section entry and routing pages.",
        "`PageHero`, a routing surface such as `RouteList` or `JourneyRail`, then `NextActions`.",
      ],
      emphasis: "recommended",
    },
    {
      cells: [
        "`task`",
        "Installation, configuration, and procedural work.",
        "When to use it, prerequisites, steps, verification, then next actions.",
      ],
    },
    {
      cells: [
        "`concept`",
        "Behavior, invariants, tradeoffs, and internal reasoning.",
        "Scope, responsibilities, implications, and related tasks.",
      ],
    },
    {
      cells: [
        "`runbook`",
        "Incident diagnosis and recovery.",
        "Trigger, diagnosis, recovery steps, verification, and escalation.",
      ],
    },
    {
      cells: [
        "`reference`",
        "Exact support data, policies, matrices, and schemas.",
        "Short scope statement plus dense, precise content with minimal ornament.",
      ],
    },
  ]}
/>

## Voice

- Write for someone doing real work on a cluster or in the repository.
- Prefer direct operational language over design commentary.
- Use the imperative for instructions: `Run`, `Apply`, `Verify`, `Check`.
- Keep explanations short enough to scan under time pressure.
- Be lightly opinionated when the project has a recommended path.

## Wording and headings

- Keep the page title as needed for navigation, but use sentence case for headings below the page title.
- Start sections with the decision, scope, or instruction itself. Do not spend the first sentence explaining that the section exists.
- Prefer direct wording over evaluative filler. Replace words such as `strong`, `practical`, `useful`, `good`, and `mature` with the actual recommendation.
- Reduce repeated `This page...`, `This section...`, and `The goal is...` framing when the heading or surrounding structure already provides that context.
- Prefer direct verbs such as `Use`, `Keep`, `Limit`, `Reserve`, `Place`, and `Do not give` over repetitive `should` phrasing.
- Prefer operational wording over abstract noun clusters. Name the action, object, or control directly.
- Vary sentence length and cadence. Do not let every section open with the same balanced or three-part construction.
- Keep one main idea per paragraph when possible.

## Terms

Use product and platform terms consistently:

- `OpenBao Operator`
- `OpenBao`
- `Kubernetes` or `K8s`
- `OpenBaoCluster`, `OpenBaoRestore`, `OpenBaoTenant`
- `Hardened` and `Development` as profile names
- `single-tenant` and `multi-tenant`

## What to avoid

- Explaining the docs system inside the docs.
- Design commentary such as "the first job of the docs is routing".
- Abstract language when concrete verbs such as `install`, `upgrade`, `restore`, or `troubleshoot` are clearer.
- Slogan-heavy copy on normal pages.
- `not X, but Y` framing when it does not clarify a real technical distinction.
- Repeated evaluative framing such as "a strong baseline", "a practical model", "a useful default", or "a good starting point".
- Opening sections by restating the heading before making a point.
- Generic transition sentences that only restate the obvious benefit of the previous line.
- Stacked hedging such as `usually`, `often`, `generally`, `typically`, and `in most cases` when the docs are describing the baseline recommendation.
- Taxonomy or categorization that does not drive an actual operator or maintainer decision.

## Metadata and shared components

Use the Docusaurus frontmatter fields that support the docs system:

```yaml
---
description: One sentence summary of the page intent.
pageType: landing | task | concept | runbook | reference
journey: get-started | operate | security | architecture | reference | contribute
journeyStep: 1
---
```

Prefer the shared components in `website/src/components`:

- `PageHero`
- `RouteList`
- `JourneyRail`
- `DecisionTable`
- `CommandBlock`
- `DiagramFrame`
- `NextActions`
- `Callout`
- `Tabs`

## Navigation language

Route labels should be task-led whenever possible.

Prefer:

- `Install the operator`
- `Plan upgrades`
- `Recover from no leader`
- `Open compatibility`

Avoid:

- `Learn more`
- `Explore`
- `Continue journey`
- `Read this section`

Section ownership:

- `Get Started` takes a user from first choice to a day 2 baseline.
- `Operate` owns routine operations, troubleshooting, recovery, and restore.
- `Security` explains trust boundaries and production controls.
- `Architecture` explains why the system behaves the way it does.
- `Reference` stores exact facts, policies, and schemas.
- `Contribute` is for maintainers and contributors, not operators.

## Validation

<CommandBlock
  language="bash"
  label="verify"
  title="Docs preview and validation"
  code={`make docs-serve
make docs-preview
make docs-build

npm --prefix website run typecheck
npm --prefix website run build
npm --prefix website run test:e2e`}
>
  Use `make docs-serve` for the fast authoring loop. Use `make docs-preview` when you need production-style behavior such as local search. Run the full site checks before merging structural or high-traffic documentation changes.
</CommandBlock>

<NextActions
  title="Related contributor pages"
  items={[
    {
      label: "Contribute",
      description: "Return to the contributor landing page for the full maintainer and contributor map.",
      to: "/contribute",
    },
    {
      label: "Generated artifacts",
      description: "Use the generation guide when docs changes also affect emitted reference or chart output.",
      to: "/contribute/standards/generated-artifacts",
    },
    {
      label: "Validate & Ship",
      description: "Open the release and CI section when the documentation change also affects verification or publication flow.",
      to: "/contribute/validate-and-ship",
    },
  ]}
/>
