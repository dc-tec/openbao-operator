---
title: Release Management
description: Maintainer release workflow for OpenBao Operator, including release-please, stable docs snapshots, channel behavior, artifact verification, and post-release checks.
pageType: task
journey: contribute
---

<PageHero
  variant="compact"
  eyebrow="Contribute / Validate & Ship"
  title="Release once, promote by digest, and prove the published artifacts before you announce them."
  lede="OpenBao Operator uses a build-once, promote-everywhere release model. `release-please` owns versioning, changelog state, and release orchestration, while publish workflows own build, verification, signing, docs deployment, and release evidence."
  actions={[
    {label: "Open CI behavior", to: "/contribute/ci", variant: "primary"},
    {label: "Open distribution", to: "/contribute/distribution", variant: "secondary"},
  ]}
>
  <Checklist
    title="Use this page when you need to"
    items={[
      "prepare or review a release-please PR before it is merged",
      "cut a prerelease with Release-As override semantics",
      "snapshot stable docs, verify published artifacts, and record release evidence",
      "understand what stable, prerelease, edge, and nightly channels are allowed to publish",
    ]}
  />
</PageHero>

<DiagramFrame
  title="Build once, promote everywhere"
  caption="Release workflows build immutable artifacts first, then gate, sign, and publish by digest."
  code={`graph TD
    PR["Release-please PR merged"] --> Tagger["Tag workflow"]
    Tagger --> Tag["Git tag + draft GitHub Release"]
    Tag --> Build["Build once"]
    Build --> Gates["Security, E2E, performance, reproducibility"]
    Gates --> Promote["Promote by digest"]
    Promote --> Sign["Sign and attest"]
    Sign --> Publish["GitHub Release, GHCR, docs, metadata"]`}
/>

<DecisionTable
  title="Release channels"
  columns={["Channel", "Trigger", "What it publishes", "Operational rule"]}
  rows={[
    {
      cells: [
        "Stable release",
        "Merge the release-please PR so the tag workflow creates the stable version tag and draft GitHub Release from `main`.",
        "GitHub Release assets, OCI Helm chart, signed images, stable docs snapshot, docs deployment, and provenance evidence.",
        "Stable releases become permanent versioned docs and own the default `/docs` route.",
      ],
      emphasis: "recommended",
    },
    {
      cells: [
        "Prerelease",
        "Merge the release-please PR with an explicit prerelease target such as `-rc.1` or `-beta.1` so the tag workflow creates the prerelease tag and draft GitHub Release.",
        "GitHub Release assets, OCI Helm chart, signed images, docs site deployment, and provenance evidence.",
        "Prereleases use `/docs/next` plus release notes; do not create a permanent versioned docs snapshot unless there is a deliberate preview exception.",
      ],
    },
    {
      cells: [
        "Edge",
        "CI success on `main`.",
        "Mutable and immutable edge manifests plus signed images and provenance metadata.",
        "Use for pre-release validation only; do not treat edge as production support policy.",
      ],
    },
    {
      cells: [
        "Nightly",
        "Nightly validation success.",
        "Nightly manifests, tags, and provenance metadata.",
        "Use for scheduled validation and drift detection, not as a replacement for a stable release.",
      ],
    },
  ]}
/>

<Callout type="important" title="Docs snapshots are for stable releases">

Before merging a stable release PR, snapshot the docs for the outgoing version and commit the generated artifacts. Prereleases continue to use `/docs/next` and do not need a permanent versioned docs snapshot.

</Callout>

<CommandBlock
  language="bash"
  label="configure"
  title="Snapshot docs for the stable release version"
  code={`make docs-version DOCS_VERSION=X.Y.Z`}
>
  This updates `website/versioned_docs/`, `website/versioned_sidebars/`, and `website/versions.json`.
</CommandBlock>

