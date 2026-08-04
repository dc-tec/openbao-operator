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
        "Merge the release-please PR so the `Release Tag` workflow can resolve the merged release PR, create the signed stable version tag, and create the draft GitHub Release from `main` or a `release-*` branch.",
        "GitHub Release assets, OCI Helm chart, signed images, stable docs snapshot, docs deployment, and provenance evidence.",
        "Stable releases become permanent release-line docs and own the default `/docs` route.",
      ],
      emphasis: "recommended",
    },
    {
      cells: [
        "Patch release",
        "Backport the targeted fixes to the relevant `release-*` branch, then use the `Prepare Release-As PR` workflow to create the auditable `Release-As: X.Y.Z` marker PR.",
        "Same release artifacts as stable releases, built from the patch branch tag.",
        "Keep the patch branch narrow; patch releases in an existing stable line reuse that line's docs snapshot.",
      ],
    },
    {
      cells: [
        "Prerelease",
        "Prefer the `Prepare Release-As PR` workflow to create a tiny PR that carries an empty commit with `Release-As: 0.1.0-rc.6`, then merge the resulting release-please PR so the `Release Tag` workflow creates the signed prerelease tag and draft GitHub Release.",
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

Before merging the first stable `X.Y.0` release PR for a release line, snapshot the docs for that release line and commit the generated artifacts. Patch releases in the same line reuse the `X.Y.0` docs version, but user-facing docs fixes for that patch must refresh the existing `X.Y.0` snapshot from the release branch. Prereleases continue to use `/docs/next` and release notes only; do not add patch, `-alpha`, `-beta`, or `-rc` entries to `website/versions.json`.

</Callout>

<Callout type="note" title="Patch branch workflow state">

GitHub Actions runs workflow definitions from the branch that receives the push. Before using release-please on an older `release-*` branch for the first time, make sure the branch contains the current release workflow support, including `Release Please PR`, `Release Tag`, and `Prepare Release-As PR` behavior.

</Callout>

<Callout type="important" title="Create a release line after its first stable release">

Cut prereleases and the first stable `X.Y.0` release from a frozen `main`. Create `release-X.Y` from the
published stable commit only after `X.Y.0` completes. The release branch then becomes the source for narrowly
scoped `X.Y.Z` patch releases while normal development resumes on `main`.

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
  title="Refresh docs for a patch release line"
  code={`git switch release-X.Y
make docs-refresh-version DOCS_VERSION=X.Y.0`}
>
  Run this from the release branch after backporting docs that apply to the patch release. This updates the existing release-line docs snapshot without adding a patch version to `website/versions.json`.
</CommandBlock>

<CommandBlock
  language="bash"
  label="configure"
  title="Cut an explicit release with a Release-As PR"
  code={`# In GitHub Actions, run:
# Prepare Release-As PR
#
# target_branch: main        # or release-0.2
# version: 0.1.0-rc.6       # or 0.2.1`}
>
  This workflow validates the requested release line and repository state, then creates a tiny PR containing an
  empty `Release-As: <version>` commit. Merge that marker PR first; the branch-aware Release Please workflow then
  opens or updates the real release PR.
</CommandBlock>

The marker branch uses the `automation/release-as-*` namespace, outside the `release-X.Y` namespace watched by
Release Please. Preparing or updating the marker therefore cannot create a release PR before the marker merges.
The workflow also rejects a missing target branch, a version for the wrong release line, an existing tag or GitHub
Release, an open release-please PR, and conflicting marker state.

<Callout type="note" title="`workflow_dispatch` `release_as` path">

`Release Please PR` still exposes a `workflow_dispatch` `release_as` input, but maintainers should prefer the `Prepare Release-As PR` workflow. The marker PR leaves the version override in git history and works consistently for `main` and `release-*` branches.

</Callout>

<Callout type="note" title="Helm chart changelog">

Release-please remains the source of truth for release notes. After release-please opens or updates a release PR,
the `Release Please PR` workflow syncs `charts/openbao-operator/Chart.yaml` so Artifact Hub receives
`artifacthub.io/changes`, image metadata, prerelease state, and security-update state from the release-please
changelog.

For a prerelease, Artifact Hub metadata contains only that exact changelog section. For the matching stable
release, metadata rolls up and deduplicates the stable and `X.Y.Z-*` prerelease sections. This keeps the generated
changelog incremental while giving the stable chart a complete change list. Security-scoped entries and dependency
fixes that identify vulnerabilities, CVEs, or GHSAs set `artifacthub.io/containsSecurityUpdates`.

</Callout>

<Callout type="note" title="Human release notes">

Keep `CHANGELOG.md` generated by release-please. Put hand-written release summaries, migration notes, and operator-facing callouts in `release-notes/X.Y.Z.md`; the website release generator and draft GitHub Release creation prepend that file to the generated changelog entry for the matching version.

For patch releases, make the source that release-please sees match the release note you want. Prefer one cherry-picked conventional commit per user-facing fix. If a backport PR is squash-merged, make the squash title deliberately user-facing because it becomes the generated changelog entry. Use `release-notes/X.Y.Z.md` for extra context, but do not hand-edit generated `CHANGELOG.md` sections.

</Callout>

<DecisionTable
  title="Release manager checklist"
  columns={["Stage", "What to prove before moving on"]}
  rows={[
    {
      cells: [
        "Pre-flight",
        "Release-please PR looks correct, docs are updated, new stable release lines have a committed `X.Y.0` docs snapshot, compatibility docs are current, CI is green, full E2E and previous-stable Development plus Hardened operator-upgrade evidence is clean, nightly regressions are reviewed, and performance findings are either cleared or explicitly accepted by maintainers.",
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
        "GitHub Release exists, assets verify, provenance evidence is recorded, Artifact Hub metadata is visible, no unexpected release-please PR or branch remains, and announcement links are captured.",
      ],
      emphasis: "caution",
    },
  ]}
/>

<CommandBlock
  language="bash"
  label="verify"
  title="Run post-release verification"
  code={`VERSION=X.Y.Z REPO=dc-tec/openbao-operator hack/ci/verify-post-release.sh`}
>
  The `Post-Release Verification` workflow runs this automatically after a successful `Release` workflow and can also be started manually for a tag. Local runs require `gh`, `jq`, `git`, Docker Buildx, and `cosign`. The verifier checks the remote tag, published GitHub Release assets, checksum signature, OCI Helm chart publication and signature, release-please pending-label cleanup, and leftover release-please PRs or branches.
</CommandBlock>

<Callout type="note" title="Post-release verification evidence">

The `Release` workflow removes the release-please `autorelease: pending` label from the merged release PR after the GitHub Release is published. Published GitHub Releases are immutable in this repository, so the follow-up workflow does not try to attach new release assets. Instead, it keeps a 30-day Actions artifact named `post-release-verification-<version>` and writes or updates a release PR comment with the release links, workflow runs, chart digest, and evidence sha256. The JSON evidence captures the release URL, release workflow run, release PR, chart digest, published asset list, provenance index, and the post-release invariant checks that passed.

</Callout>

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

Run `hack/ci/verify-post-release.sh` first and use the verification skeleton above for deeper spot checks or release evidence capture. Keep the exact workflow identity, tag ref, and digest-pinned subjects aligned with the artifacts that were just published. Once the release is fully published, delete the matching `release-please--branches--<base>` branch unless release-please still has an active release PR for that base branch.

<Callout type="note" title="release-please token requirements">

Release automation must use non-default tokens so the resulting tag and GitHub Release can trigger downstream workflows. Use two repo-scoped GitHub Apps:

- `OPENBAO_OPERATOR_RELEASE_PR_CLIENT_ID` and `OPENBAO_OPERATOR_RELEASE_PR_PRIVATE_KEY` for PR-only `release-please`
- `OPENBAO_OPERATOR_RELEASE_TAG_CLIENT_ID` and `OPENBAO_OPERATOR_RELEASE_TAG_PRIVATE_KEY` for the custom `Release Tag` workflow

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
