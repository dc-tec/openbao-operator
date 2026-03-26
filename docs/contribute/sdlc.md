---
title: Software Development Lifecycle
description: How OpenBao Operator maps design, implementation, verification, release, and operations into one secure development lifecycle.
pageType: concept
journey: contribute
---

<PageHeader
  title="Use the SDLC to understand which controls prove a change is safe to ship."
  lede="The SDLC is the maintainer model for how work moves from design to production. It ties normal implementation work to the governance controls that harden builds, gate releases, and feed operational learning back into the next change."
/>

<DiagramFrame
  title="Lifecycle model"
  caption="The lifecycle is a control loop, not a one-way checklist."
  code={`graph LR
    Plan["Plan and design"]
    Build["Implement"]
    Verify["Verify"]
    Release["Release and publish"]
    Operate["Operate and learn"]

    Plan --> Build
    Build --> Verify
    Verify --> Release
    Release --> Operate
    Operate --> Plan`}
/>

<DecisionTable
  title="Lifecycle stages and the question each one answers"
  columns={["Stage", "Primary question", "Typical evidence", "Related guides"]}
  rows={[
    {
      cells: [
        "Plan and design",
        "Should this change exist, and what constraints must it respect?",
        "Architecture notes, compatibility expectations, security model updates, and scoped implementation plans.",
        "[Architecture](/docs/architecture), [Security](/docs/security), [Reference](/docs/reference)",
      ],
      emphasis: "recommended",
    },
    {
      cells: [
        "Implement",
        "Is the change expressed in a way the codebase can maintain safely?",
        "Conforming code, generated artifacts, and reviewable diffs that follow project standards.",
        "[Coding standards](/contribute/standards), [Set up your environment](/contribute/getting-started/development)",
      ],
    },
    {
      cells: [
        "Verify",
        "What is the smallest proof that the change behaves correctly and safely?",
        "Unit, integration, E2E, security, and reproducibility signals matched to the scope of the change.",
        "[Testing strategy](/contribute/testing), [Continuous integration](/contribute/ci)",
      ],
    },
    {
      cells: [
        "Release and publish",
        "Are the artifacts reproducible, attributable, and ready to promote?",
        "Signed subjects, provenance evidence, reproducibility gates, and release metadata.",
        "[Release management](/contribute/release-management), [Distribution](/contribute/distribution), [Supply chain security](/contribute/supply-chain-security)",
      ],
      emphasis: "caution",
    },
    {
      cells: [
        "Operate and learn",
        "Did the shipped change behave as intended in real environments, and what should feed back into the next cycle?",
        "Operational feedback, recovery learnings, production checklists, and follow-up design or policy updates.",
        "[Operate](/docs/operate), [Recovery & Restore](/docs/recover), [Project conventions](/contribute/standards/project-conventions)",
      ],
    },
  ]}
/>

<Callout type="note" title="Governance model, not a task checklist">

This page explains how the project models change control. Use the workflow pages for concrete commands and release execution. Use this page when you need to understand why those checks exist and where they fit.

</Callout>

<DecisionTable
  title="Where the strongest controls live"
  columns={["Control family", "What it protects", "Main enforcement surface"]}
  rows={[
    {
      cells: [
        "Contribution standards",
        "Code quality, consistency, and maintainability before CI has to compensate for weak inputs.",
        "[Build & Change](/contribute/standards) plus normal review.",
      ],
    },
    {
      cells: [
        "Testing and CI",
        "Behavioral correctness and regression detection across code, controllers, and cluster flows.",
        "[Testing strategy](/contribute/testing) and [Continuous integration](/contribute/ci).",
      ],
    },
    {
      cells: [
        "Supply-chain hardening",
        "Artifact trust, provenance, reproducibility, and signed release outputs.",
        "[Supply chain security](/contribute/supply-chain-security) and [Release management](/contribute/release-management).",
      ],
      emphasis: "recommended",
    },
    {
      cells: [
        "Operational feedback",
        "Learning from production operation, upgrades, backup and restore, and failure handling.",
        "The operator docs themselves, especially [Operate](/docs/operate) and [Security](/docs/security).",
      ],
    },
  ]}
/>

<NextActions
  title="Follow the lifecycle into concrete work"
  items={[
    {
      label: "Supply chain security",
      description: "See the hardening controls that govern provenance, reproducibility, and release trust.",
      to: "/contribute/supply-chain-security",
    },
    {
      label: "Continuous integration",
      description: "Open the workflow view of how verification is routed and enforced on branches, tags, and release lanes.",
      to: "/contribute/ci",
    },
    {
      label: "Coding standards",
      description: "Go back to implementation rules when the next question is how to make a change fit the repository correctly.",
      to: "/contribute/standards",
    },
  ]}
/>
