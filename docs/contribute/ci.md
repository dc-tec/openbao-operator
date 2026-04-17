---
title: Continuous Integration
description: CI pipeline overview for OpenBao Operator, including local parity, change routing, E2E expansion, release hardening, and maintainer-only workflow lanes.
pageType: concept
journey: contribute
---

<PageHeader
  title="CI workflow and local parity"
  lede="Pull request, `main`, nightly, and release workflows, plus the closest local command set for each validation lane."
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
  Default local baseline before handing work to CI.
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
        "For changes isolated to documentation, routing, or site behavior.",
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
        "`make semgrep-ci`, `make security-ci`, and `make security-scan-built-images`",
        "Run these when dependencies, network-facing code, container-facing surfaces, or CI/config security surfaces changed. `make semgrep-ci` is the blocking local Semgrep scan across `cmd`, `internal`, `api`, `hack`, `config`, and `.github`; `make security-ci` includes that scan plus vulncheck, license policy, and the Trivy filesystem scan.",
      ],
    },
    {
      cells: [
        "Focused E2E and platform validation",
        "`make test-e2e-ci ...`, `make helm-e2e-smoke`, or `make test-e2e-existing ...`",
        "Label filters and the existing-cluster path provide smaller or platform-specific reproductions.",
      ],
    },
  ]}
/>

<Callout type="note" title="What CI assumes">

CI and release workflows enforce vendored Go dependencies. After dependency changes, rerun `make verify-vendor`. License verification uses that same vendored view of the dependency graph.

Inline `nosemgrep` suppressions are reserved for bounded intentional exceptions. Put the suppression on the exact reported line and add a plain-language explanation immediately above it.

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
        "This is appropriate when the change spans enough surface area that targeted routing is no longer sufficient.",
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
  Choose the smallest reproduction that still matches the CI lane under investigation.
</CommandBlock>

<NextActions
  title="After CI parity"
  items={[
    {
      label: "Testing strategy",
      description: "Testing strategy covers validation depth before it is mapped to a workflow.",
      to: "/contribute/testing",
    },
    {
      label: "Release management",
      description: "Release management covers release execution, docs snapshots, and post-publish verification.",
      to: "/contribute/release-management",
    },
    {
      label: "Dependency license policy",
      description: "Dependency license policy covers shipped-license allowlists and their enforcement.",
      to: "/contribute/dependency-licenses",
    },
  ]}
/>
