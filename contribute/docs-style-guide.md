# OpenBao Operator Documentation Style Guide

This guide defines the writing and page-design standards for OpenBao Operator docs.

The docs are written for platform engineers, SREs, and operators running OpenBao on Kubernetes. The tone should feel calm, exact, and operational.

## Voice

- Write for someone doing real work on a cluster.
- Prefer direct language over brand or design commentary.
- Use the imperative for instructions: `Run`, `Apply`, `Verify`, `Check`.
- Keep explanations short enough to scan under time pressure.
- Be slightly opinionated when the project has a recommended path.

## Terms

Use product and platform terms consistently:

- `OpenBao Operator`
- `OpenBao`
- `Kubernetes` or `K8s`
- `OpenBaoCluster`, `OpenBaoRestore`, `OpenBaoTenant`
- `Hardened` and `Development` as profile names
- `single-tenant` and `multi-tenant`

Do not invent synonyms for core product concepts when the API or docs already define them.

## What To Avoid

- Do not explain the docs system inside the docs.
- Do not write design commentary such as "the first job of the docs is routing".
- Do not overuse abstract terms like `journey`, `path`, or `operating model` when `install`, `upgrade`, `restore`, or `troubleshoot` is clearer.
- Do not use slogan-style copy on normal docs pages.
- Do not rely on `not X, but Y` framing unless it clarifies a real technical distinction.

Exception:

- Landing pages may use one stronger product line when it helps establish the section quickly. The homepage slogan is an intentional exception, not the default writing style for the rest of the docs.

## Headings And Intros

- Headings should tell the reader what they can do or learn on the page.
- The first paragraph should explain scope, not philosophy.
- Keep intros to one short paragraph on most pages.
- Landing pages may use two short paragraphs when they need both orientation and a starting recommendation.

Prefer:

- `Install the operator`
- `Recover from a sealed cluster`
- `Choose a deployment model`
- `Review compatibility and support posture`

Avoid:

- `Why this manual exists`
- `The operator changes the shape of the problem`
- `Keep supporting material close, but quieter than the main routes`

## Page Types

Use the Docusaurus frontmatter fields that support the docs system:

```yaml
---
description: One sentence summary of the page intent.
pageType: landing | task | concept | runbook | reference
journey: get-started | operate | security | architecture | reference | contribute
journeyStep: 1
---
```

Rules:

- `pageType` should be present on all new landing pages and high-traffic docs.
- `journey` is optional, but use it when the page belongs to a named docs section.
- `journeyStep` is only for guided flows such as `Get Started`.
- Recovery pages live inside the `operate` journey rather than as a separate top-level journey.

## Page Structure

### Landing Pages

Use landing pages to orient and route.

Expected structure:

1. `PageHero`
2. `RouteList` or `JourneyRail`
3. `NextActions`

Landing pages should answer:

- what this section is for
- who it is for
- where to start
- what success looks like

### Task Pages

Use task pages for setup and operations work.

Expected structure:

1. why or when to use the procedure
2. prerequisites
3. steps
4. verification
5. next actions

### Concept Pages

Use concept pages to explain behavior, tradeoffs, and internal reasoning.

Expected structure:

1. what the component or concept is
2. responsibilities or invariants
3. comparison or implications
4. related tasks

### Runbooks

Use runbooks for incident response.

Expected structure:

1. trigger or symptoms
2. diagnosis
3. recovery steps
4. recovery checks
5. escalation or follow-up

### Reference Pages

Use reference pages for exact values, compatibility, policy, and status semantics.

Expected structure:

1. short scope statement
2. dense tables or lists
3. minimal decorative copy

## Approved Components

Prefer the shared Docusaurus/MDX components in `website/src/components`.

Use:

- `PageHero` for landing pages and major entry pages
- `RouteList` for section routing
- `JourneyRail` for guided flows
- `DecisionTable` for comparisons and compatibility matrices
- `CommandBlock` for commands, manifests, and expected output
- `DiagramFrame` for Mermaid diagrams
- `NextActions` for page handoff
- `Callout` for warnings, context, and operational notes
- `Tabs` for alternative install or configuration methods

Avoid:

- using card grids as the default navigation pattern
- building custom one-off layout wrappers when an existing component already fits
- using callouts as the primary page structure

## Callouts

Use callouts sparingly and match the severity to the content.

Recommended types:

- `note` for context
- `tip` for shortcuts or best practices
- `info` for recommendations
- `warning` for recoverable risks
- `danger` for security impact, destructive actions, or irreversible data loss
- `success` only when the user needs a clear completion checkpoint

Example:

```mdx
<Callout type="warning" title="CRD updates">

Apply CRD changes before upgrading the Helm release.

</Callout>
```

## Tables

Use `DecisionTable` for comparison-heavy content.

Rules:

- Keep column labels short.
- Use `kind="decision"` for tradeoffs and recommendations.
- Use `kind="reference"` for exact support matrices and policy tables.
- Mark the default or recommended row when there is a clear project recommendation.
- Do not bury the recommendation in paragraph text if the table is the decision surface.

## Code Blocks

Use `CommandBlock` when the distinction between action, inspection, verification, and output matters.

Labels:

- `apply`
- `configure`
- `inspect`
- `verify`
- `output`

Rules:

- Shell commands should say what the reader is doing.
- Output blocks should be visually distinct from commands.
- Keep examples minimal and production-safe.
- Prefer one good example over several slight variations.

## Diagrams

Wrap Mermaid diagrams in `DiagramFrame`.

Rules:

- Every diagram should explain a real workflow, trust boundary, or component relationship.
- Add a short caption when the diagram appears on an architecture or security page.
- Use the shared theme-safe palette rather than ad hoc color choices.
- Do not include decorative diagrams.

## Navigation Copy

Route labels should be task-led whenever possible.

Prefer:

- `Install the operator`
- `Plan upgrades`
- `Recover from no leader`
- `Open compatibility`

Use section ownership consistently:

- `Get Started` gets someone from initial choice to a day 2 baseline.
- `Operate` owns routine operations, troubleshooting, recovery, and restore.
- `Security` explains trust boundaries and production controls.
- `Architecture` explains why the system behaves the way it does.
- `Reference` stores exact facts, policies, and schemas.
- `Contribute` is for maintainers and contributors, not operators.

Avoid:

- `Learn more`
- `Explore`
- `Continue journey`
- `Read this section`

## Landing Page Copy

Homepage and section landing pages should:

- establish scope quickly
- route decisively
- avoid explaining the information architecture itself

One strong product line is fine. Everything after that should return to utility language.

## Previewing Docs

```sh
make docs-serve
make docs-build
```

Website checks:

```sh
npm --prefix website run typecheck
npm --prefix website run build
npm --prefix website run test:e2e
```
