---
title: Respond to a publishing incident
description: Maintainer runbook for containing and recovering from CI, release, GitHub App, signing-key, or GHCR compromise.
eyebrow: Maintainer runbook
weight: 7
verifiedBy:
  - .github/workflows/release-please.yml
  - .github/workflows/prepare-release-as-pr.yml
  - .github/workflows/release-tag.yml
  - .github/workflows/release.yml
  - .github/workflows/post-release-verification.yml
  - .github/workflows/publish-edge.yml
  - .github/workflows/publish-nightly.yml
  - .github/workflows/ghcr-housekeeping.yml
  - .github/workflows/trusted-root-refresh.yml
  - hack/ci/create-release-tag-and-draft.sh
  - hack/ci/generate-provenance-index.sh
  - hack/ci/verify-post-release.sh
  - SECURITY.md
---

Contain publication first. Preserve enough evidence to identify affected runs and subjects, but do not leave a trusted
workflow, GitHub App, or signing key active while collecting it.

## Contain the incident

Perform these actions from a known-clean administrator session:

1. Disable GitHub Actions or restrict execution to a reviewed allowlist while the cause is unknown.
2. Suspend the `openbao-operator-release-tag` GitHub App. Suspend `openbao-operator-release-pr` as well when release PR or
   marker activity is in scope.
3. Block SemVer tag creation through the repository ruleset and stop approvals for the `release-publish` environment.
4. Pause GHCR housekeeping so evidence and reachability metadata are not deleted during the investigation.
5. Revoke active sessions or tokens that may belong to a compromised maintainer or automation identity.
6. Rotate affected App private keys, release-tag GPG material, repository or organization secrets, PATs, SSH keys, and
   account credentials after capturing the identifiers needed for the incident record.

These controls live in GitHub settings and App administration, not in the checkout. Record who changed each control and
when.

{{< callout type="warning" title="Do not repair publication in place" >}}
Do not move an existing SemVer tag, overwrite a published subject, or delete evidence to make verification pass. Treat
unexpected tags, releases, and digests as incident subjects and document the replacement or revocation decision.
{{< /callout >}}

## Capture workflow evidence

List every workflow that can change release source, tags, packages, trust roots, or public channel state. The current
high-trust set is broader than the stable release workflow alone.

{{< command label="inspect" title="List recent high-trust runs" >}}
REPO=dc-tec/openbao-operator

for workflow in \
  ci.yml \
  nightly.yml \
  release-please.yml \
  prepare-release-as-pr.yml \
  release-tag.yml \
  release.yml \
  post-release-verification.yml \
  publish-edge.yml \
  publish-nightly.yml \
  ghcr-housekeeping.yml \
  pages.yml \
  trusted-root-refresh.yml
do
  gh run list --repo "${REPO}" --workflow "${workflow}" --limit 20
done
{{< /command >}}

For each suspicious run, record the run ID, event, actor, source SHA, head branch, conclusion, attempt number, and linked
artifacts before logs or artifacts expire.

{{< command label="inspect" title="Inspect and retain one run" >}}
RUN_ID=1234567890

gh run view "${RUN_ID}" \
  --repo dc-tec/openbao-operator \
  --json databaseId,event,headBranch,headSha,displayTitle,conclusion,createdAt,updatedAt,url

gh api "repos/dc-tec/openbao-operator/actions/runs/${RUN_ID}" \
  --jq '{id,event,actor:.actor.login,head_branch,head_sha,status,conclusion,run_attempt,created_at,updated_at,html_url}'

gh run view "${RUN_ID}" --repo dc-tec/openbao-operator --log > "run-${RUN_ID}.log"
gh run download "${RUN_ID}" --repo dc-tec/openbao-operator --dir "run-${RUN_ID}-artifacts"
{{< /command >}}

Store collected evidence outside the potentially compromised repository and record a checksum for every exported file.

## Compare public state with expected state

Inspect tags, draft and published releases, images, executors, and the chart repository.

