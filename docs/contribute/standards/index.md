---
title: Coding Standards
description: Contributor standards for OpenBao Operator covering Go style, project conventions, Kubernetes patterns, error handling, generated artifacts, security practices, commit rules, and docs standards.
pageType: landing
journey: contribute
---

<PageHero
  eyebrow="Contribute / Build & Change"
  title="Repository standards"
  lede="Coding, documentation, generated-artifact, and commit-history standards for OpenBao Operator."
  actions={[
    {label: "Open project conventions", to: "/contribute/standards/project-conventions", variant: "primary"},
    {label: "Open documentation style guide", to: "/contribute/docs-style-guide", variant: "secondary"},
  ]}
>
  <Checklist
    title="Use this section to"
    items={[
      "understand what the project considers idiomatic Go and acceptable controller patterns",
      "avoid common review feedback around error handling, logging, generated files, and commit shape",
      "follow the project’s security, docs, and architecture expectations when you submit code",
      "know which standards are global and which are specific to OpenBao Operator itself",
    ]}
  />
</PageHero>

<RouteList
  title="Build & change guides"
  items={[
    {
      eyebrow: "01",
      title: "Project conventions",
      description: "OpenBao Operator-specific rules around type safety, metrics, logging, testing depth, and architectural boundaries.",
      to: "/contribute/standards/project-conventions",
    },
    {
      eyebrow: "02",
      title: "Go style guide",
      description: "Naming, formatting, package structure, and idiomatic Go patterns expected in the repo.",
      to: "/contribute/standards/go-style",
    },
    {
      eyebrow: "03",
      title: "Kubernetes operator patterns",
      description: "Reconcile-loop shape, controller behavior, and Kubernetes-native design expectations.",
      to: "/contribute/standards/kubernetes-patterns",
    },
    {
      eyebrow: "04",
      title: "Error handling",
      description: "Required error-wrapping, propagation, and debugging conventions for production-grade controller code.",
      to: "/contribute/standards/error-handling",
    },
    {
      eyebrow: "05",
      title: "Generated artifacts",
      description: "What must never be edited by hand and which commands regenerate the project outputs CI verifies.",
      to: "/contribute/standards/generated-artifacts",
    },
    {
      eyebrow: "06",
      title: "Security practices",
      description: "Contributor rules for secrets, file permissions, input handling, and secure-by-default changes.",
      to: "/contribute/standards/security-practices",
    },
    {
      eyebrow: "07",
      title: "Conventional commits",
      description: "Commit message format used for consistent history and release automation.",
      to: "/contribute/standards/conventional-commits",
    },
    {
      eyebrow: "08",
      title: "Documentation style guide",
      description: "Writing, page-type, and design-system rules for user-facing and contributor-facing docs.",
      to: "/contribute/docs-style-guide",
    },
  ]}
/>

<CommandBlock
  language="bash"
  label="verify"
  title="Baseline contributor gate"
  code={`make bootstrap
make doctor
make ci-core`}
>
  If your change affects generated artifacts, manifests, or docs, run the more specific regeneration commands before the baseline gate.
</CommandBlock>

<NextActions
  title="Pair standards with execution"
  items={[
    {
      label: "Testing strategy",
      description: "Match your change to the right test layer after you understand the code and docs rules.",
      to: "/contribute/testing",
    },
    {
      label: "Continuous integration",
      description: "See how the repo enforces these standards once your branch hits CI.",
      to: "/contribute/ci",
    },
    {
      label: "Set up your environment",
      description: "Go back to the local setup guide if you still need a clean contributor workstation.",
      to: "/contribute/getting-started/development",
    },
  ]}
/>
