---
title: Continuous Integration
description: CI pipeline overview for OpenBao Operator, including local parity, change routing, E2E expansion, release hardening, and maintainer-only workflow lanes.
pageType: concept
journey: contribute
---

<PageHeader
  title="Know what CI will enforce before you ask it to validate your branch."
  lede="CI is optimized for signal, not ceremony. Pull requests route work based on changed files and labels, while `main`, edge, nightly, and release workflows enforce the heavier publication and hardening gates. Run the closest local equivalent first so CI is confirming your work, not discovering it for you."
/>

<DiagramFrame
  title="CI and publish flow"
  caption="Pull requests are change-routed; publish channels add provenance and reproducibility gates before anything is promoted."
  code={`graph TD
    PR["Pull request"] --> Detect["Detect changes"]
    Main["Push to main"] --> Detect
    Detect --> Core["Core quality gates"]
    Detect --> E2E["E2E routing"]
    Core --> Result["CI result"]
    E2E --> Result
    Result --> Edge["Edge publish"]
    Nightly["Nightly schedule"] --> NightlyRun["Nightly validation"]
    NightlyRun --> NightlyPublish["Nightly publish"]
    Tag["SemVer tag"] --> Release["Release workflow"]
    Edge --> Harden["Hardening gates"]
    NightlyPublish --> Harden
    Release --> Harden
    Harden --> Publish["Promote and publish artifacts"]`}
/>

<CommandBlock
  language="bash"
  label="verify"
  title="PR-equivalent local gate"
  code={`make bootstrap
make doctor
make ci-core`}
>
  Treat this as the default local bar. If this is red, CI is not the right place to learn that first.
</CommandBlock>

<DecisionTable
  title="Map CI lanes to local commands"
  columns={["CI concern", "Run locally", "Notes"]}
  rows={[
    {
      cells: [
        "Core PR validation",
        "`make ci-core`",
        "Covers lint, formatting, tidy, vendor, generated artifacts, tests, docs, Helm, security, fuzz smoke, and config compatibility.",
      ],
      emphasis: "recommended",
    },
    {
      cells: [
        "Docs-only changes",
        "`make docs-build`",
        "Use this when the change is isolated to documentation, routing, or site behavior.",
      ],
    },
    {
      cells: [
        "Dependency and license policy",
        "`make verify-vendor` and `make license-check`",
        "The repository enforces vendored dependency resolution and shipped-license allowlists through its local and CI artifact checks.",
      ],
    },
    {
      cells: [
        "Static security and filesystem scans",
        "`make security-ci` and `make security-scan-built-images`",
        "Run these when dependencies, network-facing code, or container-facing surfaces changed. `make security-ci` now includes vulncheck, license policy, Semgrep, and the Trivy filesystem scan.",
      ],
    },
    {
      cells: [
        "Focused E2E and platform validation",
        "`make test-e2e-ci ...`, `make helm-e2e-smoke`, or `make test-e2e-existing ...`",
        "Use label filters or the existing-cluster path when you need a smaller or platform-specific reproduction.",
      ],
    },
  ]}
/>

<Callout type="note" title="What CI assumes">

CI and release workflows enforce vendored Go dependencies. After dependency changes, rerun `make verify-vendor`. License verification uses that same vendored view of the dependency graph.

</Callout>

<DecisionTable
  title="How routing expands"
  columns={["Situation", "What usually happens", "Why it matters"]}
  rows={[
    {
      cells: [
        "Docs-only or workflow-only pull request",
        "CI skips broad E2E by default and focuses on the relevant smaller lanes.",
        "You avoid paying for cluster work that cannot fail for the files you touched.",
      ],
    },
    {
      cells: [
        "PR touches backup, upgrades, security, provisioner, admission, or controller-critical code",
        "Targeted E2E shards expand automatically.",
        "Coverage follows the risk of the changed surface.",
      ],
      emphasis: "recommended",
    },
    {
      cells: [
        "Maintainer adds `ci:full-e2e`",
        "The broader E2E suite runs.",
        "Use this when the change is wide enough that targeted routing is no longer sufficient.",
      ],
    },
    {
      cells: [
        "Nightly, edge, or tagged release flow",
        "Publish channels add provenance, reproducibility, and release-oriented hardening gates.",
        "Passing PR CI alone is not enough to publish artifacts.",
      ],
      emphasis: "caution",
    },
  ]}
/>

<CommandBlock
  language="bash"
  label="inspect"
  title="Typical focused E2E reproductions"
  code={`make test-e2e-ci \\
  KIND_NODE_IMAGE=kindest/node:v1.34.3 \\
  E2E_LABEL_FILTER='(((lifecycle && !tls) || manager) && !openshift)' \\
  E2E_PARALLEL_NODES=1

make helm-e2e-smoke

make fuzz
FUZZ_TARGET_FILTER='FuzzDiscoverConfig|internal/service/upgrade' make fuzz`}
>
  Use the smallest reproduction that still matches the CI lane you are trying to explain.
</CommandBlock>

<NextActions
  title="After CI parity"
  items={[
    {
      label: "Testing strategy",
      description: "Go back one level when you still need to choose the right validation depth before mapping it to a workflow.",
      to: "/contribute/testing",
    },
    {
      label: "Release management",
      description: "Move into release execution, stable-doc snapshot rules, and post-publish verification once the branch is release-ready.",
      to: "/contribute/release-management",
    },
    {
      label: "Dependency license policy",
      description: "Open the shipped-license rules when the failing gate is about allowlists rather than code or workflow behavior.",
      to: "/contribute/dependency-licenses",
    },
  ]}
/>