{{< command label="inspect" title="Inventory release and registry state" >}}
gh release list --repo dc-tec/openbao-operator --limit 30
git ls-remote --tags origin

for repository in \
  openbao-operator \
  openbao-init \
  openbao-backup \
  openbao-upgrade \
  charts/openbao-operator
do
  crane ls "ghcr.io/dc-tec/${repository}" | sort -V | tail -n 30
done
{{< /command >}}

For an affected version, resolve the annotated tag to its commit and verify the GPG signature. Compare the workflow,
local actions, release scripts, dependency pins, chart metadata, and release notes at that commit with a reviewed
baseline.

{{< command label="inspect" title="Resolve and verify a release tag" >}}
VERSION=X.Y.Z

git fetch origin "refs/tags/${VERSION}:refs/tags/${VERSION}"
git verify-tag "${VERSION}"
git rev-list -n1 "${VERSION}"
gh release view "${VERSION}" \
  --repo dc-tec/openbao-operator \
  --json tagName,isDraft,isPrerelease,assets,url
{{< /command >}}

Run the repository verifier for a release that should be valid:

{{< command label="verify" title="Verify published subjects and produce evidence" >}}
VERSION=X.Y.Z \
REPO=dc-tec/openbao-operator \
EVIDENCE_OUT="incident-${VERSION}-verification.json" \
hack/ci/verify-post-release.sh
{{< /command >}}

Signature and provenance success proves which workflow identity and source ref produced a subject. It does not prove
that the workflow source at that ref was benign. Review the tagged source and the identity controls as separate evidence.

## Determine the compromised boundary

| Boundary | Evidence to check |
| --- | --- |
| Workflow source | Default-branch and tagged workflow diffs, pinned actions, local composite actions, release scripts, and recent merged PRs |
| Release identities | App installation permissions, private-key history, SemVer ruleset bypass, environment approvals, and workflow token permissions |
| Maintainer identity | Sessions, MFA events, SSH and GPG keys, PATs, recovery methods, and unexpected settings changes |
| Published subjects | Image and chart digests, signatures, attestations, checksum bundles, SBOMs, provenance index, draft releases, and channel manifests |
| Registry cleanup | GHCR housekeeping runs, deletion plans, retained referrers, and any package or version deletion |
| Trust roots | `trusted-root-refresh` inputs and commits, signer expectations, and affected verification policy |

Assume every secret available to a compromised workflow or identity is exposed. Rotate from the root of trust outward:
account recovery and sessions, GitHub App keys, release-tag GPG key, repository and organization secrets, PATs, SSH keys,
then downstream provider credentials.

## Recover in controlled order

1. Document the root cause, affected time window, workflow runs, commits, tags, releases, and GHCR digests.
2. Land reviewed workflow or policy repairs through the normal protected branch from a clean administrator session.
3. Complete credential rotation and verify App permissions, tag rules, and environment reviewers.
4. Re-enable only the minimum Actions surface needed to validate the repair.
5. Unsuspend the PR App first. Keep the tag App suspended while release PR behavior is verified.
6. Run normal CI, then an edge or nightly publication if that channel is part of the repaired boundary.
7. Create a controlled prerelease or release candidate through the marker and release-please path.
8. Verify the tag, published digests, signatures, attestations, assets, and post-release evidence.
9. Restore stable-release permissions and unsuspend the tag App only after the controlled release succeeds.

Record every recovery action and its reviewer in the incident notes. Report externally according to
[`SECURITY.md`](https://github.com/dc-tec/openbao-operator/blob/main/SECURITY.md) when users or published artifacts may be
affected.

{{< callout type="warning" title="Single-maintainer constraint" >}}
This procedure creates a repeatable evidence trail, not independent approval. If the maintainer account is compromised,
treat repository settings, Actions secrets, Apps, tags, and publication state as untrusted until verified from another
administrative root.
{{< /callout >}}

Return to [release management]({{< relref "/contribute/release.md" >}}) only after containment and trust restoration are
complete.
