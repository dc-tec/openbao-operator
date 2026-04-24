---
title: Release Management
description: Maintainer release workflow for OpenBao Operator, including release-please, stable docs snapshots, channel behavior, artifact verification, and post-release checks.
pageType: task
journey: contribute
---

<PageHeader
  title="Release management workflow"
  lede="Build-once, promote-everywhere release workflow covering versioning, changelog state, release orchestration, signing, docs deployment, and release evidence."
/>

<Callout type="note" title="Maintainer workflow">

[Release Policy](pathname:///docs/next/reference/release-policy) covers the public release cadence, channel rules, and stable release gates. The steps below cover the maintainer workflow once a release is being executed.

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
        "Stable releases become permanent release-line docs and own the default `/docs` route.",
      ],
      emphasis: "recommended",
    },
    {
      cells: [
        "Prerelease",
        "Prefer a tiny PR that carries an empty commit with `Release-As: 0.1.0-rc.6`, then merge the resulting release-please PR so the `Release Tag` workflow creates the signed prerelease tag and draft GitHub Release.",
        "GitHub Release assets, OCI Helm chart, signed images, docs site deployment, and provenance evidence.",
        "Prereleases use `/docs/next` plus release notes; do not create a permanent versioned docs snapshot.",
      ],
    },
    {
      cells: [
        "Edge",
        "CI success on `main`.",
        "Mutable and immutable edge manifests plus signed images and provenance metadata.",
        "Pre-release validation channel; production support expectations remain with stable releases.",
      ],
    },
    {
      cells: [
        "Nightly",
        "Nightly validation success.",
        "Nightly manifests, tags, and provenance metadata.",
        "Scheduled validation and drift-detection channel; stable releases remain the publication path.",
      ],
    },
  ]}
/>

<Callout type="important" title="Stable release-line docs snapshots">

Before merging the first stable `X.Y.0` release PR for a release line, snapshot the docs for that release line and commit the generated artifacts. Patch releases in the same line publish release notes and reuse the `X.Y.0` docs snapshot. Prereleases continue to use `/docs/next` and release notes only; do not add patch, `-alpha`, `-beta`, or `-rc` entries to `website/versions.json`.

</Callout>

<CommandBlock
  language="bash"
  label="configure"
  title="Snapshot docs for the stable release line"
  code={`make docs-version DOCS_VERSION=X.Y.0`}
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
  This flow creates an explicit prerelease target on `main` when the inferred Conventional Commit bump is not the intended version.
</CommandBlock>

<Callout type="note" title="`workflow_dispatch` `release_as` path">

`Release Please PR` still exposes a `workflow_dispatch` `release_as` input, and it can produce the expected release PR. The `Release-As:` PR path remains the fallback for release lines where the dispatch path is not yet reliable.

</Callout>

<DecisionTable
  title="Release manager checklist"
  columns={["Stage", "What to prove before moving on"]}
  rows={[
    {
      cells: [
        "Pre-flight",
        "Release-please PR looks correct, docs are updated, new stable release lines have a committed `X.Y.0` docs snapshot, compatibility docs are current, CI is green, full E2E release evidence is clean, nightly regressions are reviewed, and performance findings are either cleared or explicitly accepted by maintainers.",
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
      description: "Release policy covers cadence and release-gate rules.",
      to: "/docs/next/reference/release-policy",
    },
    {
      label: "Distribution",
      description: "Distribution covers Artifact Hub metadata, chart publishing assumptions, and deferred OLM posture.",
      to: "/contribute/distribution",
    },
    {
      label: "Continuous integration",
      description: "Continuous integration covers workflow behavior before publish.",
      to: "/contribute/ci",
    },
    {
      label: "Project governance",
      description: "Project governance covers SDLC and supply-chain policy changes.",
      to: "/contribute/project-governance",
    },
  ]}
/>
