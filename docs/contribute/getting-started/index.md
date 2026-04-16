---
title: Getting Started with Contributing
description: First-time contributor path for OpenBao Operator, including workstation setup, first local checks, and where to go after the environment is ready.
pageType: landing
journey: contribute
---

<PageHero
  eyebrow="Contribute / Start Here"
  title="Contributor setup"
  lede="Prepare a workstation, create a first branch, and run the initial local checks."
  actions={[
    {label: "Set up your environment", to: "/contribute/getting-started/development", variant: "primary"},
    {label: "Open testing strategy", to: "/contribute/testing", variant: "secondary"},
  ]}
>
  <Checklist
    title="Use this page to"
    items={[
      "prepare the required local tools for repo-managed commands",
      "understand the first commands maintainers expect you to run on a clean clone",
      "see the basic contribution path before diving into standards or CI details",
      "find the right next page once the environment itself is working",
    ]}
  />
</PageHero>

<CommandBlock
  language="bash"
  label="verify"
  title="PR-equivalent local gate"
  code={`make bootstrap
make doctor
make ci-core`}
>
  Use this as the baseline local proof for a new branch or workstation setup.
</CommandBlock>

<RouteList
  title="Core path"
  items={[
    {
      eyebrow: "01",
      title: "Set up your environment",
      description: "Install the required tools, use the repo-managed bootstrap flow, and verify that your local environment is actually healthy.",
      to: "/contribute/getting-started/development",
    },
    {
      eyebrow: "02",
      title: "Learn the coding rules",
      description: "Read the project conventions, code standards, and generated-artifact rules before reshaping code or manifests.",
      to: "/contribute/standards",
    },
    {
      eyebrow: "03",
      title: "Understand the testing stack",
      description: "Know which layers to run locally and what CI is going to enforce for the branch.",
      to: "/contribute/testing",
    },
    {
      eyebrow: "04",
      title: "Open a clean branch",
      description: "Use the commit and change-management rules the project expects once your local setup is stable.",
      to: "/contribute/standards/conventional-commits",
    },
  ]}
/>

<DecisionTable
  title="Contribution lanes"
  columns={["Type", "What it usually changes", "Read this next"]}
  rows={[
    {
      cells: [
        "Code changes",
        "Controller logic, APIs, adapters, reconciliation behavior, tests, or generated assets.",
        "Open the coding standards and testing strategy.",
      ],
      emphasis: "recommended",
    },
    {
      cells: [
        "Documentation changes",
        "User guides, contributor docs, IA work, examples, or style fixes.",
        "Open the documentation style guide and contributor standards.",
      ],
    },
    {
      cells: [
        "Maintainer work",
        "Release flow, CI changes, distribution policy, or supply-chain controls.",
        "Open Validate & Ship and Project Governance.",
      ],
    },
  ]}
/>

<NextActions
  title="After setup"
  items={[
    {
      label: "Set up your environment",
      description: "Use the detailed setup page for tools, versions, and local cluster guidance.",
      to: "/contribute/getting-started/development",
    },
    {
      label: "Coding standards",
      description: "Read the project rules for code and docs changes.",
      to: "/contribute/standards",
    },
    {
      label: "Testing strategy",
      description: "Map your change to the right unit, integration, E2E, or manual validation layer.",
      to: "/contribute/testing",
    },
  ]}
/>