<CommandBlock
  language="bash"
  label="configure"
  title="Cut an explicit prerelease with Release-As"
  code={`git commit --allow-empty -m "chore: release 0.2.0-beta.1" -m "Release-As: 0.2.0-beta.1"
git push`}
>
  Use this when you need a specific `-alpha`, `-beta`, or `-rc` target instead of the bump inferred from normal Conventional Commits.
</CommandBlock>

<DecisionTable
  title="Release manager checklist"
  columns={["Stage", "What to prove before moving on"]}
  rows={[
    {
      cells: [
        "Pre-flight",
        "Release-please PR looks correct, docs are updated, stable releases have a committed docs snapshot, compatibility docs are current, CI is green, the PR gate is satisfied, and the performance baseline evidence matches `make verify-perf`.",
      ],
      emphasis: "recommended",
    },
    {
      cells: [
        "Publish",
        "Release workflow passes the `release-publish` environment gate, then tags, signs, attests, publishes assets, and keeps chart/app versions aligned with the git tag.",
      ],
    },
    {
      cells: [
        "Post-release",
        "GitHub Release exists, assets verify, provenance evidence is recorded, Artifact Hub metadata is visible, and announcement links are captured.",
      ],
      emphasis: "caution",
    },
  ]}
/>

<CommandBlock
  language="bash"
  label="verify"
  title="Post-release verification skeleton"
  code={`IMAGE="ghcr.io/dc-tec/openbao-operator@sha256:<digest>"
CHART="ghcr.io/dc-tec/charts/openbao-operator@sha256:<digest>"

cosign verify \\
  --new-bundle-format=true \\
  --certificate-identity "https://github.com/dc-tec/openbao-operator/.github/workflows/release.yml@refs/tags/X.Y.Z" \\
  --certificate-oidc-issuer "https://token.actions.githubusercontent.com" \\
  "\${IMAGE}"

gh attestation verify "oci://\${CHART}" \\
  --repo dc-tec/openbao-operator \\
  --signer-workflow dc-tec/openbao-operator/.github/workflows/release.yml \\
  --source-ref refs/tags/X.Y.Z \\
  --cert-oidc-issuer https://token.actions.githubusercontent.com \\
  --deny-self-hosted-runners

jq '.release, .identity_constraints, .images, .chart, .release_artifacts.checksums_txt' provenance-index.json`}
>
  Verify images, charts, checksums, and the provenance index against the exact tag and workflow identity used for the release.
</CommandBlock>

## Verifying artifacts {#5-verifying-artifacts}

Use the verification skeleton above as the default post-release evidence pack. Keep the exact workflow identity, tag ref, and digest-pinned subjects aligned with the artifacts that were just published.

<Callout type="note" title="release-please token requirements">

`release-please` must use non-default tokens so the resulting tag and GitHub Release can trigger downstream workflows. Use two repo-scoped GitHub Apps:

- `OPENBAO_OPERATOR_RELEASE_PR_APP_ID` and `OPENBAO_OPERATOR_RELEASE_PR_PRIVATE_KEY` for PR-only `release-please`
- `OPENBAO_OPERATOR_RELEASE_TAG_APP_ID` and `OPENBAO_OPERATOR_RELEASE_TAG_PRIVATE_KEY` for tag-only `release-please`

The tag app should be the only actor with semver tag ruleset bypass. A PAT fallback is possible through `RELEASE_PLEASE_TOKEN`, but a bot identity is safer than a maintainer’s personal token.

</Callout>

<NextActions
  title="After release execution"
  items={[
    {
      label: "Distribution",
      description: "Open the public-distribution model when you need to update Artifact Hub metadata, chart publishing assumptions, or deferred OLM posture.",
      to: "/contribute/distribution",
    },
    {
      label: "Continuous integration",
      description: "Go back to workflow behavior if the release gate failed before publish.",
      to: "/contribute/ci",
    },
    {
      label: "Project governance",
      description: "Move into SDLC or supply-chain policy when the release rules themselves need to change.",
      to: "/contribute/project-governance",
    },
  ]}
/>
