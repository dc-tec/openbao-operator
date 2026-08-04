---
title: Supply Chain Security
description: Supply-chain security controls for OpenBao Operator, including provenance, reproducibility, signing, release evidence, and channel hardening.
pageType: concept
journey: contribute
---

<PageHeader
  title="Supply-chain security controls"
  lede="Provenance, reproducibility, and publish-time checks for OpenBao Operator release assets."
/>

<DiagramFrame
  title="Build once, verify, then promote"
  caption="Promotion is gated by trust evidence, not by rebuilding the same subject a second time during publish."
  code={`graph TD
    Commit["Commit or tag"] --> Build["Build immutable artifacts"]
    Build --> Provenance["Verify provenance"]
    Build --> Repro["Verify reproducibility"]
    Provenance --> Promote["Promote by digest"]
    Repro --> Promote
    Promote --> Sign["Sign and attest publish subjects"]
    Sign --> Publish["Publish releases, manifests, and metadata"]`}
/>

<DecisionTable
  title="Channel coverage"
  columns={["Channel", "Primary workflow path", "Blocking trust gates", "Published output"]}
  rows={[
    {
      cells: [
        "CI",
        "`.github/workflows/ci.yml`",
        "Validation only. No publish path.",
        "PR and branch confidence, but no public artifacts.",
      ],
    },
    {
      cells: [
        "Edge",
        "`.github/workflows/publish-edge.yml` plus reusable hardening",
        "Provenance verification and byte reproducibility before publish.",
        "GitHub Pages edge manifests, checksums, and provenance index.",
      ],
    },
    {
      cells: [
        "Nightly",
        "`.github/workflows/publish-nightly.yml` plus reusable hardening",
        "Provenance verification and byte reproducibility before publish.",
        "GitHub Pages nightly manifests, checksums, and provenance index.",
      ],
    },
    {
      cells: [
        "Stable release",
        "`.github/workflows/release.yml`",
        "Provenance, reproducibility, signing, and release evidence gates.",
        "GitHub Release assets, OCI images, chart artifacts, versioned docs, and machine-readable provenance metadata.",
      ],
      emphasis: "recommended",
    },
    {
      cells: [
        "Prerelease",
        "`.github/workflows/release.yml`",
        "Provenance, reproducibility, signing, and release evidence gates.",
        "GitHub Release assets, OCI images, chart artifacts, release notes, and docs deployment through `/docs/next`.",
      ],
    },
  ]}
/>

<DecisionTable
  title="Control families"
  columns={["Control family", "What it protects", "Primary implementation surface"]}
  rows={[
    {
      cells: [
        "Governance",
        "Branch, tag, and change-management expectations for release-critical paths.",
        "Repository rulesets, signed semver tags, CODEOWNERS, PR checks, and pinned GitHub Actions usage.",
      ],
    },
    {
      cells: [
        "Build inputs",
        "Deterministic dependency resolution and low-drift toolchains.",
        "Vendored Go modules, pinned base-image digests, pinned tool versions, and workflow-defined build parameters.",
      ],
    },
    {
      cells: [
        "Provenance and signing",
        "Evidence that published subjects came from the expected workflow and identity.",
        "`actions/attest-build-provenance`, `cosign`, `gh attestation`, and the release verification scripts.",
      ],
      emphasis: "recommended",
    },
    {
      cells: [
        "Reproducibility",
        "Confidence that an independent rebuild matches expected bytes before publication.",
        "`hack/ci/verify-byte-reproducibility.sh`, deterministic SPDX normalization, and release/channel hardening gates.",
      ],
    },
    {
      cells: [
        "Release evidence",
        "Retained proof that a release met the hardening contract at publish time.",
        "Workflow run metadata, verification output, artifact listings, and provenance indexes.",
      ],
    },
  ]}
/>

<CommandBlock
  language="bash"
  label="inspect"
  title="Inspect hardening entry points"
  code={`rg -n "verify-image-attestations|verify-byte-reproducibility|provenance-index" \\
  .github/workflows hack/ci

sed -n '1,220p' .github/workflows/release.yml
sed -n '1,220p' .github/workflows/reusable-channel-hardening.yml`}
>
  Use these commands to trace the workflow or script that implements a trust gate.
</CommandBlock>

<Callout type="warning" title="Single-maintainer operating constraint">

Human two-person approval controls are still limited by the current single-maintainer operating model. The project compensates with stronger automation, pinned workflow identity checks, and explicit release evidence requirements, but this does not create true multi-person release separation on its own.

</Callout>

<DecisionTable
  title="Common failure classes"
  columns={["Failure", "Likely cause", "Where to look first"]}
  rows={[
    {
      cells: [
        "Attestation verification fails",
        "Workflow identity, signer identity, or source ref does not match the expected constraint.",
        "Release workflow inputs and `hack/ci/verify-image-attestations.sh`.",
      ],
    },
    {
      cells: [
        "Byte reproducibility fails",
        "A build input or emitted artifact changed nondeterministically between rebuilds.",
        "`hack/ci/verify-byte-reproducibility.sh`, normalized SPDX output, and build metadata sources.",
      ],
      emphasis: "caution",
    },
    {
      cells: [
        "Checksums or chart subject verification fails",
        "A signed or attested subject was regenerated after evidence was captured.",
        "`hack/ci/verify-release-artifact-attestations.sh` and the release job ordering.",
      ],
    },
  ]}
/>

<NextActions
  title="Turn policy into release work"
  items={[
    {
      label: "Release management",
      description: "Release procedure for verification, publish, and post-release steps.",
      to: "/contribute/release-management",
    },
    {
      label: "Continuous integration",
      description: "How these controls appear earlier in the branch and PR lifecycle.",
      to: "/contribute/ci",
    },
    {
      label: "Dependency license policy",
      description: "Dependency policy for which shipped dependencies may enter the release graph.",
      to: "/contribute/dependency-licenses",
    },
    {
      label: "Incident response",
      description: "Supply-chain incident runbook for freezing publishing, rotating credentials, or inspecting recent releases.",
      to: "/contribute/supply-chain-incident-response",
    },
  ]}
/>
