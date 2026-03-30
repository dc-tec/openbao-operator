---
title: Dependency License Policy
description: License policy for shipped OpenBao Operator dependencies, including the allowlist, special handling for MPL-2.0, blocked license classes, and local verification steps.
pageType: reference
journey: contribute
---

<PageHeader
  title="Use this page when a dependency change could affect what the project is allowed to ship."
  lede="The repository enforces license policy on shipped dependencies, not just on whatever appears in the module graph during development. This page defines the allowed set, the extra handling required for MPL-2.0, and the checks maintainers expect before a dependency change is merged."
/>

<Callout type="note" title="Policy, not legal advice">

This page defines project policy for contributors and maintainers. It does not replace legal review.

</Callout>

<DecisionTable
  title="License policy summary"
  kind="reference"
  columns={["License class", "Project policy", "Contributor action"]}
  rows={[
    {
      cells: [
        "Permissive licenses such as `Apache-2.0`, `BSD-2-Clause`, `BSD-3-Clause`, `ISC`, `MIT`, and `Unicode-DFS-2016`",
        "Allowed by default for shipped binaries.",
        "Keep notices intact and still run the normal verification flow.",
      ],
      emphasis: "recommended",
    },
    {
      cells: [
        "`MPL-2.0`",
        "Allowed with explicit maintainer care because it is file-level copyleft, not whole-program copyleft.",
        "Preserve notices, avoid casual vendored patches, and call out the dependency explicitly in the PR.",
      ],
    },
    {
      cells: [
        "Strong copyleft, source-available, field-of-use restricted, unknown, or unrecognized licenses",
        "Not allowed for shipped dependencies.",
        "Do not merge until the dependency is removed or replaced.",
      ],
      emphasis: "caution",
    },
  ]}
/>

## Scope

The blocking license gate covers shipped Go binaries:

- `./cmd/controller`
- `./cmd/bao-backup`
- `./cmd/bao-upgrade`
- `./cmd/provisioner`

Use vendored mode when evaluating the policy so local results match build and release behavior.

## Allowed licenses

These licenses are allowed without additional license-specific review:

- `Apache-2.0`
- `BSD-2-Clause`
- `BSD-3-Clause`
- `ISC`
- `MIT`
- `Unicode-DFS-2016`

## Allowed with obligations

`MPL-2.0` is allowed, but maintainers expect explicit handling:

1. Do not patch vendored `MPL-2.0` files casually.
2. Do not copy `MPL-2.0` code into first-party project files without explicit review.
3. Preserve upstream license and notice files during redistribution.
4. If modified `MPL-2.0` files are redistributed, keep corresponding source for those modified files available as required by the license.
5. Call out newly introduced `MPL-2.0` dependencies in the PR description so the review trail is explicit.

## Forbidden licenses

Do not ship dependencies under these licenses or license classes:

- `GPL-2.0`
- `GPL-3.0`
- `AGPL-3.0`
- `LGPL-2.1`
- `LGPL-3.0`
- `SSPL`
- `BUSL` / `BSL`
- `Elastic License`
- `Commons-Clause`
- non-commercial, no-derivatives, field-of-use-restricted, or source-available-only licenses
- `Unknown`, `NOASSERTION`, and custom or unrecognized licenses

<CommandBlock
  language="bash"
  label="verify"
  title="Local license verification"
  code={`make verify-vendor
make license-check
make license-report`}
>
  `make license-check` is the canonical full-tree gate for this repository. `make license-report` writes the report and stderr log under `dist/licenses/`.
</CommandBlock>

<DecisionTable
  title="How the policy is enforced"
  kind="reference"
  columns={["Enforcement layer", "What it checks", "Implementation surface"]}
  rows={[
    {
      cells: [
        "Pull-request dependency review",
        "Newly introduced dependency vulnerabilities in package manifests and lockfiles.",
        "`.github/workflows/dependency-review.yml` and `.github/dependency-review-config.yml`.",
      ],
    },
    {
      cells: [
        "Vendored full-tree gate",
        "The shipped dependency graph for project binaries in the same mode used by build and release paths.",
        "`make license-check` with `go-licenses`.",
      ],
      emphasis: "recommended",
    },
    {
      cells: [
        "Documentation and maintainer review",
        "Policy intent, special cases such as `MPL-2.0`, and explicit approval of allowlist changes.",
        "This page, PR review, and matching CI configuration updates.",
      ],
    },
  ]}
/>

## Updating the policy

Treat allowlist changes as maintainer-level changes. When you change the allowed or forbidden set:

1. Explain the change in the PR description.
2. Update this document in the same PR.
3. Update the matching machine-enforced CI configuration.

## GitHub dependency review scope

The GitHub dependency review workflow is currently used as a pull-request
vulnerability gate, not as the canonical license gate. In practice, its npm
license metadata is not reliable enough for this repository's documentation-site
dependencies.

License policy is still enforced, but the authoritative check is the shipped
artifact workflow around `make license-check` and maintainer review of
dependency-policy changes.

<NextActions
  title="Related governance pages"
  items={[
    {
      label: "Supply chain security",
      description: "See how dependency policy fits into the larger artifact-trust model.",
      to: "/contribute/supply-chain-security",
    },
    {
      label: "Release management",
      description: "Use the release workflow when a dependency change is already on the path to publication.",
      to: "/contribute/release-management",
    },
    {
      label: "Project governance",
      description: "Return to the governance landing page for the broader policy and maintainer map.",
      to: "/contribute/project-governance",
    },
  ]}
/>
