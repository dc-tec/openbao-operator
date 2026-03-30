---
title: Release Management
description: Maintainer release workflow for OpenBao Operator, including release-please, stable docs snapshots, channel behavior, artifact verification, and post-release checks.
pageType: task
journey: contribute
---

<PageHeader
  title="Release once, promote by digest, and prove the published artifacts before you announce them."
  lede="OpenBao Operator uses a build-once, promote-everywhere release model. `release-please` owns versioning, changelog state, and release orchestration, while publish workflows own build, verification, signing, docs deployment, and release evidence."
/>

<Callout type="note" title="This page is the maintainer workflow, not the public cadence contract">

Use [Release Policy](pathname:///docs/next/reference/release-policy) for the public release cadence, channel rules, and stable release gates. Use this page for the operational workflow maintainers follow once a release is actually being executed.

</Callout>

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
        "Merge the release-please PR so the `Release Tag` workflow can resolve the merged release PR, create the signed stable version tag, and create the draft GitHub Release from `main`.",
        "GitHub Release assets, OCI Helm chart, signed images, stable docs snapshot, docs deployment, and provenance evidence.",
        "Stable releases become permanent versioned docs and own the default `/docs` route.",
      ],
      emphasis: "recommended",
    },
    {
      cells: [
        "Prerelease",
        "Prefer a tiny PR that carries an empty commit with `Release-As: 0.1.0-rc.6`, then merge the resulting release-please PR so the `Release Tag` workflow creates the signed prerelease tag and draft GitHub Release.",
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
  title="Cut an explicit prerelease with a Release-As PR"
  code={`git switch -c chore/release-as-0.1.0-rc.6
git commit --allow-empty -m $'chore: release 0.1.0-rc.6\n\nRelease-As: 0.1.0-rc.6\nSigned-off-by: Your Name <you@example.com>'`}
>
  Open a tiny PR with only this empty commit, merge it, and let `Release Please PR` recreate the release PR on `main`. Use this when you need a specific `-alpha`, `-beta`, or `-rc` target instead of the bump inferred from normal Conventional Commits.
</CommandBlock>

<Callout type="note" title="workflow_dispatch `release_as` is still optional, not the authoritative path">

`Release Please PR` still exposes a `workflow_dispatch` `release_as` input, and it is useful when it produces the expected release PR. Keep the `Release-As:` PR path as the reliable fallback until the dispatch path proves consistently correct for your release line.

</Callout>

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

Release automation must use non-default tokens so the resulting tag and GitHub Release can trigger downstream workflows. Use two repo-scoped GitHub Apps:

- `OPENBAO_OPERATOR_RELEASE_PR_APP_ID` and `OPENBAO_OPERATOR_RELEASE_PR_PRIVATE_KEY` for PR-only `release-please`
- `OPENBAO_OPERATOR_RELEASE_TAG_APP_ID` and `OPENBAO_OPERATOR_RELEASE_TAG_PRIVATE_KEY` for the custom `Release Tag` workflow

The tag app should be the only actor with semver tag ruleset bypass, and it only needs repository `contents: write`.

Signed release tags also require these repo secrets:

- `OPENBAO_OPERATOR_RELEASE_TAG_GPG_PRIVATE_KEY` - armored private key for the dedicated release-signing identity
- `OPENBAO_OPERATOR_RELEASE_TAG_GPG_PASSPHRASE` - passphrase for that private key
- `OPENBAO_OPERATOR_RELEASE_TAG_GPG_NAME` - tagger name to write into signed release tags
- `OPENBAO_OPERATOR_RELEASE_TAG_GPG_EMAIL` - tagger email to write into signed release tags

Upload the matching public key to the GitHub identity that should show the `Verified` badge for release tags. Prefer a dedicated release-signing key instead of a day-to-day maintainer key. A PAT fallback is possible through `RELEASE_PLEASE_TOKEN`, but a bot identity is safer than a maintainer’s personal token.

</Callout>

<NextActions
  title="After release execution"
  items={[
    {
      label: "Release policy",
      description: "Go back to the public cadence and release-gate contract when the release rules themselves need to change.",
      to: "/docs/next/reference/release-policy",
    },
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
